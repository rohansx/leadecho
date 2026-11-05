package handler

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

func TestSessionResponseManaged(t *testing.T) {
	h := NewSessionHandler(nil, nil, nil)
	h.SetManaged(TregManagedPlatforms...)
	acc := &database.PlatformAccount{AccessTokenEnc: pgtype.Text{String: "x", Valid: true}, Username: pgtype.Text{String: "me", Valid: true}}
	cases := []struct {
		platform       string
		acc            *database.PlatformAccount
		wantManaged    bool
		wantConfigured bool
	}{
		{"twitter", nil, true, false},
		{"linkedin", acc, true, true},
		{"reddit", nil, true, false},
		{"quora", acc, false, true},
	}
	for _, tc := range cases {
		r := h.sessionResponse(tc.platform, tc.acc, true)
		if r.IsManaged != tc.wantManaged || r.IsConfigured != tc.wantConfigured || !r.IsPinchtabOnline || r.Platform != tc.platform {
			t.Errorf("%s: %+v", tc.platform, r)
		}
	}
	if r := NewSessionHandler(nil, nil, nil).sessionResponse("twitter", nil, false); r.IsManaged {
		t.Error("without treg nothing is managed")
	}
}
