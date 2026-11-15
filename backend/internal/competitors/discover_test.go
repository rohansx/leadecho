package competitors

import (
	"reflect"
	"sort"
	"testing"
)

func kinds(ss []Source) map[string]string {
	m := map[string]string{}
	for _, s := range ss {
		m[s.Kind] = s.URL
	}
	return m
}

func TestExtractSourcesFindsEachKind(t *testing.T) {
	page := `<html><body>
<nav><a href="/pricing">Pricing</a><a href="/blog">Blog</a><a href="/changelog">Changelog</a></nav>
<a href="https://x.com/acme">X</a>
<a href="https://www.linkedin.com/company/acme-inc/">LinkedIn</a>
<a href="https://github.com/acme/widget/issues/3">GitHub</a>
<a href="https://other.com/blog">Partner blog</a>
<a href="/blog/why-our-pricing-changed">A post</a>
</body></html>`
	got := kinds(ExtractSources("https://acme.com/", page))
	want := map[string]string{
		KindLanding:   "https://acme.com/",
		KindPricing:   "https://acme.com/pricing",
		KindBlog:      "https://acme.com/blog",
		KindChangelog: "https://acme.com/changelog",
		KindX:         "https://x.com/acme",
		KindLinkedIn:  "https://www.linkedin.com/company/acme-inc",
		KindGitHub:    "https://github.com/acme",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestExtractSourcesSkipsAmbiguousAndNonProfileLinks(t *testing.T) {
	page := `<a href="https://x.com/acme">a</a><a href="https://twitter.com/acme_support">b</a>
<a href="https://x.com/intent/tweet?text=hi">share</a><a href="https://twitter.com/home">home</a>
<a href="https://www.linkedin.com/in/jane-doe">founder</a>
<a href="https://www.linkedin.com/shareArticle?url=x">share</a>
<a href="https://github.com/features">gh</a><a href="https://github.com/login">gh</a>`
	got := ExtractSources("https://acme.com", page)
	var ks []string
	for _, s := range got {
		ks = append(ks, s.Kind)
	}
	sort.Strings(ks)
	if !reflect.DeepEqual(ks, []string{KindLanding}) {
		t.Fatalf("two X handles, a personal LinkedIn and GitHub's own pages must yield no source, got %v", got)
	}
}

func TestExtractSourcesSameHandleTwiceIsOne(t *testing.T) {
	page := `<a href="https://twitter.com/Acme">a</a><a href="https://x.com/acme?ref=footer">b</a>`
	if got := kinds(ExtractSources("https://acme.com", page))[KindX]; got != "https://x.com/acme" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractSourcesSubdomainBlog(t *testing.T) {
	page := `<a href="https://blog.acme.com/">Blog</a><a href="https://docs.acme.com/changelog">Updates</a>`
	got := kinds(ExtractSources("https://www.acme.com", page))
	if got[KindBlog] != "https://blog.acme.com/" || got[KindChangelog] != "https://docs.acme.com/changelog" {
		t.Fatalf("got %v", got)
	}
}

func TestFeedLink(t *testing.T) {
	cases := map[string]string{
		`<head><link rel="alternate" type="application/atom+xml" href="/feed.xml"></head>`:                  "https://acme.com/feed.xml",
		`<head><link href="https://acme.com/rss" type="application/rss+xml" rel="alternate"></head>`:        "https://acme.com/rss",
		`<head><link rel="stylesheet" href="/a.css"><link rel="alternate" hreflang="de" href="/de"></head>`: "",
	}
	for page, want := range cases {
		if got := FeedLink("https://acme.com/blog", page); got != want {
			t.Errorf("FeedLink(%q) = %q, want %q", page, got, want)
		}
	}
}
