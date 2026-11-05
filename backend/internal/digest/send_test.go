package digest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSlackURLAllowed(t *testing.T) {
	tests := map[string]bool{
		"https://hooks.slack.com/services/T/B/x": true,
		"https://HOOKS.slack.com/services/T/B/x": true,
		"http://hooks.slack.com/services/T/B/x":  false,
		"https://evil.example/services":          false,
		"https://hooks.slack.com.evil.io/x":      false,
		"":                                       false,
		"::not a url":                            false,
	}
	for u, want := range tests {
		if got := slackURLAllowed(u); got != want {
			t.Errorf("slackURLAllowed(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestPostSlack(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"ok", 200, false},
		{"error status", 500, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&got)
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			err := postSlack(context.Background(), srv.Client(), srv.URL, "hello")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if got["text"] != "hello" {
				t.Fatalf("payload %v", got)
			}
		})
	}
}

func TestSlackURLFromSettings(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"present", `{"webhooks":{"slack_url":"https://hooks.slack.com/services/a"}}`, "https://hooks.slack.com/services/a"},
		{"no webhooks", `{"api_keys":{}}`, ""},
		{"garbage", `not json`, ""},
		{"empty", ``, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := slackURLFromSettings([]byte(tc.raw)); got != tc.want {
				t.Fatalf("got %q", got)
			}
		})
	}
}
