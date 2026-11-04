package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// deliver posts the output as an in-app notification and, when the
// automation asks for it, emails the workspace owner. Delivery problems are
// returned as notes; they never fail the run.
func (s *Service) deliver(ctx context.Context, a database.Automation, out Output, status string) []string {
	var notes []string
	subject := fmt.Sprintf("Automation %q", a.Name)
	if status == "failed" {
		subject += " failed"
	}
	body := RenderText(out)
	meta, _ := json.Marshal(map[string]string{"kind": "automation", "automation_id": a.ID, "automation_kind": a.Kind, "status": status})
	if err := s.notify(ctx, a.WorkspaceID, database.NotificationChannelInApp, "workspace", subject, body, meta, true); err != nil {
		s.logger.Warn().Err(err).Str("automation_id", a.ID).Msg("automations: in-app notification failed")
		notes = append(notes, "in-app notification failed")
	}
	cfg, _ := ConfigOf(a)
	if !cfg.Email {
		return notes
	}
	if !s.mail.Enabled() {
		return append(notes, "email: RESEND_API_KEY not set")
	}
	to, err := s.q.GetWorkspaceOwnerEmail(ctx, a.WorkspaceID)
	if err != nil {
		return append(notes, "email: no recipient")
	}
	html, err := renderHTML(a.Name, out, s.frontendURL+"/automations")
	if err != nil {
		return append(notes, "email: render failed")
	}
	sendErr := s.mail.Send(ctx, []string{to}, "LeadEcho: "+subject, html)
	if err := s.notify(ctx, a.WorkspaceID, database.NotificationChannelEmail, to, subject, body, meta, sendErr == nil); err != nil {
		s.logger.Warn().Err(err).Msg("automations: email notification log failed")
	}
	if sendErr != nil {
		s.logger.Warn().Err(sendErr).Str("automation_id", a.ID).Msg("automations: email failed")
		return append(notes, "email: send failed")
	}
	return notes
}

func (s *Service) notify(ctx context.Context, ws string, ch database.NotificationChannel, recipient, subject, body string, meta []byte, sent bool) error {
	sentAt := pgtype.Timestamptz{}
	if sent {
		sentAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
	}
	_, err := s.q.CreateNotification(ctx, database.CreateNotificationParams{
		WorkspaceID: ws, Channel: ch, Recipient: recipient,
		Subject: pgtype.Text{String: subject, Valid: true}, Body: body, Metadata: meta, SentAt: sentAt,
	})
	return err
}

// RenderText is the plain-text form of an output (notification body).
func RenderText(o Output) string {
	var b strings.Builder
	b.WriteString(o.Summary)
	for _, it := range o.Items {
		b.WriteString("\n- " + it.Title)
		if it.Score != nil {
			fmt.Fprintf(&b, " (score %.1f)", *it.Score)
		}
		if it.Note != "" {
			b.WriteString(" — " + it.Note)
		}
		if it.URL != "" {
			b.WriteString(" " + it.URL)
		}
	}
	for _, sg := range o.Suggestions {
		b.WriteString("\n* " + sg)
	}
	for _, n := range o.Notes {
		b.WriteString("\n! " + n)
	}
	return b.String()
}

var emailTmpl = template.Must(template.New("automation").Funcs(template.FuncMap{"deref": func(f *float32) float32 { return *f }}).Parse(`<!doctype html><html><body style="font-family:system-ui,sans-serif;color:#111;max-width:640px">
<h2 style="font-size:18px">{{.Name}}</h2>
<p>{{.Out.Summary}}</p>
{{if .Out.Items}}<ul>{{range .Out.Items}}<li>{{if .URL}}<a href="{{.URL}}">{{.Title}}</a>{{else}}{{.Title}}{{end}}{{if .Score}} (score {{printf "%.1f" (deref .Score)}}){{end}}{{if .Note}} — {{.Note}}{{end}}</li>{{end}}</ul>{{end}}
{{if .Out.Suggestions}}<p><strong>Suggestions</strong></p><ul>{{range .Out.Suggestions}}<li>{{.}}</li>{{end}}</ul>{{end}}
<p><a href="{{.Link}}">Open automations in LeadEcho</a></p>
</body></html>`))

func renderHTML(name string, o Output, link string) (string, error) {
	var b bytes.Buffer
	err := emailTmpl.Execute(&b, map[string]any{"Name": name, "Out": o, "Link": link})
	return b.String(), err
}
