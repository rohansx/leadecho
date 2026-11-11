package digest

import (
	"strings"
	"testing"
)

func TestWhoLine(t *testing.T) {
	cases := []struct{ name, meta, want string }{
		{"full", `{"enrichment":{"full_name":"Jane Doe","title":"VP Engineering","company":"Acme","employees":"51-200"}}`, "Jane Doe · VP Engineering · Acme (51-200)"},
		{"bio stands in for a missing title", `{"enrichment":{"full_name":"Sam Lee","bio":"CTO  at\nAcme"}}`, "Sam Lee · CTO at Acme"},
		{"company only", `{"enrichment":{"company":"Acme"}}`, "Acme"},
		{"not enriched", `{"auto_scored":true}`, ""},
		{"bad json", `nope`, ""},
	}
	for _, c := range cases {
		if got := whoLine([]byte(c.meta)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPipelineCounts(t *testing.T) {
	got := PipelineCounts(map[string]int{"converted": 1, "prospect": 14, "engaged": 2, "lost": 0, "unknown": 9})
	want := []Count{{"to reply", 14}, {"in conversation", 2}, {"won", 1}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	}
	d := Data{Kind: KindDaily, Pipeline: got}
	html, err := RenderHTML(d)
	if err != nil || !strings.Contains(html, "14 to reply → 2 in conversation → 1 won") {
		t.Fatalf("email pipeline line missing (err=%v)", err)
	}
	if !strings.Contains(RenderSlack(d), "14 to reply → 2 in conversation → 1 won") {
		t.Fatal("slack pipeline line missing")
	}
}

func TestRenderShowsWhoAndDraft(t *testing.T) {
	d := Data{Kind: KindDaily, TopLeads: []Lead{{
		Title: "Need a screening tool", URL: "https://example.com/p", Platform: "linkedin", Score: 8,
		Who: "Jane Doe · VP Engineering · Acme", Draft: "Happy to share how we <do> this.",
	}}}
	html, err := RenderHTML(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Jane Doe · VP Engineering · Acme") || !strings.Contains(html, "Drafted message:") {
		t.Fatalf("email is missing who/draft:\n%s", html)
	}
	if strings.Contains(html, "<do>") {
		t.Fatal("draft text must be HTML-escaped")
	}
	slack := RenderSlack(d)
	if !strings.Contains(slack, "Jane Doe") || !strings.Contains(slack, "Drafted message: Happy to share how we &lt;do&gt; this.") {
		t.Fatalf("slack is missing who/draft:\n%s", slack)
	}
}
