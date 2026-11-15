package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"leadecho/internal/onboarding"
)

// A JavaScript-only site answers a plain fetch with a shell: the page must be
// read through the browser fallback, and an unreadable one must not pass.
func TestCrawlSiteReadsJavaScriptOnlySite(t *testing.T) {
	const shell = `<html><head><title>LeadEcho - Social Intent Orchestrator</title></head><body><div id="root"></div></body></html>`
	const site = "https://93.184.216.34/" // a public address literal: no DNS in the URL check
	rendered := strings.Repeat("Find the people already asking for what you sell. ", 10)

	newHandler := func(reader onboarding.PageReader) *OnboardingHandler {
		h := NewOnboardingHandler(nil, nil, nil, zerolog.Nop())
		h.fetch = func(context.Context, string) (string, error) { return shell, nil }
		h.SetPageReader(reader)
		return h
	}
	crawl := func(h *OnboardingHandler) []onboarding.Page {
		t.Helper()
		pages, err := h.crawlSite(context.Background(), "ws", site, func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		return pages
	}

	var asked string
	pages := crawl(newHandler(func(_ context.Context, ws, u string) (string, error) {
		asked = ws + " " + u
		return rendered, nil
	}))
	if asked != "ws "+site || len(pages) == 0 || pages[0].Text != rendered {
		t.Fatalf("rendered text not used: asked %q, pages %+v", asked, pages)
	}

	for name, reader := range map[string]onboarding.PageReader{
		"no reader":    nil,
		"reader fails": func(context.Context, string, string) (string, error) { return "", errors.New("capped") },
		"reader empty": func(context.Context, string, string) (string, error) { return "", nil },
	} {
		if text := onboarding.AnalysisText(crawl(newHandler(reader))); len(text) >= onboarding.MinPageText {
			t.Errorf("%s: a shell page must stay too thin to analyse, got %d chars", name, len(text))
		}
	}
}

func TestParseRenderedPage(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"bare":    {`{"ok":true,"markdown":"  # Title\n\nBody  ","html":"<p>x</p>"}`, "# Title\n\nBody"},
		"wrapped": {`{"output":{"markdown":"Body"}}`, "Body"},
		"none":    {`{"ok":false}`, ""},
		"garbage": {`<html>`, ""},
	} {
		if got := onboarding.ParseRenderedPage([]byte(tc.body)); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}
