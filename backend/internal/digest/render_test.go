package digest

import (
	"strings"
	"testing"
)

func TestRenderHTML(t *testing.T) {
	d := sampleData()
	d.Summary = "A strong week & more."
	out, err := RenderHTML(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Weekly digest", "A strong week &amp; more.", "&lt;script&gt;", "https://reddit.com/r/saas/1",
		"(untitled)", "$1,234.50", "reddit", "30", "crm", "https://app.leadecho.io/inbox", "8.7",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("html missing %q", want)
		}
	}
	if strings.Contains(out, "<script>") {
		t.Fatal("html must escape mention content")
	}

	daily := sampleData()
	daily.Kind = KindDaily
	out, _ = RenderHTML(daily)
	if !strings.Contains(out, "Daily digest") {
		t.Fatal("daily heading missing")
	}

	empty, err := RenderHTML(Data{Kind: KindDaily})
	if err != nil || !strings.Contains(empty, "No new high-fit leads") {
		t.Fatalf("empty render err=%v", err)
	}
}

func TestRenderHTMLRejectsUnsafeURL(t *testing.T) {
	d := Data{Kind: KindDaily, TopLeads: []Lead{{Title: "x", URL: "javascript:alert(1)", Score: 9}}}
	out, err := RenderHTML(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "javascript:") {
		t.Fatal("javascript: URL must be neutralized")
	}
}

func TestRenderSlack(t *testing.T) {
	d := sampleData()
	d.Summary = "Week <b> & stuff"
	out := RenderSlack(d)
	for _, want := range []string{
		"*LeadEcho weekly digest*", "Week &lt;b&gt; &amp; stuff",
		"<https://reddit.com/r/saas/1|Looking for a &lt;script&gt;alert(1)&lt;/script&gt; tool>",
		"$1,234.50", "crm",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("slack text missing %q:\n%s", want, out)
		}
	}
}
