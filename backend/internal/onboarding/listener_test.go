package onboarding

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/monitor"
)

var testBrand = BrandProfile{
	Product:     "Acme",
	Overview:    "CRM for agencies",
	Competitors: []string{"HubSpot", "Pipedrive"},
	PainPoints:  []ai.PainPointDetail{{Problem: "lost deals"}},
	Keywords: ai.KeywordGroups{
		Brand:    []string{"Acme"},
		Category: []string{"crm for agencies"},
		Problem:  []string{"lost deals", "messy spreadsheets"},
	},
}

func TestParseGoals(t *testing.T) {
	cases := []struct {
		in      []string
		want    int
		wantErr bool
	}{
		{[]string{"find_buyers", "brand_mentions"}, 2, false},
		{[]string{"find_buyers", "find_buyers"}, 1, false},
		{[]string{}, 0, true},
		{[]string{"hack"}, 0, true},
	}
	for _, c := range cases {
		got, err := ParseGoals(c.in)
		if (err != nil) != c.wantErr || len(got) != c.want {
			t.Errorf("%v: got %v err %v", c.in, got, err)
		}
	}
}

func TestTemplateProposal(t *testing.T) {
	for _, g := range AllGoals {
		t.Run(string(g), func(t *testing.T) {
			p := TemplateProposal(g, testBrand, []string{"reddit"})
			if p.Name == "" || p.LooksFor == "" || p.Skips == "" || len(p.Keywords) == 0 {
				t.Fatalf("incomplete template: %+v", p)
			}
			if len(p.Keywords) > maxListenerKeywords {
				t.Fatalf("too many keywords: %v", p.Keywords)
			}
		})
	}
	p := TemplateProposal(GoalCompetitorSwitchers, testBrand, nil)
	if !strings.Contains(strings.Join(p.Keywords, ","), "HubSpot alternative") {
		t.Fatalf("competitor keywords: %v", p.Keywords)
	}
	// Empty brand still yields something usable.
	if p := TemplateProposal(GoalFindBuyers, BrandProfile{Product: "Zed"}, nil); len(p.Keywords) == 0 {
		t.Fatal("empty brand produced no keywords")
	}
}

type fakeGen struct {
	draft *ai.ListenerDraft
	err   error
}

func (f fakeGen) GenerateListener(context.Context, string, ai.ListenerPrompt) (*ai.ListenerDraft, error) {
	return f.draft, f.err
}

type fakeSearch struct {
	inFlight, maxInFlight atomic.Int32
	block                 bool
	counts                map[string]int // keyword -> posts per platform
}

func (f *fakeSearch) Search(ctx context.Context, platform, term string, since time.Time) ([]monitor.Post, error) {
	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		m := f.maxInFlight.Load()
		if n <= m || f.maxInFlight.CompareAndSwap(m, n) {
			break
		}
	}
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	time.Sleep(2 * time.Millisecond)
	var posts []monitor.Post
	for i := 0; i < f.counts[term]; i++ {
		posts = append(posts, monitor.Post{
			Platform: platform, URL: fmt.Sprintf("https://x.test/%s/%s/%d", platform, term, i),
			Title: term, Content: "a real post body about " + term, CreatedAt: time.Now().Add(-time.Duration(i) * time.Hour),
		})
	}
	return posts, nil
}

type recorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *recorder) emit(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) of(typ string) []Event {
	var out []Event
	for _, e := range r.events {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func TestBuilderRunWithLLMDraft(t *testing.T) {
	search := &fakeSearch{counts: map[string]int{"best crm": 5, "crm recommendation": 0}}
	b := &Builder{
		Gen:         fakeGen{draft: &ai.ListenerDraft{Name: "Buyers", Keywords: []string{"best crm", "crm recommendation"}, LooksFor: "shoppers", Skips: "vendors"}},
		Search:      search,
		Concurrency: 2,
	}
	rec := &recorder{}
	b.Run(context.Background(), "ws", testBrand, []Goal{GoalFindBuyers}, []string{"reddit", "hackernews", "twitter"}, rec.emit)

	if got := len(rec.of("query_tested")); got != 4 { // 2 keywords x 2 testable platforms
		t.Fatalf("query_tested events: %d", got)
	}
	if search.maxInFlight.Load() > 2 {
		t.Fatalf("concurrency bound exceeded: %d", search.maxInFlight.Load())
	}
	samples := rec.of("samples")
	if len(samples) != 1 || len(samples[0].Data.(SamplesEvent).Posts) != 3 {
		t.Fatalf("samples: %+v", samples)
	}
	props := rec.of("listener_proposal")
	if len(props) != 1 {
		t.Fatalf("proposals: %d", len(props))
	}
	p := props[0].Data.(Proposal)
	if p.Name != "Buyers" || p.LooksFor != "shoppers" {
		t.Fatalf("LLM draft not used: %+v", p)
	}
	if len(p.Keywords) != 1 || p.Keywords[0] != "best crm" {
		t.Fatalf("zero-result keyword should be dropped: %v", p.Keywords)
	}
	if len(p.Platforms) != 3 {
		t.Fatalf("user platforms must be kept (incl. untestable twitter): %v", p.Platforms)
	}
	if last := rec.events[len(rec.events)-1]; last.Type != "done" {
		t.Fatalf("last event %s", last.Type)
	}
}

func TestBuilderFallsBackToTemplateAndKeepsKeywordsWhenAllZero(t *testing.T) {
	b := &Builder{Gen: fakeGen{err: errors.New("no AI provider configured")}, Search: &fakeSearch{}}
	rec := &recorder{}
	b.Run(context.Background(), "ws", testBrand, []Goal{GoalBrandMentions}, []string{"hackernews"}, rec.emit)
	props := rec.of("listener_proposal")
	if len(props) != 1 {
		t.Fatal("missing proposal")
	}
	p := props[0].Data.(Proposal)
	want := TemplateProposal(GoalBrandMentions, testBrand, []string{"hackernews"})
	if p.Name != want.Name || len(p.Keywords) != len(want.Keywords) {
		t.Fatalf("template not used / keywords dropped: %+v", p)
	}
}

func TestBuilderNilGeneratorAndTimeout(t *testing.T) {
	b := &Builder{Search: &fakeSearch{block: true}, Total: 150 * time.Millisecond, PerSource: time.Second}
	rec := &recorder{}
	start := time.Now()
	b.Run(context.Background(), "ws", testBrand, []Goal{GoalFindBuyers, GoalContentIdeas}, []string{"reddit"}, rec.emit)
	if time.Since(start) > 2*time.Second {
		t.Fatal("Run ignored the total time bound")
	}
	if len(rec.of("listener_proposal")) != 2 {
		t.Fatalf("proposals must still be emitted after timeout: %d", len(rec.of("listener_proposal")))
	}
	for _, e := range rec.of("query_tested") {
		if e.Data.(QueryTested).Error == "" {
			t.Fatal("timed-out query should report an error")
		}
	}
}

func TestPickSamples(t *testing.T) {
	now := time.Now()
	posts := []monitor.Post{
		{URL: "a", Content: "short", CreatedAt: now},
		{URL: "b", Content: strings.Repeat("long text ", 10), CreatedAt: now.Add(-time.Hour)},
		{URL: "b", Content: strings.Repeat("dup ", 20), CreatedAt: now},
		{URL: "c", Content: strings.Repeat("x", 2000), CreatedAt: now.Add(-2 * time.Hour)},
	}
	got := pickSamples(posts, 3)
	if len(got) != 3 || got[0].URL != "b" || got[1].URL != "c" || got[2].URL != "a" {
		t.Fatalf("order/dedupe wrong: %+v", got)
	}
	if len([]rune(got[1].Text)) > maxSampleText {
		t.Fatal("sample text not capped")
	}
}
