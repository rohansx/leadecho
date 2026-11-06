package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"leadecho/internal/api/middleware"
	"leadecho/internal/auth"
)

func TestParseOpsAdmins(t *testing.T) {
	got := ParseOpsAdmins(" Sharmarohan615@Gmail.com, ,b@x.io ")
	if len(got) != 2 || !got["sharmarohan615@gmail.com"] || !got["b@x.io"] {
		t.Fatalf("got %v", got)
	}
	if len(ParseOpsAdmins("")) != 0 {
		t.Fatal("empty list must allow nobody")
	}
}

// /ops is gated on a server-side email allowlist only. Workspace roles
// (admin of your own workspace) never grant it.
func TestRequireOps(t *testing.T) {
	h := &OpsHandler{admins: ParseOpsAdmins("sharmarohan615@gmail.com")}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	cases := []struct {
		name   string
		claims *auth.Claims
		h      *OpsHandler
		want   int
	}{
		{"allowlisted (case-insensitive)", &auth.Claims{Email: "SharmaRohan615@gmail.com", Role: "member"}, h, http.StatusTeapot},
		{"workspace admin, not allowlisted", &auth.Claims{Email: "someone@else.com", Role: "admin"}, h, http.StatusForbidden},
		{"no session", nil, h, http.StatusForbidden},
		{"empty allowlist", &auth.Claims{Email: "sharmarohan615@gmail.com"}, &OpsHandler{admins: ParseOpsAdmins("")}, http.StatusForbidden},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/ops/workspaces", nil)
		if c.claims != nil {
			r = r.WithContext(middleware.WithClaims(r.Context(), c.claims))
		}
		w := httptest.NewRecorder()
		c.h.RequireOps(ok).ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: code=%d want %d", c.name, w.Code, c.want)
		}
	}
}
