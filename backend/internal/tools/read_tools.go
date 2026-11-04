package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"leadecho/internal/apikey"
	"leadecho/internal/database"
)

func readTools(d Deps) []Tool {
	return []Tool{
		{
			Name:        "search_mentions",
			Description: "Search or list social mentions found by LeadEcho listeners, newest first. Give at most one of query (full-text), status, intent, platform or tier=leads_ready (high-intent, score >= 6). With none, returns the latest mentions.",
			Scope:       apikey.ScopeRead,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{
"query":{"type":"string","maxLength":500,"description":"Full-text search over title and content"},
"status":{"type":"string","enum":["new","reviewed","replied","archived","spam"]},
"intent":{"type":"string","enum":["buy_signal","complaint","recommendation_ask","comparison","general"]},
"platform":{"type":"string","enum":["reddit","hackernews","devto","lobsters","indiehackers","twitter","linkedin","quora","exa","googlenews"]},
"tier":{"type":"string","enum":["leads_ready"]},
"limit":{"type":"integer","minimum":1,"maximum":100,"default":20},
"offset":{"type":"integer","minimum":0,"default":0}}}`),
			Handler: d.searchMentions,
		},
		{
			Name:        "list_leads",
			Description: "List pipeline leads, newest first, optionally filtered by stage.",
			Scope:       apikey.ScopeRead,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{
"stage":{"type":"string","enum":["prospect","qualified","engaged","converted","lost"]},
"limit":{"type":"integer","minimum":1,"maximum":100,"default":20},
"offset":{"type":"integer","minimum":0,"default":0}}}`),
			Handler: d.listLeads,
		},
		{
			Name:        "get_lead",
			Description: "Get one lead with its source mention and stage history.",
			Scope:       apikey.ScopeRead,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["lead_id"],"properties":{
"lead_id":{"type":"string","format":"uuid"}}}`),
			Handler: d.getLead,
		},
		{
			Name:        "attribution_report",
			Description: "Revenue attribution: replies posted, tracked-link clicks, signups and revenue per source platform for replies created in the last N days.",
			Scope:       apikey.ScopeRead,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{
"days":{"type":"integer","minimum":1,"maximum":365,"default":30}}}`),
			Handler: d.attributionReport,
		},
		{
			Name:        "weekly_summary",
			Description: "Summary of the last N days (default 7): mentions, high-intent buyers, complaints, new leads, replies posted, revenue attributed, top keywords and the best open leads.",
			Scope:       apikey.ScopeRead,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{
"days":{"type":"integer","minimum":1,"maximum":90,"default":7}}}`),
			Handler: d.weeklySummary,
		},
	}
}

type searchMentionsArgs struct {
	Query    string `json:"query"`
	Status   string `json:"status"`
	Intent   string `json:"intent"`
	Platform string `json:"platform"`
	Tier     string `json:"tier"`
	Limit    int32  `json:"limit"`
	Offset   int32  `json:"offset"`
}

func (a *searchMentionsArgs) validate() error {
	a.Query = strings.TrimSpace(a.Query)
	if err := checkLen("query", a.Query, 500); err != nil {
		return err
	}
	switch {
	case a.Status != "" && !mentionStatuses[a.Status]:
		return userErr("invalid status %q", a.Status)
	case a.Intent != "" && !mentionIntents[a.Intent]:
		return userErr("invalid intent %q", a.Intent)
	case a.Platform != "" && !platforms[a.Platform]:
		return userErr("invalid platform %q", a.Platform)
	case a.Tier != "" && a.Tier != "leads_ready":
		return userErr("invalid tier %q", a.Tier)
	}
	n := 0
	for _, f := range []string{a.Query, a.Status, a.Intent, a.Platform, a.Tier} {
		if f != "" {
			n++
		}
	}
	if n > 1 {
		return userErr("give at most one of query, status, intent, platform, tier")
	}
	var err error
	a.Limit, a.Offset, err = pageArgs(a.Limit, a.Offset)
	return err
}

func (d Deps) searchMentions(ctx context.Context, ws string, raw json.RawMessage) (any, error) {
	var a searchMentionsArgs
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if err := a.validate(); err != nil {
		return nil, err
	}
	var ms []database.Mention
	var err error
	switch {
	case a.Query != "":
		ms, err = d.Q.SearchMentions(ctx, database.SearchMentionsParams{WorkspaceID: ws, Query: a.Query, Lim: a.Limit, Off: a.Offset})
	case a.Status != "":
		ms, err = d.Q.ListMentionsByStatus(ctx, database.ListMentionsByStatusParams{WorkspaceID: ws, Status: database.MentionStatus(a.Status), Lim: a.Limit, Off: a.Offset})
	case a.Intent != "":
		ms, err = d.Q.ListMentionsByIntent(ctx, database.ListMentionsByIntentParams{WorkspaceID: ws, Intent: database.NullIntentType{IntentType: database.IntentType(a.Intent), Valid: true}, Lim: a.Limit, Off: a.Offset})
	case a.Platform != "":
		ms, err = d.Q.ListMentionsByPlatform(ctx, database.ListMentionsByPlatformParams{WorkspaceID: ws, Platform: a.Platform, Lim: a.Limit, Off: a.Offset})
	case a.Tier != "":
		ms, err = d.Q.ListMentionsLeadsReady(ctx, database.ListMentionsLeadsReadyParams{WorkspaceID: ws, Lim: a.Limit, Off: a.Offset})
	default:
		ms, err = d.Q.ListMentions(ctx, database.ListMentionsParams{WorkspaceID: ws, Lim: a.Limit, Off: a.Offset})
	}
	if err != nil {
		return nil, fmt.Errorf("search mentions: %w", err)
	}
	return map[string]any{"mentions": mentionViews(ms), "count": len(ms), "limit": a.Limit, "offset": a.Offset}, nil
}

type listLeadsArgs struct {
	Stage  string `json:"stage"`
	Limit  int32  `json:"limit"`
	Offset int32  `json:"offset"`
}

func (d Deps) listLeads(ctx context.Context, ws string, raw json.RawMessage) (any, error) {
	var a listLeadsArgs
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if a.Stage != "" && !leadStages[a.Stage] {
		return nil, userErr("invalid stage %q", a.Stage)
	}
	lim, off, err := pageArgs(a.Limit, a.Offset)
	if err != nil {
		return nil, err
	}
	var ls []database.Lead
	if a.Stage != "" {
		ls, err = d.Q.ListLeadsByStage(ctx, database.ListLeadsByStageParams{WorkspaceID: ws, Stage: database.LeadStage(a.Stage), Lim: lim, Off: off})
	} else {
		ls, err = d.Q.ListLeads(ctx, database.ListLeadsParams{WorkspaceID: ws, Lim: lim, Off: off})
	}
	if err != nil {
		return nil, fmt.Errorf("list leads: %w", err)
	}
	views := make([]LeadView, len(ls))
	for i, l := range ls {
		views[i] = leadView(l)
	}
	return map[string]any{"leads": views, "count": len(views), "limit": lim, "offset": off}, nil
}

type leadIDArgs struct {
	LeadID string `json:"lead_id"`
}

func (d Deps) loadLead(ctx context.Context, ws string, raw json.RawMessage) (database.Lead, error) {
	var a leadIDArgs
	if err := decode(raw, &a); err != nil {
		return database.Lead{}, err
	}
	if err := requireUUID("lead_id", a.LeadID); err != nil {
		return database.Lead{}, err
	}
	l, err := d.Q.GetLead(ctx, database.GetLeadParams{ID: a.LeadID, WorkspaceID: ws})
	return l, notFound(err, "lead")
}

func (d Deps) getLead(ctx context.Context, ws string, raw json.RawMessage) (any, error) {
	l, err := d.loadLead(ctx, ws, raw)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"lead": leadView(l)}
	if id := uuidStr(l.MentionID); id != nil {
		if m, err := d.Q.GetMention(ctx, database.GetMentionParams{ID: *id, WorkspaceID: ws}); err == nil {
			out["mention"] = mentionView(m)
		}
	}
	events, err := d.Q.ListLeadEvents(ctx, l.ID) // lead already verified in ws
	if err != nil {
		return nil, fmt.Errorf("lead events: %w", err)
	}
	type step struct {
		From string `json:"from,omitempty"`
		To   string `json:"to"`
		At   string `json:"at"`
	}
	hist := make([]step, len(events))
	for i, e := range events {
		hist[i] = step{From: string(e.PreviousStage.LeadStage), To: string(e.NewStage), At: e.CreatedAt.Format("2006-01-02T15:04:05Z07:00")}
	}
	out["history"] = hist
	return out, nil
}

type daysArgs struct {
	Days *int32 `json:"days"`
}

func parseDays(raw json.RawMessage, def, max int32) (int32, error) {
	var a daysArgs
	if err := decode(raw, &a); err != nil {
		return 0, err
	}
	if a.Days == nil {
		return def, nil
	}
	if *a.Days < 1 || *a.Days > max {
		return 0, userErr("days must be between 1 and %d", max)
	}
	return *a.Days, nil
}

func (d Deps) attributionReport(ctx context.Context, ws string, raw json.RawMessage) (any, error) {
	days, err := parseDays(raw, 30, 365)
	if err != nil {
		return nil, err
	}
	rows, err := d.Q.ToolsAttributionByPlatform(ctx, database.ToolsAttributionByPlatformParams{WorkspaceID: ws, Days: days})
	if err != nil {
		return nil, fmt.Errorf("attribution report: %w", err)
	}
	var tot database.ToolsAttributionByPlatformRow
	for _, r := range rows {
		tot.RepliesPosted += r.RepliesPosted
		tot.Clicks += r.Clicks
		tot.Signups += r.Signups
		tot.RevenueCents += r.RevenueCents
	}
	return map[string]any{
		"days":        days,
		"by_platform": rows,
		"totals": map[string]any{
			"replies_posted": tot.RepliesPosted, "clicks": tot.Clicks, "signups": tot.Signups,
			"revenue_cents": tot.RevenueCents, "revenue": dollars(tot.RevenueCents),
		},
	}, nil
}

func (d Deps) weeklySummary(ctx context.Context, ws string, raw json.RawMessage) (any, error) {
	days, err := parseDays(raw, 7, 90)
	if err != nil {
		return nil, err
	}
	s, err := d.Q.ToolsWorkspaceSummary(ctx, database.ToolsWorkspaceSummaryParams{WorkspaceID: ws, Days: days})
	if err != nil {
		return nil, fmt.Errorf("summary: %w", err)
	}
	top, err := d.Q.TopKeywords(ctx, ws) // ponytail: fixed 30-day window from the analytics query
	if err != nil {
		return nil, fmt.Errorf("top keywords: %w", err)
	}
	best, err := d.Q.ListMentionsLeadsReady(ctx, database.ListMentionsLeadsReadyParams{WorkspaceID: ws, Lim: 5})
	if err != nil {
		return nil, fmt.Errorf("best leads: %w", err)
	}
	headline := fmt.Sprintf("Last %d days: %d mentions, %d high-intent buyers, %d complaints, %d new leads, %d replies posted, %s attributed.",
		days, s.Mentions, s.HighIntent, s.Complaints, s.NewLeads, s.RepliesPosted, dollars(s.RevenueCents))
	return map[string]any{
		"days": days, "headline": headline, "stats": s,
		"top_keywords_30d": top, "best_open_mentions": mentionViews(best),
	}, nil
}

func dollars(cents int64) string { return fmt.Sprintf("$%d.%02d", cents/100, cents%100) }
