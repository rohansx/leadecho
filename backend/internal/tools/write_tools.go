package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/apikey"
	"leadecho/internal/crmsync"
	"leadecho/internal/database"
)

const (
	maxKeywords   = 25
	maxKeywordLen = 100
	maxIntentLen  = 2000
	maxNameLen    = 200
	maxReplyLen   = 10000
)

var platformEnum = `{"type":"string","enum":["reddit","hackernews","devto","lobsters","indiehackers","twitter","linkedin","quora","exa","googlenews"]}`

func writeTools(d Deps) []Tool {
	return []Tool{
		{
			Name:        "create_listener",
			Description: "Create a listener (monitoring profile): what buyers it looks for, what to skip, and the keywords to monitor on the given platforms.",
			Scope:       apikey.ScopeWrite,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["name"],"properties":{
"name":{"type":"string","maxLength":200},
"description":{"type":"string","maxLength":2000},
"looks_for":{"type":"string","maxLength":2000,"description":"Plain-English description of posts worth surfacing"},
"skips":{"type":"string","maxLength":2000,"description":"Plain-English description of posts to ignore"},
"keywords":{"type":"array","maxItems":25,"items":{"type":"string","maxLength":100}},
"platforms":{"type":"array","items":` + platformEnum + `,"description":"Platforms for the keywords (default reddit, hackernews, twitter, linkedin)"}}}`),
			Handler: d.createListener,
		},
		{
			Name:        "tune_listener",
			Description: "Tune a listener: replace looks_for / skips (omitted fields are kept) and add or remove keywords.",
			Scope:       apikey.ScopeWrite,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["listener_id"],"properties":{
"listener_id":{"type":"string","format":"uuid"},
"looks_for":{"type":"string","maxLength":2000},
"skips":{"type":"string","maxLength":2000},
"add_keywords":{"type":"array","maxItems":25,"items":{"type":"string","maxLength":100}},
"remove_keywords":{"type":"array","maxItems":25,"items":{"type":"string","maxLength":100}},
"platforms":{"type":"array","items":` + platformEnum + `,"description":"Platforms for added keywords"}}}`),
			Handler: d.tuneListener,
		},
		{
			Name:        "mute",
			Description: "Mute an author, domain or topic so future mentions from it are hidden.",
			Scope:       apikey.ScopeWrite,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["kind","value"],"properties":{
"kind":{"type":"string","enum":["author","domain","topic"]},
"value":{"type":"string","minLength":1,"maxLength":200},
"platform":` + platformEnum + `}}`),
			Handler: d.mute,
		},
		{
			Name:        "draft_reply",
			Description: "Draft a knowledge-base-grounded reply to a mention. Returns should_reply=false with a reason when replying is not advisable.",
			Scope:       apikey.ScopeWrite,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["mention_id"],"properties":{
"mention_id":{"type":"string","format":"uuid"}}}`),
			Handler: d.draftReply,
		},
		{
			Name:        "queue_reply",
			Description: "Approve a drafted reply (optionally replacing its text) and put it in the posting queue used by the LeadEcho browser extension.",
			Scope:       apikey.ScopeWrite,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["reply_id"],"properties":{
"reply_id":{"type":"string","format":"uuid"},
"content":{"type":"string","minLength":1,"maxLength":10000,"description":"Final text; omit to keep the draft"}}}`),
			Handler: d.queueReply,
		},
		{
			Name:        "push_to_crm",
			Description: "Sync a lead to the workspace's connected CRM (HubSpot / Attio) and signed webhook. Asynchronous.",
			Scope:       apikey.ScopeWrite,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["lead_id"],"properties":{
"lead_id":{"type":"string","format":"uuid"}}}`),
			Handler: d.pushToCRM,
		},
		{
			Name:        "set_stage",
			Description: "Move a lead to a pipeline stage; the change is recorded in its history and synced to the CRM.",
			Scope:       apikey.ScopeWrite,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["lead_id","stage"],"properties":{
"lead_id":{"type":"string","format":"uuid"},
"stage":{"type":"string","enum":["prospect","qualified","engaged","converted","lost"]}}}`),
			Handler: d.setStage,
		},
	}
}

func validPlatforms(ps []string) ([]string, error) {
	if len(ps) == 0 {
		return defaultPlatforms, nil
	}
	for _, p := range ps {
		if !platforms[p] {
			return nil, userErr("invalid platform %q", p)
		}
	}
	return ps, nil
}

func validKeywords(field string, in []string) ([]string, error) {
	out := cleanTerms(in)
	if len(out) > maxKeywords {
		return nil, userErr("%s: at most %d keywords", field, maxKeywords)
	}
	for _, k := range out {
		if err := checkLen("keyword", k, maxKeywordLen); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// addKeywords creates keywords on a listener; duplicates are reported, not fatal.
func (d Deps) addKeywords(ctx context.Context, ws, profileID string, terms, plats []string) (added, skipped []string, err error) {
	added, skipped = []string{}, []string{}
	for _, t := range terms {
		_, err := d.Q.CreateKeyword(ctx, database.CreateKeywordParams{
			WorkspaceID: ws, ProfileID: profileID, Term: t, Platforms: plats, IsActive: true,
			MatchType: "contains", NegativeTerms: []string{}, Subreddits: []string{},
		})
		switch {
		case err == nil:
			added = append(added, t)
		case isUniqueViolation(err):
			skipped = append(skipped, t)
		default:
			return nil, nil, fmt.Errorf("create keyword: %w", err)
		}
	}
	return added, skipped, nil
}

type createListenerArgs struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	LooksFor    string   `json:"looks_for"`
	Skips       string   `json:"skips"`
	Keywords    []string `json:"keywords"`
	Platforms   []string `json:"platforms"`
}

func (a *createListenerArgs) validate() error {
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		return userErr("name is required")
	}
	for _, f := range []struct {
		name, v string
		max     int
	}{{"name", a.Name, maxNameLen}, {"description", a.Description, maxIntentLen}, {"looks_for", a.LooksFor, maxIntentLen}, {"skips", a.Skips, maxIntentLen}} {
		if err := checkLen(f.name, f.v, f.max); err != nil {
			return err
		}
	}
	var err error
	if a.Platforms, err = validPlatforms(a.Platforms); err != nil {
		return err
	}
	a.Keywords, err = validKeywords("keywords", a.Keywords)
	return err
}

func (d Deps) createListener(ctx context.Context, ws string, raw json.RawMessage) (any, error) {
	var a createListenerArgs
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if err := a.validate(); err != nil {
		return nil, err
	}
	p, err := d.Q.CreateMonitoringProfile(ctx, database.CreateMonitoringProfileParams{WorkspaceID: ws, Name: a.Name, Description: a.Description, IsActive: true})
	if err != nil {
		return nil, fmt.Errorf("create listener: %w", err)
	}
	if a.LooksFor != "" || a.Skips != "" {
		if p, err = d.Q.ToolsUpdateListenerIntent(ctx, database.ToolsUpdateListenerIntentParams{LooksFor: a.LooksFor, Skips: a.Skips, ID: p.ID, WorkspaceID: ws}); err != nil {
			return nil, fmt.Errorf("save listener intent: %w", err)
		}
	}
	added, skipped, err := d.addKeywords(ctx, ws, p.ID, a.Keywords, a.Platforms)
	if err != nil {
		return nil, err
	}
	return map[string]any{"listener": listenerView(p), "keywords": added, "skipped_keywords": skipped}, nil
}

type tuneListenerArgs struct {
	ListenerID     string   `json:"listener_id"`
	LooksFor       *string  `json:"looks_for"`
	Skips          *string  `json:"skips"`
	AddKeywords    []string `json:"add_keywords"`
	RemoveKeywords []string `json:"remove_keywords"`
	Platforms      []string `json:"platforms"`
}

func (a *tuneListenerArgs) validate() error {
	if err := requireUUID("listener_id", a.ListenerID); err != nil {
		return err
	}
	if a.LooksFor == nil && a.Skips == nil && len(a.AddKeywords) == 0 && len(a.RemoveKeywords) == 0 {
		return userErr("nothing to change: give looks_for, skips, add_keywords or remove_keywords")
	}
	if a.LooksFor != nil {
		if err := checkLen("looks_for", *a.LooksFor, maxIntentLen); err != nil {
			return err
		}
	}
	if a.Skips != nil {
		if err := checkLen("skips", *a.Skips, maxIntentLen); err != nil {
			return err
		}
	}
	var err error
	if a.Platforms, err = validPlatforms(a.Platforms); err != nil {
		return err
	}
	if a.AddKeywords, err = validKeywords("add_keywords", a.AddKeywords); err != nil {
		return err
	}
	a.RemoveKeywords, err = validKeywords("remove_keywords", a.RemoveKeywords)
	return err
}

func (d Deps) tuneListener(ctx context.Context, ws string, raw json.RawMessage) (any, error) {
	var a tuneListenerArgs
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if err := a.validate(); err != nil {
		return nil, err
	}
	p, err := d.Q.GetMonitoringProfile(ctx, database.GetMonitoringProfileParams{ID: a.ListenerID, WorkspaceID: ws})
	if err != nil {
		return nil, notFound(err, "listener")
	}
	if a.LooksFor != nil || a.Skips != nil {
		lf, sk := p.LooksFor, p.Skips
		if a.LooksFor != nil {
			lf = strings.TrimSpace(*a.LooksFor)
		}
		if a.Skips != nil {
			sk = strings.TrimSpace(*a.Skips)
		}
		if p, err = d.Q.ToolsUpdateListenerIntent(ctx, database.ToolsUpdateListenerIntentParams{LooksFor: lf, Skips: sk, ID: p.ID, WorkspaceID: ws}); err != nil {
			return nil, fmt.Errorf("update listener intent: %w", err)
		}
	}
	removed, err := d.removeKeywords(ctx, ws, p.ID, a.RemoveKeywords)
	if err != nil {
		return nil, err
	}
	added, skipped, err := d.addKeywords(ctx, ws, p.ID, a.AddKeywords, a.Platforms)
	if err != nil {
		return nil, err
	}
	return map[string]any{"listener": listenerView(p), "added_keywords": added, "skipped_keywords": skipped, "removed_keywords": removed}, nil
}

func (d Deps) removeKeywords(ctx context.Context, ws, profileID string, terms []string) (int, error) {
	if len(terms) == 0 {
		return 0, nil
	}
	drop := map[string]bool{}
	for _, t := range terms {
		drop[strings.ToLower(t)] = true
	}
	kws, err := d.Q.ListKeywords(ctx, ws)
	if err != nil {
		return 0, fmt.Errorf("list keywords: %w", err)
	}
	n := 0
	for _, k := range kws {
		if k.ProfileID != profileID || !drop[strings.ToLower(k.Term)] {
			continue
		}
		if err := d.Q.DeleteKeyword(ctx, database.DeleteKeywordParams{ID: k.ID, WorkspaceID: ws}); err != nil {
			return n, fmt.Errorf("delete keyword: %w", err)
		}
		n++
	}
	return n, nil
}

func (d Deps) mute(ctx context.Context, ws string, raw json.RawMessage) (any, error) {
	var m Mute
	if err := decode(raw, &m); err != nil {
		return nil, err
	}
	m.Value = strings.TrimSpace(m.Value)
	switch {
	case m.Kind != "author" && m.Kind != "domain" && m.Kind != "topic":
		return nil, userErr("kind must be author, domain or topic")
	case m.Value == "":
		return nil, userErr("value is required")
	case m.Platform != "" && !platforms[m.Platform]:
		return nil, userErr("invalid platform %q", m.Platform)
	}
	if err := checkLen("value", m.Value, maxNameLen); err != nil {
		return nil, err
	}
	if d.Mutes == nil {
		return map[string]any{"muted": false, "mute": m, "note": "mute lists are not enabled on this server yet; nothing was stored"}, nil
	}
	if err := d.Mutes.Mute(ctx, ws, m); err != nil {
		return nil, fmt.Errorf("mute: %w", err)
	}
	return map[string]any{"muted": true, "mute": m}, nil
}

type mentionIDArgs struct {
	MentionID string `json:"mention_id"`
}

func (d Deps) draftReply(ctx context.Context, ws string, raw json.RawMessage) (any, error) {
	var a mentionIDArgs
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if err := requireUUID("mention_id", a.MentionID); err != nil {
		return nil, err
	}
	if d.Drafter == nil {
		return nil, userErr("reply drafting is not available on this server")
	}
	res, err := d.Drafter.DraftForMention(ctx, ws, a.MentionID)
	if err != nil {
		return nil, notFound(err, "mention")
	}
	out := map[string]any{"should_reply": res.ShouldReply, "awareness_level": res.AwarenessLevel}
	if !res.ShouldReply {
		out["reason"] = res.Reason
		return out, nil
	}
	out["reply"] = replyView(res.Reply)
	out["template_style"] = res.TemplateStyle
	return out, nil
}

type queueReplyArgs struct {
	ReplyID string  `json:"reply_id"`
	Content *string `json:"content"`
}

func (d Deps) queueReply(ctx context.Context, ws string, raw json.RawMessage) (any, error) {
	var a queueReplyArgs
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if err := requireUUID("reply_id", a.ReplyID); err != nil {
		return nil, err
	}
	if a.Content != nil {
		c := strings.TrimSpace(*a.Content)
		if c == "" {
			return nil, userErr("content must not be empty")
		}
		if err := checkLen("content", c, maxReplyLen); err != nil {
			return nil, err
		}
		a.Content = &c
	}
	r, err := d.Q.GetReply(ctx, database.GetReplyParams{ID: a.ReplyID, WorkspaceID: ws})
	if err != nil {
		return nil, notFound(err, "reply")
	}
	if r.Status == database.ReplyStatusPosted {
		return nil, userErr("reply is already posted")
	}
	edited := r.EditedContent
	if a.Content != nil {
		edited = pgtype.Text{String: *a.Content, Valid: true}
	}
	// ponytail: keeps any existing tracking link; minting a new UTM link stays in the dashboard flow.
	r, err = d.Q.ApproveReply(ctx, database.ApproveReplyParams{ID: r.ID, WorkspaceID: ws, UtmLinkID: r.UtmLinkID, EditedContent: edited})
	if err != nil {
		return nil, fmt.Errorf("approve reply: %w", err)
	}
	return map[string]any{"reply": replyView(r), "queued": true}, nil
}

func (d Deps) pushToCRM(ctx context.Context, ws string, raw json.RawMessage) (any, error) {
	l, err := d.loadLead(ctx, ws, raw)
	if err != nil {
		return nil, err
	}
	if d.CRM == nil {
		return nil, userErr("CRM sync is not configured on this server")
	}
	// lead.created upserts: the sinks create the contact once, then update it.
	d.CRM.Emit(ws, crmsync.Event{Type: crmsync.EventLeadCreated, LeadID: l.ID})
	return map[string]any{"queued": true, "lead_id": l.ID}, nil
}

type setStageArgs struct {
	LeadID string `json:"lead_id"`
	Stage  string `json:"stage"`
}

func (d Deps) setStage(ctx context.Context, ws string, raw json.RawMessage) (any, error) {
	var a setStageArgs
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if err := requireUUID("lead_id", a.LeadID); err != nil {
		return nil, err
	}
	if !leadStages[a.Stage] {
		return nil, userErr("invalid stage %q", a.Stage)
	}
	prev, err := d.Q.GetLead(ctx, database.GetLeadParams{ID: a.LeadID, WorkspaceID: ws})
	if err != nil {
		return nil, notFound(err, "lead")
	}
	l, err := d.Q.UpdateLeadStage(ctx, database.UpdateLeadStageParams{Stage: database.LeadStage(a.Stage), ID: a.LeadID, WorkspaceID: ws})
	if err != nil {
		return nil, notFound(err, "lead")
	}
	if prev.Stage != l.Stage {
		if _, err := d.Q.CreateLeadEvent(ctx, database.CreateLeadEventParams{
			LeadID: l.ID, PreviousStage: database.NullLeadStage{LeadStage: prev.Stage, Valid: true},
			NewStage: l.Stage, Notes: pgtype.Text{String: "stage_updated_via_api", Valid: true},
		}); err != nil {
			return nil, fmt.Errorf("record stage change: %w", err)
		}
		if d.CRM != nil {
			d.CRM.Emit(ws, crmsync.Event{Type: crmsync.EventLeadStageChanged, LeadID: l.ID, PrevStage: string(prev.Stage)})
		}
	}
	return map[string]any{"lead": leadView(l), "previous_stage": string(prev.Stage)}, nil
}
