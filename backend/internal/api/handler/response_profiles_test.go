package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"leadecho/internal/reply"
)

func TestDecodeResponseProfile(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantErr  string
		wantName string
		check    func(t *testing.T, v reply.Voice)
	}{
		{name: "partial body keeps defaults", body: `{"name":"  Launch "}`, wantName: "Launch", check: func(t *testing.T, v reply.Voice) {
			if v.Goal != "drive_conversion" || v.BrandPlug != "low" || v.Length != "medium" {
				t.Fatalf("defaults lost: %+v", v)
			}
		}},
		{name: "full body", body: `{"name":"A","goal":"support","tone":90,"length":"short","assertiveness":5,"brand_plug":"none","examples":{"reddit":"hey"}}`, wantName: "A",
			check: func(t *testing.T, v reply.Voice) {
				if v.Goal != "support" || v.Tone != 90 || v.Examples["reddit"] != "hey" {
					t.Fatalf("not applied: %+v", v)
				}
			}},
		{name: "missing name", body: `{"goal":"support"}`, wantErr: "name"},
		{name: "long name", body: `{"name":"` + strings.Repeat("a", 121) + `"}`, wantErr: "name"},
		{name: "bad json", body: `{`, wantErr: "invalid JSON"},
		{name: "bad tone", body: `{"name":"A","tone":101}`, wantErr: "tone"},
		{name: "bad example platform", body: `{"name":"A","examples":{"tiktok":"x"}}`, wantErr: "examples"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			name, v, err := decodeResponseProfile(r, "", reply.DefaultVoice())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if name != tt.wantName {
				t.Fatalf("name = %q", name)
			}
			tt.check(t, v)
		})
	}
}

func TestDraftReplyProfileID(t *testing.T) {
	tests := []struct {
		name, body, want string
		wantErr          bool
	}{
		{name: "empty body", body: "", want: ""},
		{name: "empty object", body: "{}", want: ""},
		{name: "id", body: `{"response_profile_id":" 6f1c0b9e-3a3e-4a37-9a51-5d1f6f7e2b11 "}`, want: "6f1c0b9e-3a3e-4a37-9a51-5d1f6f7e2b11"},
		{name: "bad json", body: "{", wantErr: true},
		{name: "not a uuid", body: `{"response_profile_id":"nope"}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			got, err := draftReplyProfileID(r)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("got %q err %v", got, err)
			}
		})
	}
}
