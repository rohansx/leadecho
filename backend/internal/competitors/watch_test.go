package competitors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const rssSample = `<?xml version="1.0"?><rss version="2.0"><channel><title>Acme blog</title>
<item><title>Bulk export is here</title><link>https://acme.com/blog/bulk</link><guid>bulk-1</guid>
<pubDate>Tue, 10 Nov 2026 09:00:00 +0000</pubDate><description>&lt;p&gt;Export &lt;b&gt;everything&lt;/b&gt; as CSV.&lt;/p&gt;</description></item>
<item><title>No guid</title><link>https://acme.com/blog/noguid</link></item>
</channel></rss>`

const atomSample = `<?xml version="1.0" encoding="UTF-8"?><feed xmlns="http://www.w3.org/2005/Atom">
<entry><id>tag:github.com,2008:Repository/1/v2.0.0</id><updated>2026-11-12T08:00:00Z</updated>
<link rel="alternate" type="text/html" href="https://github.com/acme/widget/releases/tag/v2.0.0"/>
<title>v2.0.0</title><content type="html">&lt;p&gt;Adds SSO&lt;/p&gt;</content></entry></feed>`

func TestParseFeedRSS(t *testing.T) {
	items, err := ParseFeed([]byte(rssSample))
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	a := items[0]
	if a.Key != "bulk-1" || a.Title != "Bulk export is here" || a.URL != "https://acme.com/blog/bulk" {
		t.Fatalf("got %+v", a)
	}
	if a.Body != "Export everything as CSV." {
		t.Fatalf("body %q", a.Body)
	}
	if !a.At.Equal(time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("at %v", a.At)
	}
	if items[1].Key != "https://acme.com/blog/noguid" {
		t.Fatalf("key falls back to the link, got %q", items[1].Key)
	}
}

func TestParseFeedAtom(t *testing.T) {
	items, err := ParseFeed([]byte(atomSample))
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	a := items[0]
	if a.Key != "tag:github.com,2008:Repository/1/v2.0.0" || a.Title != "v2.0.0" ||
		a.URL != "https://github.com/acme/widget/releases/tag/v2.0.0" || a.Body != "Adds SSO" ||
		!a.At.Equal(time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %+v", a)
	}
}

func TestParseFeedRejectsHTML(t *testing.T) {
	if _, err := ParseFeed([]byte(`<!doctype html><html><body>hi</body></html>`)); err == nil {
		t.Fatal("an HTML page is not a feed")
	}
}

func TestNormalizeTextIgnoresDatesAndSpacing(t *testing.T) {
	a := NormalizeText("Pricing\n  Starter   $29 \n\nNov 12, 2026\n© 2026 Acme\n1,204")
	b := NormalizeText("Pricing\nStarter $29\n2026-11-15\n© 2027 Acme\n1,377\n")
	if a != b || a != "Pricing\nStarter $29" {
		t.Fatalf("a=%q b=%q", a, b)
	}
}

func TestPageTextKeepsLinesAndSkipsChrome(t *testing.T) {
	got := PageText(`<html><head><title>x</title><style>p{}</style></head><body><nav>Home</nav>
<h1>Starter</h1><p>$29 per <b>month</b></p><script>var a=1</script><footer>legal</footer></body></html>`)
	if got != "Starter\n$29 per\nmonth" {
		t.Fatalf("got %q", got)
	}
}

func page(lines ...string) string {
	return "<html><body><p>" + strings.Join(lines, "</p><p>") + "</p></body></html>"
}

func longLines(n int) []string {
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, "This is line number "+string(rune('a'+i))+" of the page, long enough to count as text.")
	}
	return out
}

func fetcherOf(pages map[string]string) func(context.Context, string) (string, error) {
	return func(_ context.Context, u string) (string, error) {
		if b, ok := pages[u]; ok {
			return b, nil
		}
		return "", errors.New("status 404")
	}
}

func TestFetchPageBaselineThenChange(t *testing.T) {
	base := longLines(6)
	u := "https://acme.com/pricing"
	fetch := fetcherOf(map[string]string{u: page(append(base, "Starter $29")...)})

	res, err := FetchURL(context.Background(), fetch, KindPricing, u, "", "")
	if err != nil || len(res.Items) != 0 || res.Hash == "" {
		t.Fatalf("first check records a baseline and no items: %+v %v", res, err)
	}
	same, err := FetchURL(context.Background(), fetch, KindPricing, u, res.Hash, res.Text)
	if err != nil || len(same.Items) != 0 || same.Hash != res.Hash {
		t.Fatalf("unchanged page: %+v %v", same, err)
	}
	fetch = fetcherOf(map[string]string{u: page(append(base, "Starter $49")...)})
	changed, err := FetchURL(context.Background(), fetch, KindPricing, u, res.Hash, res.Text)
	if err != nil || len(changed.Items) != 1 {
		t.Fatalf("changed page: %+v %v", changed, err)
	}
	it := changed.Items[0]
	if it.Key != changed.Hash || it.URL != u ||
		!strings.Contains(it.Body, "REMOVED:\nStarter $29") || !strings.Contains(it.Body, "ADDED:\nStarter $49") {
		t.Fatalf("item %+v", it)
	}
	if strings.Contains(it.Body, "line number") {
		t.Fatalf("unchanged lines must not be in the item: %q", it.Body)
	}
}

func TestFetchPageTooShortIsAFailure(t *testing.T) {
	u := "https://acme.com/pricing"
	fetch := fetcherOf(map[string]string{u: `<html><body><div id="root"></div><script>render()</script></body></html>`})
	if _, err := FetchURL(context.Background(), fetch, KindPricing, u, "abc", "old"); !errors.Is(err, ErrEmptyPage) {
		t.Fatalf("a page with no text is a failed check, got %v", err)
	}
}

func TestFetchBlogUsesFeed(t *testing.T) {
	u := "https://acme.com/blog"
	fetch := fetcherOf(map[string]string{
		u:                          `<html><head><link rel="alternate" type="application/rss+xml" href="/rss.xml"></head><body>` + strings.Join(longLines(6), " ") + `</body></html>`,
		"https://acme.com/rss.xml": rssSample,
	})
	res, err := FetchURL(context.Background(), fetch, KindBlog, u, "", "")
	if err != nil || len(res.Items) != 2 || res.Hash != feedBaseline {
		t.Fatalf("got %+v %v", res, err)
	}
	// A feed served directly at the source URL works too.
	direct, err := FetchURL(context.Background(), fetcherOf(map[string]string{u: atomSample}), KindChangelog, u, "", "")
	if err != nil || len(direct.Items) != 1 {
		t.Fatalf("got %+v %v", direct, err)
	}
}

func TestFetchGitHubReadsReleaseFeeds(t *testing.T) {
	org := `<a href="/acme/widget">widget</a><a href="/acme/widget/stargazers">s</a><a href="/acme/cli">cli</a>
<a href="/acme/a">a</a><a href="/acme/b">b</a><a href="/orgs/acme/people">p</a><a href="/other/repo">x</a>`
	var asked []string
	fetch := func(_ context.Context, u string) (string, error) {
		asked = append(asked, u)
		switch u {
		case "https://github.com/acme":
			return org, nil
		case "https://github.com/acme/widget/releases.atom":
			return atomSample, nil
		}
		return `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"></feed>`, nil
	}
	res, err := FetchURL(context.Background(), fetch, KindGitHub, "https://github.com/acme", "", "")
	if err != nil || len(res.Items) != 1 || res.Items[0].Title != "widget v2.0.0" {
		t.Fatalf("got %+v %v", res, err)
	}
	if len(asked) != 4 {
		t.Fatalf("org page plus at most 3 repos, asked %v", asked)
	}
}

func TestParseFeedKeepsTheNewestWhenTheFeedIsOldestFirst(t *testing.T) {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\"?><rss version=\"2.0\"><channel>")
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&b, "<item><title>entry %d</title><guid>g%d</guid><pubDate>%s</pubDate></item>", i, i,
			time.Date(2026, 1, i, 0, 0, 0, 0, time.UTC).Format(time.RFC1123Z))
	}
	b.WriteString("</channel></rss>")
	items, err := ParseFeed([]byte(b.String()))
	if err != nil || len(items) != 30 || items[0].Title != "entry 40" {
		t.Fatalf("want the 30 newest, newest first: %d items, first %q, err %v", len(items), items[0].Title, err)
	}
}

func TestFetchGitHubFailsWhenNoReleaseFeedCanBeRead(t *testing.T) {
	fetch := func(_ context.Context, u string) (string, error) {
		if u == "https://github.com/acme" {
			return "<a href=\"/acme/widget\">w</a><a href=\"/acme/cli\">c</a>", nil
		}
		return "", errors.New("status 429")
	}
	if _, err := FetchURL(context.Background(), fetch, KindGitHub, "https://github.com/acme", "", ""); err == nil {
		t.Fatal("repositories exist but none could be read: that is a failed check, not an empty baseline")
	}
	// An organisation page with no repositories is simply empty.
	none := func(context.Context, string) (string, error) { return "<p>nothing here</p>", nil }
	if res, err := FetchURL(context.Background(), none, KindGitHub, "https://github.com/acme", "", ""); err != nil || len(res.Items) != 0 {
		t.Fatalf("got %+v %v", res, err)
	}
}
