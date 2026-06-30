package monitor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestFetchExaRequestAndParse(t *testing.T) {
	var gotMethod, gotKey, gotContentType string
	var gotBody exaSearchRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotKey = r.Header.Get("x-api-key")
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{
			"requestId": "abc",
			"results": [
				{
					"id": "https://example.com/post",
					"url": "https://example.com/post",
					"title": "Looking for a CRM alternative",
					"author": "jane",
					"publishedDate": "2026-06-10T00:00:00.000Z",
					"text": "We outgrew our CRM and need something cheaper."
				}
			]
		}`)
	}))
	defer srv.Close()

	orig := exaSearchURL
	exaSearchURL = srv.URL
	defer func() { exaSearchURL = orig }()

	m := &Monitor{exaAPIKey: "test-key", logger: zerolog.Nop()}
	results, err := m.fetchExa(context.Background(), "CRM alternative")
	if err != nil {
		t.Fatalf("fetchExa error: %v", err)
	}

	// Request contract
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotKey != "test-key" {
		t.Errorf("x-api-key = %q, want test-key", gotKey)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody.Query != "CRM alternative" {
		t.Errorf("query = %q, want %q", gotBody.Query, "CRM alternative")
	}
	if gotBody.Type != "auto" {
		t.Errorf("type = %q, want auto", gotBody.Type)
	}
	if gotBody.NumResults != exaNumResults {
		t.Errorf("numResults = %d, want %d", gotBody.NumResults, exaNumResults)
	}
	if !gotBody.Contents.Text {
		t.Error("contents.text = false, want true")
	}
	if gotBody.StartPublishedDate == "" {
		t.Error("startPublishedDate is empty, want a recency filter")
	} else if _, perr := time.Parse(time.RFC3339, gotBody.StartPublishedDate); perr != nil {
		t.Errorf("startPublishedDate %q not RFC3339: %v", gotBody.StartPublishedDate, perr)
	}

	// Response parse
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Title != "Looking for a CRM alternative" || results[0].Author != "jane" {
		t.Errorf("parsed result mismatch: %+v", results[0])
	}
}

func TestFetchExaNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	orig := exaSearchURL
	exaSearchURL = srv.URL
	defer func() { exaSearchURL = orig }()

	m := &Monitor{exaAPIKey: "bad", logger: zerolog.Nop()}
	results, err := m.fetchExa(context.Background(), "x")
	if err != nil {
		t.Fatalf("expected nil error on non-200, got %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results on non-200, got %d", len(results))
	}
}

func TestExaResultContent(t *testing.T) {
	tests := []struct {
		name string
		hit  exaResult
		want string
	}{
		{
			name: "title and text joined",
			hit:  exaResult{Title: "Title", Text: "Body text here"},
			want: "Title\n\nBody text here",
		},
		{
			name: "falls back to highlights when text empty",
			hit:  exaResult{Title: "T", Highlights: []string{"snippet one", "snippet two"}},
			want: "T\n\nsnippet one snippet two",
		},
		{
			name: "title only",
			hit:  exaResult{Title: "Just a title"},
			want: "Just a title",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exaResultContent(tt.hit); got != tt.want {
				t.Errorf("content = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExaResultContentTruncates(t *testing.T) {
	long := strings.Repeat("a", exaMaxContent+500)
	got := exaResultContent(exaResult{Text: long})
	if n := len([]rune(got)); n != exaMaxContent {
		t.Errorf("content length = %d runes, want %d", n, exaMaxContent)
	}
}

func TestExaResultLink(t *testing.T) {
	if got := exaResultLink(exaResult{URL: "https://u", ID: "https://i"}); got != "https://u" {
		t.Errorf("link = %q, want url preferred", got)
	}
	if got := exaResultLink(exaResult{ID: "https://i"}); got != "https://i" {
		t.Errorf("link = %q, want id fallback", got)
	}
	if got := exaResultLink(exaResult{}); got != "" {
		t.Errorf("link = %q, want empty", got)
	}
}

func TestExaResultPublished(t *testing.T) {
	hit := exaResult{PublishedDate: "2026-06-10T12:00:00Z"}
	got := exaResultPublished(hit)
	want, _ := time.Parse(time.RFC3339, "2026-06-10T12:00:00Z")
	if !got.Equal(want) {
		t.Errorf("published = %v, want %v", got, want)
	}

	// Missing/malformed dates default to ~now.
	for _, bad := range []string{"", "not-a-date"} {
		got := exaResultPublished(exaResult{PublishedDate: bad})
		if time.Since(got) > time.Minute {
			t.Errorf("published for %q = %v, want ~now", bad, got)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("hello", 10); got != "hello" {
		t.Errorf("under-limit changed: %q", got)
	}
	if got := truncateRunes("hello", 3); got != "hel" {
		t.Errorf("over-limit = %q, want hel", got)
	}
	// Multibyte: each rune is one unit, never split.
	if got := truncateRunes("héllo", 2); got != "hé" {
		t.Errorf("multibyte = %q, want hé", got)
	}
	if got := truncateRunes("x", 0); got != "" {
		t.Errorf("zero max = %q, want empty", got)
	}
}
