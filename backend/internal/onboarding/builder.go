package onboarding

import (
	"context"
	"strings"
	"sync"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/monitor"
)

// Generator drafts a listener with an LLM (llm.Router satisfies it).
type Generator interface {
	GenerateListener(ctx context.Context, workspaceID string, lp ai.ListenerPrompt) (*ai.ListenerDraft, error)
}

// Searcher live-tests a keyword without inserting anything
// (monitor.PublicSources satisfies it).
type Searcher interface {
	Search(ctx context.Context, platform, term string, since time.Time) ([]monitor.Post, error)
}

// Event is one Server-Sent Event emitted by the builder.
type Event struct {
	Type string
	Data any
}

// StepEvent is a human-readable progress line.
type StepEvent struct {
	Goal    Goal   `json:"goal,omitempty"`
	Message string `json:"message"`
}

// QueryTested reports one keyword x platform live test.
type QueryTested struct {
	Goal     Goal   `json:"goal"`
	Platform string `json:"platform"`
	Keyword  string `json:"keyword"`
	Count    int    `json:"count"`
	Error    string `json:"error,omitempty"`
}

// SamplesEvent carries the real sample posts for one listener.
type SamplesEvent struct {
	Goal  Goal     `json:"goal"`
	Posts []Sample `json:"posts"`
}

const (
	defaultTotal       = 45 * time.Second
	defaultPerSource   = 8 * time.Second
	defaultConcurrency = 4
	generateTimeout    = 15 * time.Second
	sampleLookback     = 7 * 24 * time.Hour
)

// Builder builds one listener per goal: draft keywords + Looks for / Skips,
// live-test each keyword on the free public sources, pick sample posts.
// Zero values get sane defaults; Gen may be nil (template only).
type Builder struct {
	Gen         Generator
	Search      Searcher
	Total       time.Duration // whole run
	PerSource   time.Duration // one search call
	Concurrency int           // simultaneous searches across all goals
}

// Run emits events for every goal and always ends with "done". emit is
// called serially. Every goal yields a proposal even when tests time out.
func (b *Builder) Run(ctx context.Context, workspaceID string, bp BrandProfile, goals []Goal, platforms []string, emit func(Event)) {
	ctx, cancel := context.WithTimeout(ctx, orDur(b.Total, defaultTotal))
	defer cancel()

	var mu sync.Mutex
	safeEmit := func(typ string, data any) {
		mu.Lock()
		defer mu.Unlock()
		emit(Event{Type: typ, Data: data})
	}
	n := b.Concurrency
	if n <= 0 {
		n = defaultConcurrency
	}
	sem := make(chan struct{}, n)

	var wg sync.WaitGroup
	for _, g := range goals {
		wg.Add(1)
		go func(g Goal) {
			defer wg.Done()
			b.buildGoal(ctx, workspaceID, bp, g, platforms, sem, safeEmit)
		}(g)
	}
	wg.Wait()
	safeEmit("done", StepEvent{Message: "Listeners ready"})
}

func (b *Builder) buildGoal(ctx context.Context, wsID string, bp BrandProfile, g Goal, platforms []string, sem chan struct{}, emit func(string, any)) {
	emit("step", StepEvent{Goal: g, Message: "Drafting keywords for " + strings.ReplaceAll(string(g), "_", " ")})
	p := b.draft(ctx, wsID, bp, g, platforms)

	var testable []string
	for _, pl := range p.Platforms {
		if TestablePlatforms[pl] {
			testable = append(testable, pl)
		}
	}
	emit("step", StepEvent{Goal: g, Message: "Testing " + strings.Join(p.Keywords, ", ")})

	var (
		mu    sync.Mutex
		posts []monitor.Post
		hits  = map[string]int{}
		wg    sync.WaitGroup
	)
	since := time.Now().Add(-sampleLookback)
	for _, kw := range p.Keywords {
		for _, pl := range testable {
			wg.Add(1)
			go func(kw, pl string) {
				defer wg.Done()
				found, err := b.test(ctx, sem, pl, kw, since)
				qt := QueryTested{Goal: g, Platform: pl, Keyword: kw, Count: len(found)}
				if err != nil {
					qt.Error = err.Error()
				}
				emit("query_tested", qt)
				mu.Lock()
				posts = append(posts, found...)
				hits[kw] += len(found)
				mu.Unlock()
			}(kw, pl)
		}
	}
	wg.Wait()

	emit("samples", SamplesEvent{Goal: g, Posts: pickSamples(posts, samplesPerListener)})
	p.Keywords = keepProductive(p.Keywords, hits)
	emit("listener_proposal", p)
}

// test runs one search under the concurrency and per-source bounds.
func (b *Builder) test(ctx context.Context, sem chan struct{}, platform, kw string, since time.Time) ([]monitor.Post, error) {
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-sem }()
	sctx, cancel := context.WithTimeout(ctx, orDur(b.PerSource, defaultPerSource))
	defer cancel()
	return b.Search.Search(sctx, platform, kw, since)
}

// draft asks the LLM and falls back to the template on any failure.
func (b *Builder) draft(ctx context.Context, wsID string, bp BrandProfile, g Goal, platforms []string) Proposal {
	tmpl := TemplateProposal(g, bp, platforms)
	if b.Gen == nil {
		return tmpl
	}
	gctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()
	var problems []string
	for _, pp := range bp.PainPoints {
		problems = append(problems, pp.Problem)
	}
	d, err := b.Gen.GenerateListener(gctx, wsID, ai.ListenerPrompt{
		Goal: string(g), Product: bp.Product, Overview: bp.Overview, Audience: bp.TargetAudience,
		Competitors: bp.Competitors, PainPoints: head(problems, 8), Keywords: head(bp.AllKeywords(), 15),
	})
	if err != nil || d == nil {
		return tmpl
	}
	kws := cleanList(d.Keywords, maxListenerKeywords)
	if len(kws) == 0 {
		return tmpl
	}
	p := tmpl
	p.Keywords = kws
	if name := capLen(strings.TrimSpace(d.Name), 80); name != "" {
		p.Name = name
	}
	if lf := capLen(strings.TrimSpace(d.LooksFor), maxTextLen); lf != "" {
		p.LooksFor = lf
	}
	if sk := capLen(strings.TrimSpace(d.Skips), maxTextLen); sk != "" {
		p.Skips = sk
	}
	return p
}

// keepProductive drops keywords that found nothing anywhere, unless none
// found anything (then the user decides; nothing to rank by).
func keepProductive(keywords []string, hits map[string]int) []string {
	var out []string
	for _, k := range keywords {
		if hits[k] > 0 {
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return keywords
	}
	return out
}

func orDur(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}
