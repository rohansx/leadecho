package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestLLMSTxt(t *testing.T) {
	rec := httptest.NewRecorder()
	LLMSTxt(rec, httptest.NewRequest(http.MethodGet, "/llms.txt", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"# LeadEcho", "/mcp", "/api/public/v1/probe", "/docs", "search_mentions", "attribution_report"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

// Guards drift between the Go copy and the landing site's static file.
func TestLLMSTxtMatchesLanding(t *testing.T) {
	landing, err := os.ReadFile("../../../../landing/public/llms.txt")
	if err != nil {
		t.Skipf("landing/public/llms.txt not found: %v", err)
	}
	if string(landing) != llmsTxt {
		t.Error("landing/public/llms.txt and handler.llmsTxt differ; keep them identical")
	}
}
