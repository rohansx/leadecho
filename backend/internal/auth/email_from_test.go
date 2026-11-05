package auth

import (
	"testing"

	"github.com/rs/zerolog"
)

// Verification and password-reset mail must go out from the configured,
// Resend-verified sender; a hard-coded unverified domain made Resend reject
// every sign-up email, so new accounts could never verify.
func TestEmailHandlerFromAddress(t *testing.T) {
	h := NewEmailHandler(Config{ResendAPIKey: "re_x", FromEmail: "LeadEcho <digest@mail.leadecho.app>"}, nil, nil, nil, zerolog.Nop())
	if h.mail.from != "LeadEcho <digest@mail.leadecho.app>" {
		t.Errorf("from = %q, want the configured sender", h.mail.from)
	}
	if d := NewEmailHandler(Config{}, nil, nil, nil, zerolog.Nop()); d.mail.from != "LeadEcho <hello@leadecho.io>" {
		t.Errorf("default from = %q", d.mail.from)
	}
}
