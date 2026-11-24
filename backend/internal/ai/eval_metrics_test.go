package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// evalMetrics is a confusion matrix for "is this a lead?" plus what the
// calls cost. The live feedback eval (feedback_eval_live_test.go) fills one
// per workspace, per label source and overall.
type evalMetrics struct {
	N         int     `json:"n"`
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	TN        int     `json:"tn"`
	FN        int     `json:"fn"`
	Errors    int     `json:"errors"` // no result (API error, refusal); not in the matrix
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	FPRate    float64 `json:"false_positive_rate"`
	CostUSD   float64 `json:"cost_usd"`
	Tokens    Usage   `json:"tokens"`
}

// add counts one case: want and got are "is a lead".
func (m *evalMetrics) add(want, got bool) {
	m.N++
	switch {
	case want && got:
		m.TP++
	case !want && got:
		m.FP++
	case want && !got:
		m.FN++
	default:
		m.TN++
	}
}

// finish fills the ratios. An empty denominator gives NaN-free 0 so the
// report stays valid JSON; read it next to the counts.
func (m *evalMetrics) finish() {
	m.Precision = ratio(m.TP, m.TP+m.FP)
	m.Recall = ratio(m.TP, m.TP+m.FN)
	m.FPRate = ratio(m.FP, m.FP+m.TN)
	m.CostUSD = math.Round(m.CostUSD*1e4) / 1e4
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return math.Round(float64(a)/float64(b)*1e4) / 1e4
}

// evalPrices are USD per 1M tokens {input, output}, copied from
// llm/cost.go (ai cannot import llm). Cache reads bill 0.1x, writes 1.25x.
var evalPrices = map[string][2]float64{
	"claude-opus-5-5":   {4, 20},
	"claude-sonnet-5-5": {2, 10},
	"claude-haiku-5-5":  {0.10, 0.50},
}

// evalBudget checks the run can be costed and capped before any call is made:
// a model missing from evalPrices would cost $0 and never trip the cap, and
// an unparsable cap would silently become 0.
func evalBudget(model, rawMax string) (float64, error) {
	if _, ok := evalPrices[model]; !ok {
		return 0, fmt.Errorf("no price for model %q in evalPrices: add it so EVAL_MAX_USD can cap the run", model)
	}
	maxUSD, err := strconv.ParseFloat(strings.TrimSpace(rawMax), 64)
	if err != nil || math.IsNaN(maxUSD) || math.IsInf(maxUSD, 0) || maxUSD <= 0 {
		return 0, fmt.Errorf("EVAL_MAX_USD = %q: want a positive number of dollars", rawMax)
	}
	return maxUSD, nil
}

func evalCostUSD(model string, u Usage) float64 {
	p := evalPrices[model]
	in := float64(u.PromptTokens) + 0.1*float64(u.CacheReadTokens) + 1.25*float64(u.CacheWriteTokens)
	return (in*p[0] + float64(u.CompletionTokens)*p[1]) / 1e6
}

// promptVariants are the cumulative prompt ablations EVAL_VARIANT picks:
// each adds one part of the per-workspace prompt to the one before it.
// "full" is the prompt production sends. "story" is "full" plus storyRule in
// the base prompt; "v1" is the prompt before the open-need change
// (v1System); "v2card" is "full" with a specific ask and reasoning
// (cardSystem).
var promptVariants = []string{"base", "product", "buyers", "looks", "rules", "full", "story", "v1", "v2card"}

// storyRule is a candidate base-prompt hard rule aimed at the dominant false
// positive: a first-hand story scored as a buyer's complaint. It lives here,
// not in classify.go, until the eval says it earns its place.
const storyRule = `- An author telling their own story is not shopping. A build-in-public or progress update, an opinion or hot take (a recruiter's or manager's view of the market included), venting about a product or vendor they use or used, or a technical comment or answer, with no ask for a tool, a vendor or help with the problem, is not buying intent and its author is not a buyer: set author_role "non_buyer" and keep relevance_score at 4 or below. When the author also asks for a tool, a vendor, or how others solve it, score the ask as usual.`

// storyAnchor is the hard rule storyRule is inserted after.
const storyAnchor = "- Only tag buying_intent / seeking_alternative / recommendation_request when the AUTHOR is the one looking.\n"

// storyBasePrompt is classifyBasePrompt with storyRule added to the hard rules.
func storyBasePrompt() (string, error) {
	if !strings.Contains(classifyBasePrompt, storyAnchor) {
		return "", fmt.Errorf("story variant: anchor rule not found in classifyBasePrompt")
	}
	return strings.Replace(classifyBasePrompt, storyAnchor, storyAnchor+storyRule+"\n", 1), nil
}

// The "v2card" variant: ask and reasoning are written for the person who
// will reply (signal spec section 4: who, ask, why now, fit). Measured only
// together with candidate rules and examples (EVAL_RULES_FILE,
// EVAL_EXAMPLES_FILE), where it did not change precision or recall.
// A base-prompt fit rule (an ask the product does not answer, or an author
// with no budget, scores 5 at most) was tried with it on the train split and
// left out: no false positive remained, but it hid six gold leads (price
// shoppers, bring-your-own-key requests, "free" asks with a real angle).
const cardAsk = `- "ask": what the author needs, as one plain sentence in their terms (max 30 words), specific enough to answer without opening the post: the job to be done plus the one constraint that matters to them (the tool they use now, their scale, platform, price or deadline), e.g. "Meeting transcription for one person at about 10 pounds a month that copes with a Scottish accent" or "To tell which candidates use AI during a live interview, for a team hiring two backend engineers". Say only what the post says. When the author voices a frustration or objection and asks for nothing, state it as theirs ("Take-home tests get done by AI and tell us nothing") instead of inventing a request ("A way to ..."). Write it whoever the author is: a buyer, or someone on the other side of the problem (for a hiring product, a candidate saying what went wrong in how they were assessed), because both say what the market wants. Use "" when no need is voiced: news and reports of what some company does, jokes, asking for moral support, opinion with no need behind it, a need that has nothing to do with the product's problem, and someone promoting or building their own tool`

const cardReasoning = `- "reasoning": two short sentences for the person who will reply. First, who the author is in the post's own terms (role, setting, scale: "solo founder of a real-estate SaaS with a few paying users"; "unknown" things are left out, not guessed). Second, why THE PRODUCT answers what is open for them, or what rules the post out (nothing open, not a buyer, wants it free, wrong platform, a different kind of tool). Plain statements: no "though", "however" or "may", and do not describe THE PRODUCT`

// cardSystem is classifySystem with cardAsk and cardReasoning in place of
// the ask and reasoning fields.
func cardSystem(p Provider, sc ScoringContext) ([]chatMessage, error) {
	msgs := classifySystem(p, sc)
	c := msgs[0].Content // the base prompt comes first for every provider
	for _, r := range [][2]string{{`- "ask": `, cardAsk}, {`- "reasoning": `, cardReasoning}} {
		a := strings.Index(c, r[0])
		if a < 0 {
			return nil, fmt.Errorf("v2card variant: field %q not found in the base prompt", r[0])
		}
		c = c[:a] + r[1] + c[a+strings.Index(c[a:], "\n"):]
	}
	msgs[0].Content = c
	return msgs, nil
}

// v1Band is the 6-7.5 score band before the open-need change (scoring rubric
// 4), which made any first-hand account by a buyer a lead.
const v1Band = `- 6-7.5: the author speaks as a buyer about their own team or company and describes the exact problem THE PRODUCT solves, with no request for a tool. This includes describing the workaround they built for it ("we added a step to our process because ..."), and saying they cannot do or cannot trust the thing THE PRODUCT does ("I can't tell whether ...") even when they add their own idea for a fix: they have the problem and say so in public, so they are worth writing to. The form does not matter: an essay or opinion post counts when the author reports from their own work what does not work today (the method they dropped, the people it let through or kept out, the time it cost), and so does one that ends by asking the audience what they think. A buyer who says they are doing this work right now (hiring for a role, going through candidates this month) and names what they find lacking belongs here even when most of the post is then opinion. Score 7 or 7.5, not 6, when the post gives a number for the problem or says what it costs them (how many, how often, how long). Tag it pain_point, with post_type "discussion".
`

// v1System is the classifier prompt before the open-need change: no
// open_need or why_now field, no open-need rule, and v1Band. The model then
// returns no open_need, so the verdict falls back to score and intent tag,
// as production decided it then. Kept so the change can be measured again.
func v1System(p Provider, sc ScoringContext) ([]chatMessage, error) {
	msgs := classifySystem(p, sc)
	for _, part := range []string{openNeedFields, openNeedRule} {
		if !strings.Contains(msgs[0].Content, part) {
			return nil, fmt.Errorf("v1 variant: open-need part not found in the base prompt")
		}
		msgs[0].Content = strings.Replace(msgs[0].Content, part, "", 1)
	}
	if sc.Product == "" {
		return msgs, nil
	}
	last := &msgs[len(msgs)-1].Content
	if !strings.Contains(*last, unsolvedPainBand) {
		return nil, fmt.Errorf("v1 variant: 6-7.5 band not found in productFitRules")
	}
	*last = strings.Replace(*last, unsolvedPainBand, v1Band, 1)
	return msgs, nil
}

// classifyVariant is ClassifyIntent with the variant's system prompt: "story"
// swaps in storyBasePrompt, "v1" v1System and "v2card" cardSystem. The tail
// mirrors ClassifyIntent; TestStorySystem pins it.
func classifyVariant(ctx context.Context, p Provider, title, content, platform string, sc ScoringContext, variant string) (*ClassifyResult, error) {
	var msgs []chatMessage
	var err error
	switch variant {
	case "story":
		var base string
		if base, err = storyBasePrompt(); err == nil {
			msgs = storySystem(p, sc, base)
		}
	case "v1":
		msgs, err = v1System(p, sc)
	case "v2card":
		msgs, err = cardSystem(p, sc)
	default:
		return ClassifyIntent(ctx, p, title, content, platform, sc)
	}
	if err != nil {
		return nil, err
	}
	msgs = append(msgs, chatMessage{Role: "user", Content: fmt.Sprintf("Platform: %s\nTitle: %s\nContent: %s", platform, title, content)})
	result, err := callChat(ctx, p, msgs, 0.1)
	if err != nil {
		return nil, err
	}
	var cr ClassifyResult
	if err := json.Unmarshal([]byte(stripCodeFences(result)), &cr); err != nil {
		return nil, fmt.Errorf("parse classification result: %w (raw: %s)", err, result)
	}
	cr = NormalizeClassifyResult(cr, title)
	return &cr, nil
}

// storySystem is classifySystem with base in place of classifyBasePrompt.
func storySystem(p Provider, sc ScoringContext, base string) []chatMessage {
	msgs := classifySystem(p, sc)
	for i := range msgs {
		msgs[i].Content = strings.Replace(msgs[i].Content, classifyBasePrompt, base, 1)
	}
	return msgs
}

// ablate keeps only the parts of sc that variant includes. "full" (or "")
// is sc unchanged, as production sends it; "story" too (its change is in
// the base prompt, see classifyVariant).
func ablate(sc ScoringContext, variant string) (ScoringContext, error) {
	var out ScoringContext
	switch variant {
	case "", "full", "story", "v1", "v2card":
		return sc, nil
	case "rules":
		out.Rules = sc.Rules
		fallthrough
	case "looks":
		out.LooksFor, out.Skips = sc.LooksFor, sc.Skips
		fallthrough
	case "buyers":
		out.Buyers = sc.Buyers
		fallthrough
	case "product":
		out.Product = sc.Product
		fallthrough
	case "base":
		return out, nil
	}
	return out, fmt.Errorf("unknown variant %q (want one of %v)", variant, promptVariants)
}

// stratifiedSample picks about n of the items, the same ones on every run:
// each stratum gets its proportional share (largest remainder), and within
// a stratum items are taken in FNV order of their id. Returns indices in
// input order.
func stratifiedSample(ids, strata []string, n int) []int {
	if n <= 0 || n >= len(ids) {
		all := make([]int, len(ids))
		for i := range all {
			all[i] = i
		}
		return all
	}
	by := map[string][]int{}
	var keys []string
	for i, s := range strata {
		if by[s] == nil {
			keys = append(keys, s)
		}
		by[s] = append(by[s], i)
	}
	sort.Strings(keys)
	take := map[string]int{}
	type rem struct {
		k string
		r float64
	}
	var rems []rem
	left := n
	for _, k := range keys {
		exact := float64(len(by[k])) * float64(n) / float64(len(ids))
		take[k] = int(exact)
		left -= take[k]
		rems = append(rems, rem{k, exact - float64(take[k])})
	}
	sort.SliceStable(rems, func(i, j int) bool { return rems[i].r > rems[j].r })
	for i := 0; i < left; i++ {
		take[rems[i].k]++
	}
	hash := func(s string) uint32 { h := fnv.New32a(); h.Write([]byte(s)); return h.Sum32() }
	var out []int
	for _, k := range keys {
		idx := by[k]
		sort.Slice(idx, func(i, j int) bool { return hash(ids[idx[i]]) < hash(ids[idx[j]]) })
		out = append(out, idx[:take[k]]...)
	}
	sort.Ints(out)
	return out
}

func TestAblate(t *testing.T) {
	sc := ScoringContext{Product: "p", Buyers: "b", LooksFor: "l", Skips: "s", Rules: []string{"r"}, Examples: []ScoringExample{{Title: "e"}}}
	want := map[string]ScoringContext{
		"base":    {},
		"product": {Product: "p"},
		"buyers":  {Product: "p", Buyers: "b"},
		"looks":   {Product: "p", Buyers: "b", LooksFor: "l", Skips: "s"},
		"rules":   {Product: "p", Buyers: "b", LooksFor: "l", Skips: "s", Rules: []string{"r"}},
		"full":    sc,
		"story":   sc,
		"v1":      sc,
		"v2card":  sc,
	}
	for _, v := range promptVariants {
		got, err := ablate(sc, v)
		if err != nil || buildClassifyPrompt(got) != buildClassifyPrompt(want[v]) {
			t.Errorf("%s: got %+v err %v", v, got, err)
		}
	}
	if _, err := ablate(sc, "nope"); err == nil {
		t.Error("unknown variant should error")
	}
}

func TestStorySystem(t *testing.T) {
	sc := ScoringContext{Product: "p", Buyers: "b", Rules: []string{"r"}}
	for _, name := range []string{"anthropic", "openai"} {
		p := Provider{Name: name}
		if got, want := storySystem(p, sc, classifyBasePrompt), classifySystem(p, sc); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s: with the production base, storySystem must equal classifySystem", name)
		}
		base, err := storyBasePrompt()
		if err != nil {
			t.Fatal(err)
		}
		got := storySystem(p, sc, base)
		if !strings.Contains(got[0].Content, storyAnchor+storyRule) || !strings.Contains(fmt.Sprint(got), "WHO BUYS") {
			t.Fatalf("%s: story rule or listener part missing", name)
		}
	}
}

func TestV1System(t *testing.T) {
	sc := ScoringContext{Product: "p", Buyers: "b", Rules: []string{"r"}}
	for _, name := range []string{"anthropic", "openai"} {
		got, err := v1System(Provider{Name: name}, sc)
		if err != nil {
			t.Fatal(err)
		}
		all := fmt.Sprint(got)
		for _, gone := range []string{"open_need", "why_now", "unsolved today"} {
			if strings.Contains(all, gone) {
				t.Fatalf("%s: v1 prompt still has %q", name, gone)
			}
		}
		for _, want := range []string{"- \"ask\": ", "- \"matches_skips\": boolean", storyAnchor + "\nPost type:", v1Band + "- 4-5.5:", "WHO BUYS", "- 8-10:"} {
			if !strings.Contains(all, want) {
				t.Fatalf("%s: v1 prompt lacks %q", name, want)
			}
		}
	}
	// With no product there is no score band to swap.
	if got, err := v1System(Provider{Name: "anthropic"}, ScoringContext{}); err != nil || strings.Contains(fmt.Sprint(got), "open_need") {
		t.Fatalf("no product: %v", err)
	}
}

func TestCardSystem(t *testing.T) {
	sc := ScoringContext{Product: "p", Buyers: "b", Rules: []string{"r"}}
	for _, name := range []string{"anthropic", "openai"} {
		got, err := cardSystem(Provider{Name: name}, sc)
		if err != nil {
			t.Fatal(err)
		}
		all := fmt.Sprint(got)
		for _, want := range []string{cardAsk + "\n" + openNeedFields, cardReasoning + "\n- \"awareness_level\"", openNeedRule, unsolvedPainBand, "WHO BUYS"} {
			if !strings.Contains(all, want) {
				t.Fatalf("%s: v2card prompt lacks %q", name, want[:40])
			}
		}
		if strings.Contains(all, "brief one-sentence explanation") || strings.Count(all, `- "ask": `) != 1 {
			t.Fatalf("%s: the old ask or reasoning field is still in the prompt", name)
		}
	}
}

func TestStratifiedSample(t *testing.T) {
	var ids, strata []string
	for i := 0; i < 100; i++ {
		ids = append(ids, fmt.Sprint("c", i))
		strata = append(strata, map[bool]string{true: "a", false: "b"}[i < 70]) // 70 a, 30 b
	}
	got := stratifiedSample(ids, strata, 10)
	if len(got) != 10 {
		t.Fatalf("len %d", len(got))
	}
	a := 0
	for _, i := range got {
		if strata[i] == "a" {
			a++
		}
	}
	if a != 7 {
		t.Fatalf("stratum a got %d of 10, want 7", a)
	}
	if again := stratifiedSample(ids, strata, 10); fmt.Sprint(again) != fmt.Sprint(got) {
		t.Fatalf("not deterministic: %v vs %v", again, got)
	}
	if len(stratifiedSample(ids, strata, 0)) != 100 {
		t.Fatal("n=0 should keep everything")
	}
}

func TestEvalMetrics(t *testing.T) {
	var m evalMetrics
	// 3 TP, 1 FP, 4 TN, 2 FN
	for _, c := range []struct{ want, got bool }{
		{true, true}, {true, true}, {true, true},
		{false, true},
		{false, false}, {false, false}, {false, false}, {false, false},
		{true, false}, {true, false},
	} {
		m.add(c.want, c.got)
	}
	m.finish()
	if m.N != 10 || m.TP != 3 || m.FP != 1 || m.TN != 4 || m.FN != 2 {
		t.Fatalf("counts = %+v", m)
	}
	if m.Precision != 0.75 || m.Recall != 0.6 || m.FPRate != 0.2 {
		t.Fatalf("precision %v recall %v fpr %v, want 0.75 0.6 0.2", m.Precision, m.Recall, m.FPRate)
	}

	var empty evalMetrics
	empty.finish()
	if empty.Precision != 0 || empty.Recall != 0 || empty.FPRate != 0 {
		t.Fatalf("empty metrics should be 0, got %+v", empty)
	}

	// 1M uncached in + 1M cache read + 1M out on Opus: 4 + 0.4 + 20.
	got := evalCostUSD("claude-opus-5-5", Usage{PromptTokens: 1e6, CacheReadTokens: 1e6, CompletionTokens: 1e6})
	if math.Abs(got-24.4) > 1e-9 {
		t.Fatalf("cost = %v, want 24.4", got)
	}
}

// evalCase is one line of cases.jsonl.
type evalCase struct {
	ID        string `json:"id"`
	Workspace string `json:"workspace"` // anonymised, e.g. ws_03
	Context   string `json:"context"`   // key into contexts.json
	Platform  string `json:"platform"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Label     string `json:"label"`  // lead | not_lead
	Source    string `json:"source"` // where the label came from
	Reason    string `json:"reason,omitempty"`
}

// withoutCase drops a few-shot example that is the case itself, so a post
// the workspace already labelled is not scored with its own answer. It keeps
// only the examples production would send (the first MaxScoringExamples), so
// a dropped example leaves its slot empty instead of letting a 7th in.
// Posts match the way formatExamples renders them (whitespace collapsed), or
// on the start of title and body joined.
func withoutCase(sc ScoringContext, c evalCase) ScoringContext {
	// Review examples are stored with an empty title and "title body" as the
	// content, so the fields rarely line up with the case's: compare the start
	// of the joined text too.
	joined := func(title, content string) string {
		return strings.TrimSuffix(truncate(title+" "+content, 300), "…")
	}
	same := func(e ScoringExample) bool {
		if truncate(e.Title, 150) == truncate(c.Title, 150) && truncate(e.Content, 400) == truncate(c.Content, 400) {
			return true
		}
		a, b := joined(e.Title, e.Content), joined(c.Title, c.Content)
		return len(a) >= 40 && len(b) >= 40 && (strings.HasPrefix(a, b) || strings.HasPrefix(b, a))
	}
	in := sc.Examples
	if len(in) > MaxScoringExamples {
		in = in[:MaxScoringExamples]
	}
	var ex []ScoringExample
	for _, e := range in {
		if !same(e) {
			ex = append(ex, e)
		}
	}
	sc.Examples = ex
	return sc
}

func loadEvalSet(t *testing.T, dir string) ([]evalCase, map[string]ScoringContext) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "contexts.json"))
	if err != nil {
		t.Fatal(err)
	}
	contexts := map[string]ScoringContext{}
	if err := json.Unmarshal(raw, &contexts); err != nil {
		t.Fatalf("contexts.json: %v", err)
	}
	name := os.Getenv("EVAL_CASES") // e.g. gold_test.jsonl
	if name == "" {
		name = "cases.jsonl"
	}
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []evalCase
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var c evalCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatalf("%s line %d: %v", name, len(cases)+1, err)
		}
		if c.Label != "lead" && c.Label != "not_lead" {
			t.Fatalf("%s: label %q", c.ID, c.Label)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cases, contexts
}

func TestEvalBudget(t *testing.T) {
	if got, err := evalBudget(classifyEvalModel, "8"); err != nil || got != 8 {
		t.Fatalf("default = %v, %v", got, err)
	}
	if got, err := evalBudget("claude-haiku-5-5", " 2.5 "); err != nil || got != 2.5 {
		t.Fatalf("haiku 2.5 = %v, %v", got, err)
	}
	for _, c := range []struct{ model, max string }{
		{"claude-unknown-9", "8"}, // would cost $0 and never trip the cap
		{classifyEvalModel, "8usd"},
		{classifyEvalModel, "$8"},
		{classifyEvalModel, "0"},
		{classifyEvalModel, "-1"},
		{classifyEvalModel, "NaN"},
		{classifyEvalModel, "Inf"},
	} {
		if _, err := evalBudget(c.model, c.max); err == nil {
			t.Errorf("evalBudget(%q, %q) = nil error", c.model, c.max)
		}
	}
}

func TestWithoutCase(t *testing.T) {
	var all []ScoringExample
	for i := range MaxScoringExamples + 2 {
		all = append(all, ScoringExample{Title: fmt.Sprintf("t%d", i), Content: fmt.Sprintf("body %d", i), Label: "up"})
	}
	titles := func(ex []ScoringExample) string {
		var s []string
		for _, e := range ex {
			s = append(s, e.Title)
		}
		return strings.Join(s, ",")
	}
	sc := ScoringContext{Examples: all}

	// The case is example 2 with different whitespace: dropped, and the
	// 7th example does not move up into the prompt.
	got := withoutCase(sc, evalCase{Title: " t2 ", Content: "body\n\n  2"})
	if titles(got.Examples) != "t0,t1,t3,t4,t5" {
		t.Fatalf("examples = %s", titles(got.Examples))
	}
	if len(sc.Examples) != MaxScoringExamples+2 || sc.Examples[2].Title != "t2" {
		t.Fatal("input context was changed")
	}

	// A review example keeps the post as "title body" under an empty title.
	post := "How do you take meeting notes when you run every call yourself"
	merged := ScoringContext{Examples: []ScoringExample{{Content: post + " " + post + " and more of the body", Label: "up"}, all[0]}}
	if got := withoutCase(merged, evalCase{Title: post, Content: post + " and more of the body"}); titles(got.Examples) != "t0" {
		t.Fatalf("merged-title example not dropped: %d left", len(got.Examples))
	}

	// No match: production's first six, unchanged.
	got = withoutCase(sc, evalCase{Title: "t2", Content: "other"})
	if titles(got.Examples) != "t0,t1,t2,t3,t4,t5" {
		t.Fatalf("no match: examples = %s", titles(got.Examples))
	}
}

func TestLoadEvalSet(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("contexts.json", `{"c1":{"Product":"Uptime","Examples":[{"Title":"a","Content":"b","Label":"up"}]}}`)
	write("cases.jsonl", `{"id":"x1","workspace":"ws_01","context":"c1","title":"T","content":"C","label":"lead","source":"feedback"}

{"id":"x2","workspace":"ws_02","context":"c1","label":"not_lead","source":"review"}
`)
	cases, contexts := loadEvalSet(t, dir)
	if len(cases) != 2 || cases[0].ID != "x1" || cases[0].Title != "T" || cases[1].Label != "not_lead" {
		t.Fatalf("cases = %+v", cases)
	}
	if c := contexts["c1"]; c.Product != "Uptime" || len(c.Examples) != 1 || c.Examples[0].Label != "up" {
		t.Fatalf("contexts = %+v", contexts)
	}
}
