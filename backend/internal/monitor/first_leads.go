package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// One email per workspace, when its first lead is stored: "your first leads
// are ready". A new customer may have closed the tab while the first scan ran.

const (
	// firstLeadsEmailKey is the workspaces.settings key written, once, when
	// the email is sent or ruled out for good. While it is absent the email is
	// still owed.
	firstLeadsEmailKey = "first_leads_email"
	// A workspace that finished onboarding before the email existed gets it
	// only if it is this new and its leads are this recent: everyone else has
	// been reading their leads for a while.
	firstLeadsNewWorkspace = 7 * 24 * time.Hour
	firstLeadsRecentLead   = 24 * time.Hour
)

// Mailer sends one email (auth.Resend).
type Mailer interface {
	Enabled() bool
	Send(ctx context.Context, to []string, subject, html string) error
}

// SetMail turns the first-leads email on. appURL is where its inbox link goes.
func (m *Monitor) SetMail(mail Mailer, appURL string) {
	m.mail, m.appURL = mail, strings.TrimRight(appURL, "/")
}

// firstLeadsOwed reports whether the workspace should still get the email.
// Not when it was sent (or ruled out) already. Not when the workspace was
// using LeadEcho before the email existed: completed_at is only written since
// then, so a workspace without it qualifies only while new and without older
// leads (the constants above). oldestLead is zero for a workspace without a lead.
func firstLeadsOwed(settings []byte, created, oldestLead, now time.Time) bool {
	var s struct {
		Done       json.RawMessage `json:"first_leads_email"`
		Onboarding struct {
			CompletedAt time.Time `json:"completed_at"`
		} `json:"onboarding"`
	}
	if json.Unmarshal(settings, &s) != nil || len(s.Done) > 0 {
		return false
	}
	return !s.Onboarding.CompletedAt.IsZero() ||
		(now.Sub(created) <= firstLeadsNewWorkspace && (oldestLead.IsZero() || now.Sub(oldestLead) <= firstLeadsRecentLead))
}

// FirstLeadsEmailTo is the address the first-leads email will go to, or ""
// when it will not be sent: it is not owed, or there is nobody to write to.
// The address is the one alert emails use, else the owner's (as the digest does).
func FirstLeadsEmailTo(ctx context.Context, q *database.Queries, ws database.Workspace, oldestLead, now time.Time) string {
	if !firstLeadsOwed(ws.Settings, ws.CreatedAt, oldestLead, now) {
		return ""
	}
	var s struct {
		Webhooks webhookConfig `json:"webhooks"`
	}
	_ = json.Unmarshal(ws.Settings, &s)
	if s.Webhooks.EmailTo != "" {
		return s.Webhooks.EmailTo
	}
	owner, err := q.GetWorkspaceOwnerEmail(ctx, ws.ID)
	if err != nil {
		return ""
	}
	return owner
}

// notifyFirstLeads sends the email if this workspace has its first leads and
// has not been told. Called after anything that can store a lead; cheap when
// there is nothing to do.
func (m *Monitor) notifyFirstLeads(ctx context.Context, wsID string) {
	if m.mail == nil || !m.mail.Enabled() {
		return
	}
	ws, err := m.q.GetWorkspace(ctx, wsID)
	if err != nil || strings.Contains(string(ws.Settings), `"`+firstLeadsEmailKey+`"`) {
		return
	}
	now := time.Now().UTC()
	// ruledOut records that this workspace never gets the email, so an
	// existing customer costs one query here from then on.
	ruledOut := func() {
		_, _ = m.q.SwapWorkspaceSettingsKey(ctx, wsID, firstLeadsEmailKey, nil, []byte(`{"skipped":true}`))
	}
	if !firstLeadsOwed(ws.Settings, ws.CreatedAt, time.Time{}, now) {
		ruledOut()
		return
	}
	counts, err := m.q.CountSetup(ctx, wsID)
	total := counts.PostLeads + counts.CompanyLeads
	if err != nil || total == 0 {
		return
	}
	if !firstLeadsOwed(ws.Settings, ws.CreatedAt, counts.OldestLead, now) {
		ruledOut()
		return
	}
	to := FirstLeadsEmailTo(ctx, m.q, ws, counts.OldestLead, now)
	if to == "" {
		return // nobody to write to yet
	}
	top, err := m.q.ListMentionsLeadsReady(ctx, database.ListMentionsLeadsReadyParams{WorkspaceID: wsID, Lim: 2})
	if err != nil {
		return
	}
	// Recorded before sending, and only if nobody else has: two monitor
	// processes can get here together, and a failed send must not be retried
	// into a second email.
	claimed, err := m.q.SwapWorkspaceSettingsKey(ctx, wsID, firstLeadsEmailKey, nil, jsonBytes(map[string]any{"sent_at": now, "leads": total}))
	if err != nil || !claimed {
		return
	}
	subject, body := firstLeadsEmail(total, top, m.appURL)
	sentAt := pgtype.Timestamptz{Time: now, Valid: true}
	if err := m.mail.Send(ctx, []string{to}, subject, body); err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("notify: first leads email failed")
		sentAt = pgtype.Timestamptz{}
	}
	_, _ = m.q.CreateNotification(ctx, database.CreateNotificationParams{
		WorkspaceID: wsID, Channel: database.NotificationChannelEmail, Recipient: to,
		Subject: pgtype.Text{String: subject, Valid: true}, Body: body, Metadata: []byte(`{}`), SentAt: sentAt,
	})
}

// firstLeadsEmail is the message: how many, the best one or two as a line
// each, and the way to the inbox. A line is the lead's title (a post's
// headline, or "Acme is hiring: ..."), never its text: a company lead's text
// names the contact.
func firstLeadsEmail(total int, top []database.Mention, appURL string) (subject, body string) {
	headline := fmt.Sprintf("Your first %d leads are ready", total)
	if total == 1 {
		headline = "Your first lead is ready"
	}
	var b strings.Builder
	b.WriteString(`<div style="font-family:sans-serif;max-width:600px">`)
	fmt.Fprintf(&b, `<h2 style="margin:0 0 16px">%s</h2>`, headline)
	for _, m := range top {
		line := m.Title.String
		if line == "" {
			line = "A post by " + m.AuthorUsername.String
		}
		if short := truncateRunes(line, 117); short != line {
			line = short + "..."
		}
		fmt.Fprintf(&b, `<div style="border-left:3px solid #6366f1;padding:8px 12px;margin:8px 0"><strong>[%s]</strong> %s</div>`,
			html.EscapeString(m.Platform), html.EscapeString(line))
	}
	if total > len(top) {
		fmt.Fprintf(&b, `<p style="color:#666">and %d more.</p>`, total-len(top))
	}
	fmt.Fprintf(&b, `<p><a href="%s/inbox">Open your inbox</a>. Replies are saved as drafts; nothing is sent for you.</p></div>`, html.EscapeString(appURL))
	return "LeadEcho: " + strings.ToLower(headline[:1]) + headline[1:], b.String()
}
