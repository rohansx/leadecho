package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/auth"
	"leadecho/internal/crmsync"
	"leadecho/internal/database"
	"leadecho/internal/netsafe"
)

// CRMEmitter queues a CRM sync (crmsync.Dispatcher).
type CRMEmitter interface {
	Emit(wsID string, ev crmsync.Event)
}

type actions struct {
	q      *database.Queries
	draft  Drafter
	crm    CRMEmitter
	mail   auth.Resend
	appURL string
	http   *http.Client
}

func newActions(q *database.Queries) *actions {
	return &actions{q: q, http: netsafe.Client(10 * time.Second)}
}

// errSkip marks an action that could not apply (e.g. Slack not connected);
// it is recorded as skipped, not failed.
type errSkip struct{ msg string }

func (e errSkip) Error() string { return e.msg }

func (a *actions) runChain(ctx context.Context, wf database.Workflow, l Lead, chain []ActionConfig) ([]StepResult, int) {
	steps := make([]StepResult, 0, len(chain))
	failed := 0
	leadID := ""
	for i, act := range chain {
		step := StepResult{Step: i, Type: act.Type, Status: "completed"}
		detail, err := a.run(ctx, wf, l, act, &leadID)
		var skip errSkip
		switch {
		case errors.As(err, &skip):
			step.Status, step.Detail = "skipped", skip.msg
		case err != nil:
			step.Status, step.Detail = "failed", clip(err.Error(), 300)
			failed++
		default:
			step.Detail = detail
		}
		step.LeadID = leadID
		steps = append(steps, step)
	}
	return steps, failed
}

func (a *actions) run(ctx context.Context, wf database.Workflow, l Lead, act ActionConfig, leadID *string) (string, error) {
	switch act.Type {
	case ActionNotifySlack, ActionNotifyDiscord, ActionNotifyEmail:
		return a.notify(ctx, wf, l, act.Type)
	case ActionAIDraft:
		if a.draft == nil {
			return "", errSkip{"AI drafting is not configured on this server"}
		}
		if err := a.draft(ctx, l.WorkspaceID, l.MentionID); err != nil {
			return "", fmt.Errorf("draft reply: %w", err)
		}
		return "reply draft ready in the inbox", nil
	case ActionCreateLead:
		id, created, err := a.ensureLead(ctx, l, act.str("tag"))
		*leadID = id
		if err != nil || !created {
			return "already in the pipeline", err
		}
		return "added to the pipeline", nil
	case ActionPushCRM:
		id, _, err := a.ensureLead(ctx, l, "")
		*leadID = id
		if err != nil {
			return "", err
		}
		if a.crm == nil {
			return "", errSkip{"CRM sync is not configured on this server"}
		}
		a.crm.Emit(l.WorkspaceID, crmsync.Event{Type: crmsync.EventLeadCreated, LeadID: id})
		return "queued for CRM sync", nil
	case ActionArchive:
		_, err := a.q.UpdateMentionStatus(ctx, database.UpdateMentionStatusParams{ID: l.MentionID, WorkspaceID: l.WorkspaceID, Status: database.MentionStatusArchived})
		if err != nil {
			return "", fmt.Errorf("archive: %w", err)
		}
		return "archived", nil
	case ActionWebhook:
		return a.webhook(ctx, wf, l, act.str("url"))
	}
	return "", errSkip{fmt.Sprintf("unknown action %q", act.Type)}
}

// ensureLead returns the mention's lead, creating it when missing.
func (a *actions) ensureLead(ctx context.Context, l Lead, tag string) (string, bool, error) {
	existing, err := a.q.GetLeadByMention(ctx, database.GetLeadByMentionParams{MentionID: pgUUID(l.MentionID), WorkspaceID: l.WorkspaceID})
	if err == nil {
		return existing.ID, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("find lead: %w", err)
	}
	tags := []string{"workflow"}
	if tag != "" {
		tags = append(tags, tag)
	}
	meta, _ := json.Marshal(map[string]any{"relevance": l.Score, "intent": l.Intent, "source": "workflow"})
	lead, err := a.q.CreateLead(ctx, database.CreateLeadParams{
		WorkspaceID: l.WorkspaceID,
		MentionID:   pgUUID(l.MentionID),
		Stage:       database.LeadStageProspect,
		Username:    pgtype.Text{String: l.Author, Valid: l.Author != ""},
		Platform:    database.NullPlatformType{PlatformType: database.PlatformType(l.Platform), Valid: l.Platform != ""},
		Tags:        tags,
		Metadata:    meta,
	})
	if err != nil {
		return "", false, fmt.Errorf("create lead: %w", err)
	}
	return lead.ID, true, nil
}

// alertSettings mirrors workspace settings.webhooks (set on the Alerts page).
type alertSettings struct {
	SlackURL   string `json:"slack_url"`
	DiscordURL string `json:"discord_url"`
	EmailTo    string `json:"email_to"`
}

func (a *actions) alerts(ctx context.Context, wsID string) (alertSettings, error) {
	raw, err := a.q.GetWorkspaceSettings(ctx, wsID)
	if err != nil {
		return alertSettings{}, fmt.Errorf("load settings: %w", err)
	}
	var s struct {
		Webhooks alertSettings `json:"webhooks"`
	}
	_ = json.Unmarshal(raw, &s) // missing/odd settings = nothing connected
	return s.Webhooks, nil
}

func (a *actions) notify(ctx context.Context, wf database.Workflow, l Lead, kind ActionType) (string, error) {
	cfg, err := a.alerts(ctx, l.WorkspaceID)
	if err != nil {
		return "", err
	}
	title := l.Title
	if title == "" {
		title = clip(l.Content, 140)
	}
	head := fmt.Sprintf("%s · %s · score %.1f", wf.Name, l.Platform, l.Score)
	switch kind {
	case ActionNotifySlack:
		if cfg.SlackURL == "" {
			return "", errSkip{"Slack isn't connected (Alerts page)"}
		}
		text := fmt.Sprintf("*%s*\n<%s|%s>%s", slackEscape(head), l.URL, slackEscape(title), a.inboxLine(l, true))
		return "sent to Slack", a.postJSON(ctx, cfg.SlackURL, map[string]string{"text": text})
	case ActionNotifyDiscord:
		if cfg.DiscordURL == "" {
			return "", errSkip{"Discord isn't connected (Alerts page)"}
		}
		text := fmt.Sprintf("**%s**\n[%s](<%s>)%s", head, strings.NewReplacer("[", "(", "]", ")").Replace(title), l.URL, a.inboxLine(l, false))
		return "sent to Discord", a.postJSON(ctx, cfg.DiscordURL, map[string]any{"content": clip(text, 1900), "allowed_mentions": map[string]any{"parse": []string{}}})
	default:
		if !a.mail.Enabled() {
			return "", errSkip{"email isn't configured on this server"}
		}
		to := cfg.EmailTo
		if to == "" {
			if to, err = a.q.GetWorkspaceOwnerEmail(ctx, l.WorkspaceID); err != nil {
				return "", errSkip{"no email address to send to"}
			}
		}
		body := fmt.Sprintf(`<div style="font-family:sans-serif;max-width:600px"><p style="color:#666">%s</p><h3><a href="%s">%s</a></h3><p>%s</p>%s</div>`,
			html.EscapeString(head), html.EscapeString(l.URL), html.EscapeString(title), html.EscapeString(clip(l.Content, 600)), a.inboxHTML(l))
		if err := a.mail.Send(ctx, []string{to}, "LeadEcho: "+clip(title, 80), body); err != nil {
			return "", fmt.Errorf("email: %w", err)
		}
		return "emailed " + to, nil
	}
}

func (a *actions) inboxURL(l Lead) string {
	if a.appURL == "" {
		return ""
	}
	return strings.TrimRight(a.appURL, "/") + "/inbox?id=" + l.MentionID
}

func (a *actions) inboxLine(l Lead, slack bool) string {
	u := a.inboxURL(l)
	switch {
	case u == "":
		return ""
	case slack:
		return fmt.Sprintf("\n<%s|Open in LeadEcho>", u)
	default:
		return fmt.Sprintf("\n[Open in LeadEcho](%s)", u)
	}
}

func (a *actions) inboxHTML(l Lead) string {
	if u := a.inboxURL(l); u != "" {
		return fmt.Sprintf(`<p><a href="%s">Open in LeadEcho</a></p>`, html.EscapeString(u))
	}
	return ""
}

func (a *actions) webhook(ctx context.Context, wf database.Workflow, l Lead, url string) (string, error) {
	if err := netsafe.ValidatePublicURL(ctx, url); err != nil {
		return "", fmt.Errorf("webhook url: %w", err)
	}
	// ponytail: unsigned; add an HMAC header (like crmsync's signed webhook) if a receiver needs to verify.
	payload := map[string]any{"event": "workflow.matched", "workflow": map[string]string{"id": wf.ID, "name": wf.Name}, "lead": l}
	if err := a.postJSON(ctx, url, payload); err != nil {
		return "", err
	}
	return "delivered", nil
}

func (a *actions) postJSON(ctx context.Context, url string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LeadEcho-Workflows/1")
	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("receiver answered %d", resp.StatusCode)
	}
	return nil
}

// slackEscape neutralises Slack mrkdwn control characters in untrusted text.
func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
