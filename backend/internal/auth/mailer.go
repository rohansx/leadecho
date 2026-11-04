package auth

import (
	"context"
	"fmt"
	"html"
)

// mailer sends transactional email through Resend. With no API key it is
// disabled and every send is a no-op (callers decide what that means).
type mailer struct {
	apiKey      string
	frontendURL string
	from        string
}

func (m mailer) enabled() bool { return m.apiKey != "" }

func (m mailer) send(ctx context.Context, to, subject, body string) error {
	return Resend{APIKey: m.apiKey, From: m.from}.Send(ctx, []string{to}, subject, body)
}

func emailLayout(heading, text, buttonLabel, link string) string {
	return fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;max-width:560px;margin:0 auto;padding:40px 24px;color:#0d1117">
  <h1 style="font-size:1.4rem;font-weight:800;margin-bottom:8px">%s</h1>
  <p style="color:#444d56;line-height:1.6">%s</p>
  <p style="margin-top:28px"><a href="%s" style="background:#27c17b;color:#0d1117;padding:12px 24px;border-radius:6px;font-weight:700;text-decoration:none;display:inline-block">%s</a></p>
  <p style="margin-top:28px;font-size:0.8rem;color:#999">If you didn't request this, you can ignore this email.</p>
</div>`, heading, text, html.EscapeString(link), buttonLabel)
}

func (m mailer) welcome(ctx context.Context, name, to string) error {
	return m.send(ctx, to, "Welcome to LeadEcho — let's find your first lead", emailLayout(
		"Welcome to LeadEcho, "+html.EscapeString(name)+"!",
		"Add keywords, describe the problem you solve in a pain-point profile, and install the Chrome extension to start catching buying signals.",
		"Open your inbox →", m.frontendURL+"/inbox"))
}

func (m mailer) verify(ctx context.Context, to, rawToken string) error {
	return m.send(ctx, to, "Verify your LeadEcho email", emailLayout(
		"Confirm your email",
		"Click below to verify your email and finish creating your LeadEcho account. The link expires in 24 hours.",
		"Verify email", m.frontendURL+"/api/v1/auth/verify?token="+rawToken))
}

func (m mailer) reset(ctx context.Context, to, rawToken string) error {
	return m.send(ctx, to, "Reset your LeadEcho password", emailLayout(
		"Reset your password",
		"Click below to choose a new password. The link expires in 1 hour and works once.",
		"Reset password", m.frontendURL+"/reset-password?token="+rawToken))
}
