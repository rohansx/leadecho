package onboarding

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const homeHTML = `<html><head><title>Acme CRM</title><script>var x=1</script></head><body>
<nav><a href="/pricing">Pricing</a><a href="/about">About</a><a href="https://evil.test/pricing">ext</a></nav>
<h1>Acme keeps agency deals moving</h1>
<a href="/pricing#plans">plans</a><a href="mailto:hi@acme.test">mail</a><a href="/blog/random-post">blog</a>
<a href="/docs/start">Docs</a><a href="/features">Features</a><a href="javascript:alert(1)">x</a>
<footer>copyright</footer></body></html>`

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestExtractPage(t *testing.T) {
	title, text, links := extractPage(homeHTML, mustURL(t, "https://acme.test/"))
	if title != "Acme CRM" {
		t.Fatalf("title %q", title)
	}
	if !strings.Contains(text, "Acme keeps agency deals moving") || strings.Contains(text, "var x") || strings.Contains(text, "copyright") {
		t.Fatalf("text %q", text)
	}
	joined := strings.Join(links, " ")
	if !strings.Contains(joined, "https://acme.test/pricing") || strings.Contains(joined, "mailto") || strings.Contains(joined, "javascript") {
		t.Fatalf("links %v", links)
	}
}

func TestPickLinks(t *testing.T) {
	base := mustURL(t, "https://acme.test/")
	links := []string{
		"https://acme.test/blog/random-post", "https://acme.test/about", "https://acme.test/pricing",
		"https://acme.test/pricing", "https://evil.test/pricing", "https://acme.test/docs/start",
		"https://acme.test/features", "https://acme.test/", "https://sub.acme.test/pricing",
	}
	got := pickLinks(base, links, 3)
	want := []string{"https://acme.test/pricing", "https://acme.test/features", "https://acme.test/docs/start"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCrawlSameHostBounded(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, strings.ReplaceAll(homeHTML, "https://acme.test", ""))
		case "/about":
			w.WriteHeader(500)
		default:
			fmt.Fprintf(w, "<html><title>%s</title><body><p>Content of %s page is here.</p></body></html>", r.URL.Path, r.URL.Path)
		}
	}))
	defer srv.Close()

	var steps []string
	pages, err := Crawl(context.Background(), PlainFetcher(srv.Client()), srv.URL+"/", 3, func(s string) { steps = append(steps, s) })
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 || pages[0].Title != "Acme CRM" {
		t.Fatalf("pages: %+v", pages)
	}
	for _, h := range hits {
		if h == "/blog/random-post" {
			t.Fatal("non-priority link crawled")
		}
	}
	if len(steps) == 0 {
		t.Fatal("no progress reported")
	}
}

func TestCrawlStartFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer srv.Close()
	if _, err := Crawl(context.Background(), PlainFetcher(srv.Client()), srv.URL, 8, nil); err == nil {
		t.Fatal("want error for unreachable start page")
	}
}

func TestSafeFetcherBlocksInternal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "secret") }))
	defer srv.Close()
	if _, err := SafeFetcher(2*time.Second)(context.Background(), srv.URL); err == nil {
		t.Fatal("SSRF guard must refuse loopback")
	}
}

func TestNormalizeProductURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"acme.test", "https://acme.test"},
		{"  https://acme.test/x ", "https://acme.test/x"},
		{"ftp://acme.test", ""},
		{"", ""},
		{"https://", ""},
	}
	for _, c := range cases {
		got, err := NormalizeProductURL(c.in)
		if got != c.want || (c.want == "") != (err != nil) {
			t.Errorf("%q: got %q err %v", c.in, got, err)
		}
	}
}
