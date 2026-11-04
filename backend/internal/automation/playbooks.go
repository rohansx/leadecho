package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/apikey"
	"leadecho/internal/database"
	"leadecho/internal/tools"
)

// ToolCaller runs a named agent tool for a workspace (*tools.Registry).
type ToolCaller interface {
	Call(ctx context.Context, workspaceID string, scopes []string, name string, args json.RawMessage) (any, error)
}

// Store is the data access the playbooks need beyond the tools.
type Store interface {
	UpdateMentionStatus(ctx context.Context, arg database.UpdateMentionStatusParams) (database.Mention, error)
	GetLeadByMention(ctx context.Context, arg database.GetLeadByMentionParams) (database.Lead, error)
	CreateLead(ctx context.Context, arg database.CreateLeadParams) (database.Lead, error)
	AutomationKeywordValue(ctx context.Context, arg database.AutomationKeywordValueParams) ([]database.AutomationKeywordValueRow, error)
}

// Runner executes playbooks deterministically: no LLM, only tool calls and
// fixed rules, so the same data always yields the same output.
type Runner struct {
	Tools ToolCaller
	Q     Store
}

// Item is one line of a playbook's output (a mention, lead or keyword).
type Item struct {
	MentionID string   `json:"mention_id,omitempty"`
	LeadID    string   `json:"lead_id,omitempty"`
	Title     string   `json:"title"`
	URL       string   `json:"url,omitempty"`
	Platform  string   `json:"platform,omitempty"`
	Score     *float32 `json:"score,omitempty"`
	Intent    string   `json:"intent,omitempty"`
	Note      string   `json:"note,omitempty"`
}

// Output is what a run produced; it is stored on the run and delivered.
type Output struct {
	Summary     string   `json:"summary"`
	Items       []Item   `json:"items,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
	Notes       []string `json:"notes,omitempty"` // non-fatal issues
	Data        any      `json:"data,omitempty"`
}

var (
	readScope = []string{apikey.ScopeRead}
	allScopes = []string{apikey.ScopeRead, apikey.ScopeWrite}
)

// Run executes the playbook of kind. since bounds "new since last run".
func (r Runner) Run(ctx context.Context, ws, kind string, cfg Config, since time.Time) (Output, error) {
	switch kind {
	case KindDailyTriage:
		return r.dailyTriage(ctx, ws, cfg, since)
	case KindLeadSweep:
		return r.leadSweep(ctx, ws, cfg)
	case KindAttributionReview:
		return r.attributionReview(ctx, ws, cfg)
	case KindCustom:
		return r.custom(ctx, ws, cfg)
	}
	return Output{}, fmt.Errorf("unknown automation kind %q", kind)
}

func (r Runner) call(ctx context.Context, ws string, scopes []string, name string, args any, into any) error {
	raw, err := json.Marshal(args)
	if err != nil {
		return err
	}
	res, err := r.Tools.Call(ctx, ws, scopes, name, raw)
	if err != nil {
		return err
	}
	if into == nil {
		return nil
	}
	// Round-trip through JSON: tools return JSON-shaped results by contract.
	b, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, into)
}

// ponytail: one page of 100 per query; raise or paginate if a workspace gets more per period.
func (r Runner) mentions(ctx context.Context, ws string, args map[string]any) ([]tools.MentionView, error) {
	args["limit"] = 100
	var out struct {
		Mentions []tools.MentionView `json:"mentions"`
	}
	if err := r.call(ctx, ws, readScope, "search_mentions", args, &out); err != nil {
		return nil, fmt.Errorf("search_mentions: %w", err)
	}
	return out.Mentions, nil
}

var intentWeight = map[string]int{"buy_signal": 3, "recommendation_ask": 2, "comparison": 1, "complaint": 1}

func score(m tools.MentionView) float32 {
	if m.Score == nil {
		return 0
	}
	return *m.Score
}

func intent(m tools.MentionView) string {
	if m.Intent == nil {
		return ""
	}
	return *m.Intent
}

// rank orders by score, then intent strength, then recency.
func rank(ms []tools.MentionView) {
	sort.SliceStable(ms, func(i, j int) bool {
		a, b := ms[i], ms[j]
		if score(a) != score(b) {
			return score(a) > score(b)
		}
		if wa, wb := intentWeight[intent(a)], intentWeight[intent(b)]; wa != wb {
			return wa > wb
		}
		return a.CreatedAt.After(b.CreatedAt)
	})
}

func mentionItem(m tools.MentionView, note string) Item {
	title := m.Content
	if m.Title != nil && *m.Title != "" {
		title = *m.Title
	}
	if len([]rune(title)) > 140 {
		title = string([]rune(title)[:140]) + "…"
	}
	return Item{MentionID: m.ID, Title: title, URL: m.URL, Platform: m.Platform, Score: m.Score, Intent: intent(m), Note: note}
}

func (r Runner) dailyTriage(ctx context.Context, ws string, cfg Config, since time.Time) (Output, error) {
	ms, err := r.mentions(ctx, ws, map[string]any{"status": "new"})
	if err != nil {
		return Output{}, err
	}
	var keep, noise []tools.MentionView
	for _, m := range ms {
		switch {
		case !m.CreatedAt.After(since):
		case m.Score != nil && *m.Score < cfg.NoiseBelow: // unscored is unknown, not noise
			noise = append(noise, m)
		default:
			keep = append(keep, m)
		}
	}
	rank(keep)
	out := Output{}
	for i, m := range keep {
		if i == cfg.TopN {
			break
		}
		out.Items = append(out.Items, mentionItem(m, ""))
	}
	archived := 0
	if cfg.MarkNoise {
		for _, m := range noise {
			if _, err := r.Q.UpdateMentionStatus(ctx, database.UpdateMentionStatusParams{ID: m.ID, WorkspaceID: ws, Status: database.MentionStatusArchived}); err != nil {
				return Output{}, fmt.Errorf("archive noise: %w", err)
			}
			archived++
		}
	}
	out.Summary = fmt.Sprintf("%d new mentions since %s: top %d listed, %d look like noise (score below %.1f)",
		len(keep)+len(noise), since.UTC().Format("Jan 2 15:04 MST"), len(out.Items), len(noise), cfg.NoiseBelow)
	if archived > 0 {
		out.Summary += fmt.Sprintf(", %d archived", archived)
	}
	out.Summary += "."
	return out, nil
}

// leadIntents are the legacy intents that carry the shopping tags
// (buying_intent and seeking_alternative map to buy_signal).
var leadIntents = []string{"buy_signal", "recommendation_ask"}

func (r Runner) leadSweep(ctx context.Context, ws string, cfg Config) (Output, error) {
	seen := map[string]bool{}
	var cands []tools.MentionView
	for _, in := range leadIntents {
		ms, err := r.mentions(ctx, ws, map[string]any{"intent": in})
		if err != nil {
			return Output{}, err
		}
		for _, m := range ms {
			if seen[m.ID] || m.Status == "spam" || m.Status == "archived" || score(m) < cfg.MinScore {
				continue
			}
			seen[m.ID] = true
			cands = append(cands, m)
		}
	}
	rank(cands)
	out := Output{}
	crmOK := cfg.PushToCRM
	for _, m := range cands {
		if len(out.Items) == cfg.MaxLeads {
			break
		}
		lead, created, err := r.ensureLead(ctx, ws, m)
		if err != nil {
			return Output{}, err
		}
		if !created {
			continue
		}
		it := mentionItem(m, "new lead")
		it.LeadID = lead.ID
		if crmOK {
			if err := r.call(ctx, ws, allScopes, "push_to_crm", map[string]string{"lead_id": lead.ID}, nil); err != nil {
				crmOK = false // one failure explains them all; don't retry per lead
				out.Notes = append(out.Notes, "push_to_crm: "+publicMessage(err))
			} else {
				it.Note = "new lead, sent to CRM"
			}
		}
		out.Items = append(out.Items, it)
	}
	out.Summary = fmt.Sprintf("%d buying-signal mentions scored %.1f+; %d new leads created.", len(cands), cfg.MinScore, len(out.Items))
	return out, nil
}

func (r Runner) ensureLead(ctx context.Context, ws string, m tools.MentionView) (database.Lead, bool, error) {
	var mid pgtype.UUID
	if err := mid.Scan(m.ID); err != nil {
		return database.Lead{}, false, fmt.Errorf("mention id: %w", err)
	}
	_, err := r.Q.GetLeadByMention(ctx, database.GetLeadByMentionParams{MentionID: mid, WorkspaceID: ws})
	if err == nil {
		return database.Lead{}, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return database.Lead{}, false, fmt.Errorf("lookup lead: %w", err)
	}
	author := ""
	if m.Author != nil {
		author = *m.Author
	}
	meta, _ := json.Marshal(map[string]any{"source": "automation", "playbook": KindLeadSweep, "relevance": score(m), "intent": intent(m)})
	lead, err := r.Q.CreateLead(ctx, database.CreateLeadParams{
		WorkspaceID: ws, MentionID: mid, Stage: database.LeadStageProspect,
		Username: pgtype.Text{String: author, Valid: author != ""},
		Platform: database.NullPlatformType{PlatformType: database.PlatformType(m.Platform), Valid: m.Platform != ""},
		Tags:     []string{"automation"}, Metadata: meta,
	})
	if err != nil {
		return database.Lead{}, false, fmt.Errorf("create lead: %w", err)
	}
	return lead, true, nil
}

func (r Runner) attributionReview(ctx context.Context, ws string, cfg Config) (Output, error) {
	var report struct {
		Totals struct {
			RepliesPosted int    `json:"replies_posted"`
			Clicks        int    `json:"clicks"`
			Signups       int    `json:"signups"`
			Revenue       string `json:"revenue"`
		} `json:"totals"`
		ByPlatform json.RawMessage `json:"by_platform"`
	}
	if err := r.call(ctx, ws, readScope, "attribution_report", map[string]int{"days": cfg.Days}, &report); err != nil {
		return Output{}, fmt.Errorf("attribution_report: %w", err)
	}
	rows, err := r.Q.AutomationKeywordValue(ctx, database.AutomationKeywordValueParams{WorkspaceID: ws, Days: int32(cfg.Days)})
	if err != nil {
		return Output{}, fmt.Errorf("keyword value: %w", err)
	}
	out := Output{Data: map[string]any{"keywords": rows, "by_platform": report.ByPlatform}}
	for _, k := range rows {
		valuable := k.Clicks > 0 || k.Signups > 0 || k.RevenueCents > 0
		switch {
		case valuable:
			out.Items = append(out.Items, Item{Title: k.Term, Note: fmt.Sprintf("%d mentions, %d replies, %d clicks, %d signups, $%d.%02d",
				k.Mentions, k.RepliesPosted, k.Clicks, k.Signups, k.RevenueCents/100, k.RevenueCents%100)})
		case int(k.Mentions) >= cfg.MinMentions:
			out.Suggestions = append(out.Suggestions, fmt.Sprintf("Mute or narrow keyword %q: %d mentions, no clicks, signups or revenue in %d days.", k.Term, k.Mentions, cfg.Days))
		}
	}
	rev := report.Totals.Revenue
	if rev == "" {
		rev = "$0.00"
	}
	out.Summary = fmt.Sprintf("Last %d days: %s revenue, %d signups and %d clicks from %d posted replies. %d keywords drove value; %d look like noise.",
		cfg.Days, rev, report.Totals.Signups, report.Totals.Clicks, report.Totals.RepliesPosted, len(out.Items), len(out.Suggestions))
	return out, nil
}

// custom runs read-only tool steps in order; user-facing tool errors are
// recorded per step, internal errors fail the run.
func (r Runner) custom(ctx context.Context, ws string, cfg Config) (Output, error) {
	type stepResult struct {
		Tool   string `json:"tool"`
		Result any    `json:"result,omitempty"`
		Error  string `json:"error,omitempty"`
	}
	results := make([]stepResult, 0, len(cfg.Steps))
	out := Output{}
	failed := 0
	for i, s := range cfg.Steps {
		res, err := r.Tools.Call(ctx, ws, readScope, s.Tool, s.Args)
		var te *tools.Error
		switch {
		case errors.As(err, &te), errors.Is(err, tools.ErrUnknownTool):
			failed++
			results = append(results, stepResult{Tool: s.Tool, Error: publicMessage(err)})
			out.Notes = append(out.Notes, fmt.Sprintf("step %d (%s): %s", i+1, s.Tool, publicMessage(err)))
		case err != nil:
			return Output{}, fmt.Errorf("step %d (%s): %w", i+1, s.Tool, err)
		default:
			results = append(results, stepResult{Tool: s.Tool, Result: res})
		}
	}
	out.Data = results
	out.Summary = fmt.Sprintf("Ran %d steps (%d failed).", len(cfg.Steps), failed)
	return out, nil
}

// publicMessage returns a caller-safe message: tool errors verbatim,
// anything else generic (details go to the server log).
func publicMessage(err error) string {
	var te *tools.Error
	switch {
	case errors.As(err, &te):
		return te.Msg
	case errors.Is(err, tools.ErrUnknownTool):
		return "unknown tool"
	}
	return "internal error"
}
