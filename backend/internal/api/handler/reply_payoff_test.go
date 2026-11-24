package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"leadecho/internal/api/middleware"
	"leadecho/internal/apikey"
	"leadecho/internal/database"
)

func TestConversionInputValidate(t *testing.T) {
	const replyID = "3f2b8c1e-6a4d-4e2f-9b1a-0c5d7e8f9a10"
	tests := []struct {
		name    string
		in      conversionInput
		wantErr string
		want    conversionInput
	}{
		{"code defaults to signup", conversionInput{UTMCode: " aB3dE6gH "}, "", conversionInput{UTMCode: "aB3dE6gH", EventType: "signup"}},
		{"reply id via utm_content", conversionInput{UTMContent: replyID, EventType: "Purchase", RevenueCents: 4900}, "", conversionInput{UTMContent: replyID, EventType: "purchase", RevenueCents: 4900}},
		{"neither id", conversionInput{EventType: "signup"}, "utm_code or utm_content is required", conversionInput{}},
		{"malformed code", conversionInput{UTMCode: "abc'; drop"}, "utm_code is malformed", conversionInput{}},
		{"too short code", conversionInput{UTMCode: "ab"}, "utm_code is malformed", conversionInput{}},
		{"utm_content not a reply id", conversionInput{UTMContent: "spring-campaign"}, "utm_content must be the reply id", conversionInput{}},
		{"click is not reportable", conversionInput{UTMCode: "aB3dE6gH", EventType: "click"}, "event_type must be signup or purchase", conversionInput{}},
		{"negative revenue", conversionInput{UTMCode: "aB3dE6gH", RevenueCents: -1}, "revenue_cents out of range", conversionInput{}},
		{"huge revenue", conversionInput{UTMCode: "aB3dE6gH", RevenueCents: maxConversionRevenue + 1}, "revenue_cents out of range", conversionInput{}},
		{"long event id", conversionInput{UTMCode: "aB3dE6gH", EventID: strings.Repeat("x", maxConversionEventID+1)}, "event_id too long", conversionInput{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.validate()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if got.UTMCode != tt.want.UTMCode || got.UTMContent != tt.want.UTMContent ||
				got.EventType != tt.want.EventType || got.RevenueCents != tt.want.RevenueCents {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// The API endpoint rejects a request before touching the DB: no API key
// principal, bad JSON, an oversized body or invalid fields.
func TestRecordConversionAPIRejects(t *testing.T) {
	key, err := apikey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	store := oneKeyStore{database.WorkspaceApiKey{ID: "k1", WorkspaceID: "ws-1", KeyHash: key.Hash, Scopes: []string{"read", "write"}}}
	h := middleware.APIKeyAuth(store, nil, 0)(http.HandlerFunc(NewConversionWebhookHandler(nil, nil, nil).RecordConversionAPI))
	tests := []struct {
		name   string
		body   string
		auth   bool
		status int
	}{
		{"no api key", `{"utm_code":"aB3dE6gH"}`, false, http.StatusUnauthorized},
		{"bad json", `{`, true, http.StatusBadRequest},
		{"oversized body", `{"utm_code":"aB3dE6gH","metadata":{"x":"` + strings.Repeat("a", maxConversionBody) + `"}}`, true, http.StatusBadRequest},
		{"invalid event", `{"utm_code":"aB3dE6gH","event_type":"click"}`, true, http.StatusBadRequest},
		{"missing id", `{"event_type":"signup"}`, true, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/public/v1/conversions", strings.NewReader(tt.body))
			if tt.auth {
				req.Header.Set("Authorization", "Bearer "+key.Plaintext)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.status, rec.Body.String())
			}
		})
	}
}

// The legacy hook caps each workspace_id before the secret is checked.
func TestLegacyConversionHookWorkspaceQuota(t *testing.T) {
	h := NewConversionWebhookHandler(nil, nil, middleware.NewQuota(1, time.Hour))
	do := func(ws string) int {
		body := `{"workspace_id":"` + ws + `","secret":"guess","utm_code":"aB3dE6gH","event_type":"click"}`
		rec := httptest.NewRecorder()
		h.RecordConversion(rec, httptest.NewRequest(http.MethodPost, "/api/v1/hooks/conversion", strings.NewReader(body)))
		return rec.Code
	}
	if c := do("ws-a"); c != http.StatusBadRequest { // within quota; rejected by validation
		t.Fatalf("first = %d, want 400", c)
	}
	if c := do("ws-a"); c != http.StatusTooManyRequests {
		t.Fatalf("second = %d, want 429", c)
	}
	if c := do("ws-b"); c != http.StatusBadRequest {
		t.Fatalf("other workspace = %d, want 400", c)
	}
}

type oneKeyStore struct{ key database.WorkspaceApiKey }

func (s oneKeyStore) GetActiveWorkspaceAPIKeyByHash(_ context.Context, hash string) (database.WorkspaceApiKey, error) {
	if hash != s.key.KeyHash {
		return database.WorkspaceApiKey{}, pgx.ErrNoRows
	}
	return s.key, nil
}

func (oneKeyStore) TouchWorkspaceAPIKey(context.Context, database.TouchWorkspaceAPIKeyParams) error {
	return nil
}

func TestWithEventIDCopies(t *testing.T) {
	meta := map[string]any{"plan": "pro"}
	got := withEventID(meta, "evt_1")
	if got["event_id"] != "evt_1" || got["plan"] != "pro" {
		t.Fatalf("got %v", got)
	}
	if _, mutated := meta["event_id"]; mutated {
		t.Fatal("input map was mutated")
	}
	if _, ok := withEventID(nil, "")["event_id"]; ok {
		t.Fatal("empty event id must not be stored")
	}
}

func TestIsBotUserAgent(t *testing.T) {
	tests := []struct {
		ua   string
		want bool
	}{
		{"", true},
		{"Mozilla/5.0 (compatible; redditbot/1.0; +http://www.reddit.com/feedback)", true},
		{"Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)", true},
		{"Mozilla/5.0 (compatible; Discordbot/2.0; +https://discordapp.com)", true},
		{"facebookexternalhit/1.1", true},
		{"Twitterbot/1.0", true},
		{"curl/8.4.0", true},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36", false},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148", false},
	}
	for _, tt := range tests {
		if got := isBotUserAgent(tt.ua); got != tt.want {
			t.Errorf("isBotUserAgent(%q) = %v, want %v", tt.ua, got, tt.want)
		}
	}
}

func TestSummarizePayoff(t *testing.T) {
	rows := []database.ReplyPayoffRow{ // newest first, as the query returns them
		{ReplyID: "a", Platform: "reddit", Keyword: "crm", Clicks: 3, Signups: 0},
		{ReplyID: "b", Platform: "hackernews", Keyword: "", Clicks: 1, Signups: 1},
		{ReplyID: "c", Platform: "reddit", Keyword: "crm", Clicks: 10, Signups: 2},
		{ReplyID: "d", Platform: "reddit", Keyword: "sales tool", Clicks: 3, Signups: 0},
	}
	got := summarizePayoff(rows, 3)

	if got.Totals != (payoffTotal{Key: "all", Replies: 4, Clicks: 17, Signups: 3}) {
		t.Fatalf("totals = %+v", got.Totals)
	}
	var order []string
	for _, r := range got.Replies {
		order = append(order, r.ReplyID)
	}
	// sign-ups, then clicks, then newest first; capped at 3.
	if strings.Join(order, ",") != "c,b,a" {
		t.Fatalf("order = %v", order)
	}
	if want := []payoffTotal{{"reddit", 3, 16, 2}, {"hackernews", 1, 1, 1}}; !equalTotals(got.ByPlatform, want) {
		t.Fatalf("by platform = %+v", got.ByPlatform)
	}
	if want := []payoffTotal{{"crm", 2, 13, 2}, {"(no keyword)", 1, 1, 1}, {"sales tool", 1, 3, 0}}; !equalTotals(got.ByKeyword, want) {
		t.Fatalf("by keyword = %+v", got.ByKeyword)
	}

	empty := summarizePayoff(nil, 50)
	if empty.Replies == nil || len(empty.ByPlatform) != 0 || empty.Totals.Replies != 0 {
		t.Fatalf("empty = %+v", empty)
	}
}

func equalTotals(a, b []payoffTotal) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
