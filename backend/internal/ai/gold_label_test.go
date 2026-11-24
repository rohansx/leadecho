package ai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Helpers for the gold labeller (gold_label_live_test.go): a stronger model
// labels the eval set against testdata/signal_spec_v0.md, then adjudicates
// its disagreements with the labels the set already had.

// goldCase is a post to label: an eval case, or an unlabelled pool post
// (Label "", Bucket = where in the score range production put it).
type goldCase struct {
	evalCase
	Bucket string `json:"bucket,omitempty"`
}

// goldLabel is what the labelling model returns.
type goldLabel struct {
	Label      string  `json:"label"` // lead | not_lead
	SignalType string  `json:"signal_type"`
	BuyerFit   string  `json:"buyer_fit"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
	Ask        string  `json:"ask"`
	WhyNow     string  `json:"why_now"`
	Rationale  string  `json:"rationale,omitempty"` // adjudication only: why this label over the other
}

// goldRow is one line of fable_labels.jsonl / fable_adjudications.jsonl.
type goldRow struct {
	ID        string `json:"id"`
	Workspace string `json:"workspace"`
	goldLabel
	Model    string  `json:"model"`
	ServedBy string  `json:"served_by,omitempty"` // differs from Model when a fallback model answered
	Effort   string  `json:"effort"`
	Refused  bool    `json:"refused,omitempty"`
	Err      string  `json:"error,omitempty"`
	CostUSD  float64 `json:"cost_usd"`
	Tokens   Usage   `json:"tokens"`
}

func (r goldRow) ok() bool { return r.Err == "" && !r.Refused }

var goldSignalTypes = []string{"asking_for_tool", "evaluating_or_comparing", "switching_away", "open_pain_with_intent", "hiring_or_scaling_trigger", "not_a_lead"}

// goldSchema is the structured-output schema; adjudication adds "rationale".
func goldSchema(adjudicate bool) map[string]any {
	str := map[string]any{"type": "string"}
	props := map[string]any{
		"label":       map[string]any{"type": "string", "enum": []string{"lead", "not_lead"}},
		"signal_type": map[string]any{"type": "string", "enum": goldSignalTypes},
		"buyer_fit":   map[string]any{"type": "string", "enum": []string{"strong", "possible", "no"}},
		"confidence":  map[string]any{"type": "number"},
		"reason":      str, "ask": str, "why_now": str,
	}
	req := []string{"label", "signal_type", "buyer_fit", "confidence", "reason", "ask", "why_now"}
	if adjudicate {
		props["rationale"] = str
		req = append(req, "rationale")
	}
	return map[string]any{"type": "object", "properties": props, "required": req, "additionalProperties": false}
}

// check rejects an answer the spec does not allow, so a malformed label is
// retried or logged instead of landing in the gold set.
func (l goldLabel) check() error {
	switch {
	case l.Label != "lead" && l.Label != "not_lead":
		return fmt.Errorf("label %q", l.Label)
	case (l.Label == "not_lead") != (l.SignalType == "not_a_lead"):
		return fmt.Errorf("label %q with signal_type %q", l.Label, l.SignalType)
	case l.Label == "lead" && l.BuyerFit == "no":
		return fmt.Errorf("lead with buyer_fit no")
	case l.Confidence < 0 || l.Confidence > 1:
		return fmt.Errorf("confidence %v", l.Confidence)
	}
	return nil
}

// goldWorkspace renders what the labeller knows about the customer. The
// workspace's few-shot examples are left out on purpose: they are model
// labels (and some are eval cases), and the gold label must not copy them.
func goldWorkspace(sc ScoringContext) string {
	var b strings.Builder
	b.WriteString("# The workspace\n\nProduct (and the listener that found the post, when known):\n" + strings.TrimSpace(sc.Product) + "\n")
	for _, kv := range [][2]string{{"Buyers", sc.Buyers}, {"Looks for", sc.LooksFor}, {"Skips", sc.Skips}} {
		if v := strings.TrimSpace(kv[1]); v != "" {
			b.WriteString("\n" + kv[0] + ": " + v + "\n")
		}
	}
	if len(sc.Rules) > 0 {
		b.WriteString("\nWorkspace notes (learned from this customer's past posts; hints, not definitions):\n")
		for _, r := range sc.Rules {
			b.WriteString("- " + strings.TrimSpace(r) + "\n")
		}
	}
	return b.String()
}

func goldPost(c goldCase) string {
	s := "Platform: " + c.Platform + "\n"
	if c.Title != "" {
		s += "Title: " + c.Title + "\n"
	}
	return s + "\n<post>\n" + c.Content + "\n</post>"
}

// needsAdjudication: the first pass disagrees with the label the case
// already had, or is not sure. Pool posts have no earlier label.
func needsAdjudication(c goldCase, first goldRow) bool {
	return first.ok() && ((c.Label != "" && c.Label != first.Label) || first.Confidence < 0.7)
}

// goldAdjudicatePrompt shows the second pass both labels and their reasons.
func goldAdjudicatePrompt(c goldCase, first goldRow) string {
	var b strings.Builder
	b.WriteString(goldPost(c) + "\n\nThis post was labelled before. Decide the final label under the spec.\n\n")
	if c.Label != "" {
		who := map[string]string{
			"human":          "A human at the customer (an action in the product: approved a reply, moved the lead forward, or archived the post). Their action is strong evidence of what this customer wants; go against it only for a strong, specific reason from the post and the spec, and state that reason in `rationale`.",
			"review_example": "An earlier model review of this workspace's posts.",
			"agent_blind":    "An earlier model labeller using a looser one-line bar (plausible buyer with the problem now).",
		}[c.Source]
		b.WriteString("Label A: " + c.Label + "\nFrom: " + who + "\n")
		if c.Reason != "" {
			b.WriteString("Its note: " + c.Reason + "\n")
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Label B: %s (signal_type %s, buyer_fit %s, confidence %.2f)\nFrom: a first pass under this spec.\nIts reason: %s\n",
		first.Label, first.SignalType, first.BuyerFit, first.Confidence, first.Reason)
	if first.Ask != "" {
		b.WriteString("Its ask: " + first.Ask + "\n")
	}
	b.WriteString("\nNeither label is authoritative. Re-read the post, test both conditions of the spec, and return the final fields. In `rationale`, say in one or two sentences why the final label is right and the other reading is wrong.")
	return b.String()
}

// fablePrices are USD per 1M tokens: input, output, cache read, cache write (5 min).
var fablePrices = [4]float64{8, 40, 0.20, 10}

func goldCostUSD(u Usage) float64 {
	p := fablePrices
	return (float64(u.PromptTokens)*p[0] + float64(u.CompletionTokens)*p[1] + float64(u.CacheReadTokens)*p[2] + float64(u.CacheWriteTokens)*p[3]) / 1e6
}

// readJSONL reads one JSON value per line into a slice; a missing file is empty.
func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 4<<20)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var v T
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, len(out)+1, err)
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

func TestGoldHelpers(t *testing.T) {
	lead := goldLabel{Label: "lead", SignalType: "asking_for_tool", BuyerFit: "strong", Confidence: 0.9}
	if err := lead.check(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []goldLabel{
		{Label: "maybe"},
		{Label: "lead", SignalType: "not_a_lead", BuyerFit: "strong"},
		{Label: "not_lead", SignalType: "asking_for_tool"},
		{Label: "lead", SignalType: "asking_for_tool", BuyerFit: "no"},
		{Label: "not_lead", SignalType: "not_a_lead", Confidence: 1.2},
	} {
		if bad.check() == nil {
			t.Errorf("check accepted %+v", bad)
		}
	}

	row := func(label string, conf float64) goldRow {
		return goldRow{goldLabel: goldLabel{Label: label, Confidence: conf}}
	}
	existing := goldCase{evalCase: evalCase{Label: "lead", Source: "human", Reason: "reply approved"}}
	pool := goldCase{}
	for _, tc := range []struct {
		c    goldCase
		r    goldRow
		want bool
	}{
		{existing, row("lead", 0.9), false},
		{existing, row("not_lead", 0.95), true}, // disagrees
		{existing, row("lead", 0.6), true},      // unsure
		{pool, row("lead", 0.9), false},         // no earlier label to disagree with
		{pool, row("not_lead", 0.5), true},
		{existing, goldRow{Refused: true}, false}, // nothing to adjudicate
	} {
		if got := needsAdjudication(tc.c, tc.r); got != tc.want {
			t.Errorf("needsAdjudication(%q, %+v) = %v, want %v", tc.c.Label, tc.r.goldLabel, got, tc.want)
		}
	}

	p := goldAdjudicatePrompt(existing, row("not_lead", 0.8))
	for _, want := range []string{"Label A: lead", "A human at the customer", "reply approved", "Label B: not_lead"} {
		if !strings.Contains(p, want) {
			t.Errorf("adjudication prompt lacks %q", want)
		}
	}
	if strings.Contains(goldAdjudicatePrompt(pool, row("lead", 0.5)), "Label A") {
		t.Error("pool post has no label A")
	}

	ws := goldWorkspace(ScoringContext{Product: "- P: d", Buyers: "CTOs", Rules: []string{"r1"}, Examples: []ScoringExample{{Content: "SECRET"}}})
	if !strings.Contains(ws, "Buyers: CTOs") || !strings.Contains(ws, "- r1") || strings.Contains(ws, "SECRET") || strings.Contains(ws, "Skips") {
		t.Errorf("goldWorkspace:\n%s", ws)
	}

	// 1M uncached in + 1M out + 1M cache read + 1M cache write
	if got := goldCostUSD(Usage{PromptTokens: 1e6, CompletionTokens: 1e6, CacheReadTokens: 1e6, CacheWriteTokens: 1e6}); got != 58.2 {
		t.Errorf("goldCostUSD = %v, want 58.2", got)
	}
}
