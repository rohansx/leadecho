package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResendSend(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  string
		status  int
		wantErr bool
		wantHit bool
	}{
		{"disabled without key is a no-op", "", 200, false, false},
		{"success", "re_test", 200, false, true},
		{"upstream error surfaces", "re_test", 422, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hit := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hit = true
				if r.Header.Get("Authorization") != "Bearer re_test" {
					t.Errorf("auth header %q", r.Header.Get("Authorization"))
				}
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if to, _ := body["to"].([]any); len(to) != 2 || body["subject"] != "Hi" || body["from"] != "LeadEcho <x@y.z>" {
					t.Errorf("unexpected body %v", body)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"message":"bad"}`))
			}))
			defer srv.Close()
			r := Resend{APIKey: tc.apiKey, From: "LeadEcho <x@y.z>", Endpoint: srv.URL}
			err := r.Send(context.Background(), []string{"a@b.c", "d@e.f"}, "Hi", "<p>x</p>")
			if (err != nil) != tc.wantErr || hit != tc.wantHit {
				t.Fatalf("err=%v hit=%v", err, hit)
			}
		})
	}
}
