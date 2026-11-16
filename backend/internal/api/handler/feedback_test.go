package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/auth"
	"leadecho/internal/lobby"
)

type fakeFeedbackLobby struct {
	tokens map[string]string // email → token; missing = feedback off
	err    error
	calls  int
}

func (f *fakeFeedbackLobby) ReporterToken(_ context.Context, email string) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	if t, ok := f.tokens[email]; ok {
		return t, nil
	}
	return "", lobby.ErrFeedbackOff
}

func (f *fakeFeedbackLobby) PublicKey(context.Context) (string, error) { return "pk_test", nil }

func widgetFor(h *FeedbackHandler, email string) feedbackWidget {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feedback/widget", nil)
	if email != "" {
		req = req.WithContext(middleware.WithClaims(req.Context(), &auth.Claims{Email: email}))
	}
	w := httptest.NewRecorder()
	h.Widget(w, req)
	var out feedbackWidget
	_ = json.NewDecoder(w.Body).Decode(&out)
	return out
}

func TestFeedbackWidget(t *testing.T) {
	fl := &fakeFeedbackLobby{tokens: map[string]string{"pat@acme.com": "rt_pat"}}
	h := &FeedbackHandler{lobby: fl, logger: zerolog.Nop(), reporters: map[string]cachedReporter{}}

	if got := widgetFor(h, "Pat@Acme.com"); !got.Enabled || got.Project != "pk_test" || got.Reporter != "rt_pat" {
		t.Fatalf("tester: %+v", got)
	}
	if got := widgetFor(h, "nobody@x.test"); got.Enabled || got.Reporter != "" {
		t.Fatalf("not on the waitlist must get no widget: %+v", got)
	}
	if got := widgetFor(h, ""); got.Enabled {
		t.Fatal("no session, no widget")
	}
	// Both answers are cached: no extra Lobby calls.
	widgetFor(h, "pat@acme.com")
	widgetFor(h, "nobody@x.test")
	if fl.calls != 2 {
		t.Fatalf("lobby called %d times, want 2", fl.calls)
	}

	// A Lobby outage hides the widget and isn't cached.
	down := &fakeFeedbackLobby{err: errors.New("boom")}
	hd := &FeedbackHandler{lobby: down, logger: zerolog.Nop(), reporters: map[string]cachedReporter{}}
	widgetFor(hd, "pat@acme.com")
	if got := widgetFor(hd, "pat@acme.com"); got.Enabled || down.calls != 2 {
		t.Fatalf("outage: %+v, calls %d", got, down.calls)
	}

	// Lobby not configured: always off.
	var none *FeedbackHandler
	if got := widgetFor(none, "pat@acme.com"); got.Enabled {
		t.Fatal("nil handler must answer off")
	}
}
