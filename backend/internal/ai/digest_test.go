package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSummarizeDigest(t *testing.T) {
	tests := []struct {
		name    string
		reply   string
		status  int
		want    string
		wantErr bool
	}{
		{"trims reply", "  12 buyers this week.  ", 200, "12 buyers this week.", false},
		{"empty reply is an error", "   ", 200, "", true},
		{"upstream failure", "", 500, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req chatRequest
				_ = json.NewDecoder(r.Body).Decode(&req)
				if len(req.Messages) != 2 || !strings.Contains(req.Messages[1].Content, "buyers: 12") {
					t.Errorf("facts not passed: %+v", req.Messages)
				}
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": tc.reply}}}})
			}))
			defer srv.Close()
			got, err := SummarizeDigest(context.Background(), Provider{Name: "openai", BaseURL: srv.URL, Model: "m"}, "buyers: 12")
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("got %q err %v", got, err)
			}
		})
	}
}
