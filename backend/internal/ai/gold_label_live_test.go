//go:build live

// Gold labels for the classifier eval set. A stronger model than the one
// production scores with (claude-fable-5-2) labels every post in
// cases.jsonl and pool.jsonl against testdata/signal_spec_v0.md, with the
// workspace's product, buyers, Looks for, Skips and notes. A second pass at
// higher effort then adjudicates every case where the first pass disagrees
// with the label the set already had, or is unsure. Run:
//
//	ANTHROPIC_API_KEY=... LEADECHO_EVAL_DIR=~/.config/workz/leadecho-evals GOLD_MAX_USD=45 \
//	  go test -tags live -run 'TestGoldLabel$' -v -count=1 -timeout 180m ./internal/ai/
//	... GOLD_MAX_USD=25 go test -tags live -run 'TestGoldAdjudicate$' ...
//
// Both append to a file next to the set (fable_labels.jsonl,
// fable_adjudications.jsonl) and resume: a case with a good row is skipped,
// and GOLD_MAX_USD caps the total cost recorded in that file, earlier runs
// included. GOLD_LIMIT labels only N more cases. The set's build/gold.py
// turns the two files into gold.jsonl and the train/test split.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

const goldModel = "claude-fable-5-2"

func TestGoldLabel(t *testing.T) {
	runGold(t, "fable_labels.jsonl", "high", nil)
}

func TestGoldAdjudicate(t *testing.T) {
	dir := os.Getenv("LEADECHO_EVAL_DIR")
	rows, err := readJSONL[goldRow](filepath.Join(dir, "fable_labels.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	first := map[string]goldRow{}
	for _, r := range rows {
		if r.ok() {
			first[r.ID] = r
		}
	}
	runGold(t, "fable_adjudications.jsonl", "xhigh", first)
}

// runGold labels every case that has no good row in outName yet. With first
// set it is the adjudication pass: only cases needsAdjudication picks, each
// shown its first-pass label.
func runGold(t *testing.T, outName, effort string, first map[string]goldRow) {
	key, dir := os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("LEADECHO_EVAL_DIR")
	if key == "" || dir == "" {
		t.Skip("ANTHROPIC_API_KEY and LEADECHO_EVAL_DIR must be set")
	}
	maxUSD, err := strconv.ParseFloat(envOr("GOLD_MAX_USD", "5"), 64)
	if err != nil || maxUSD <= 0 {
		t.Fatalf("GOLD_MAX_USD = %q: want a positive number of dollars", os.Getenv("GOLD_MAX_USD"))
	}
	spec, err := os.ReadFile("testdata/signal_spec_v0.md")
	if err != nil {
		t.Fatal(err)
	}
	contexts := map[string]ScoringContext{}
	raw, err := os.ReadFile(filepath.Join(dir, "contexts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &contexts); err != nil {
		t.Fatalf("contexts.json: %v", err)
	}
	var cases []goldCase
	for _, name := range []string{"cases.jsonl", "pool.jsonl"} {
		cs, err := readJSONL[goldCase](filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, cs...)
	}

	out := filepath.Join(dir, outName)
	prev, err := readJSONL[goldRow](out)
	if err != nil {
		t.Fatal(err)
	}
	done, spent := map[string]bool{}, 0.0
	for _, r := range prev {
		spent += r.CostUSD
		done[r.ID] = done[r.ID] || r.ok()
	}
	limit, _ := strconv.Atoi(os.Getenv("GOLD_LIMIT"))
	groups := map[string][]goldCase{} // by context: one cached prompt prefix each
	todo := 0
	for _, c := range cases {
		if done[c.ID] || (first != nil && !needsAdjudication(c, first[c.ID])) || (limit > 0 && todo >= limit) {
			continue
		}
		if _, ok := contexts[c.Context]; !ok {
			t.Fatalf("%s: no context %q", c.ID, c.Context)
		}
		groups[c.Context] = append(groups[c.Context], c)
		todo++
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	t.Logf("%s: model %s effort %s, %d to do (%d done), $%.2f of $%.2f already spent", outName, goldModel, effort, todo, len(done), spent, maxUSD)

	f, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 170*time.Minute)
	defer cancel()
	client := anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(5)) // SDK retries 429, 5xx and connection errors with backoff
	var (
		mu                      sync.Mutex
		wg                      sync.WaitGroup
		n, refused, failed, cut int
	)
	sem := make(chan struct{}, 8)
	label := func(c goldCase) {
		system := []anthropic.BetaTextBlockParam{
			{Text: string(spec), CacheControl: anthropic.NewBetaCacheControlEphemeralParam()},
			{Text: goldWorkspace(contexts[c.Context]), CacheControl: anthropic.NewBetaCacheControlEphemeralParam()},
		}
		user := goldPost(c)
		if first != nil {
			user = goldAdjudicatePrompt(c, first[c.ID])
		}
		r := fableLabel(ctx, client, system, user, effort, first != nil)
		r.ID, r.Workspace = c.ID, c.Workspace
		line, _ := json.Marshal(r)
		mu.Lock()
		defer mu.Unlock()
		spent += r.CostUSD
		n++
		switch {
		case r.Refused:
			refused++
			t.Logf("REFUSED %s %s", c.ID, r.Err)
		case r.Err != "":
			failed++
			t.Logf("ERROR %s %s", c.ID, r.Err)
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			t.Errorf("write %s: %v", out, err)
		}
		if n%25 == 0 {
			t.Logf("%d/%d  $%.2f", n, todo, spent)
		}
	}
	over := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return spent >= maxUSD
	}
	for _, k := range keys {
		for i, c := range groups[k] {
			if over() {
				cut++
				continue
			}
			if i == 0 { // alone, so the rest of the group reads the prompt cache instead of each writing it
				wg.Wait()
				label(c)
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				label(c)
			}()
		}
	}
	wg.Wait()
	t.Logf("%s: labelled %d, refused %d, errors %d, not run (spend cap) %d, total spend in file $%.2f", outName, n-refused-failed, refused, failed, cut, spent)
	if cut > 0 {
		t.Errorf("spend cap $%.2f reached with %d cases left; raise GOLD_MAX_USD and re-run to resume", maxUSD, cut)
	}
}

// fableLabel makes one labelling call. A refusal comes back as Refused (the
// request asks for a server-side fallback model first); a failed or
// malformed answer is tried three times. Cost covers every attempt.
func fableLabel(ctx context.Context, client anthropic.Client, system []anthropic.BetaTextBlockParam, user, effort string, adjudicate bool) (r goldRow) {
	r = goldRow{Model: goldModel, Effort: effort}
	var u Usage
	defer func() { r.Tokens, r.CostUSD = u, goldCostUSD(u) }()
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 5 * time.Second)
		}
		// Thinking is always on for this model and sampling parameters are
		// rejected, so neither is sent; effort sets the depth.
		resp, err := client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
			Model:     goldModel,
			MaxTokens: 16000,
			System:    system,
			Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user))},
			OutputConfig: anthropic.BetaOutputConfigParam{
				Effort: anthropic.BetaOutputConfigEffort(effort),
				Format: anthropic.BetaJSONOutputFormatParam{Schema: goldSchema(adjudicate)},
			},
			Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
			Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
		})
		if err != nil {
			r.Err = err.Error()
			continue
		}
		// usage.iterations lists every attempt when a fallback ran; top-level
		// usage covers only the last. All priced at this model's rates (an
		// upper bound: fallback models cost less).
		// Read from the raw JSON: SDK v2.5.0 leaves the typed iteration
		// entries zero (their "model" is null on a plain message).
		type tok struct {
			In, Out, Read, Write int32
		}
		var raw struct {
			In         int32 `json:"input_tokens"`
			Out        int32 `json:"output_tokens"`
			Read       int32 `json:"cache_read_input_tokens"`
			Write      int32 `json:"cache_creation_input_tokens"`
			Iterations []struct {
				In    int32 `json:"input_tokens"`
				Out   int32 `json:"output_tokens"`
				Read  int32 `json:"cache_read_input_tokens"`
				Write int32 `json:"cache_creation_input_tokens"`
			} `json:"iterations"`
		}
		if err := json.Unmarshal([]byte(resp.Usage.RawJSON()), &raw); err != nil {
			r.Err = "usage: " + err.Error() // cost unknown: stop rather than spend blind
			return r
		}
		attempts := []tok{{raw.In, raw.Out, raw.Read, raw.Write}}
		if len(raw.Iterations) > 0 {
			attempts = attempts[:0]
			for _, it := range raw.Iterations {
				attempts = append(attempts, tok{it.In, it.Out, it.Read, it.Write})
			}
		}
		for _, a := range attempts {
			u.Add(Usage{PromptTokens: a.In, CompletionTokens: a.Out, CacheReadTokens: a.Read, CacheWriteTokens: a.Write})
		}
		r.ServedBy = string(resp.Model)
		switch resp.StopReason {
		case anthropic.BetaStopReasonRefusal: // the fallback refused too; retrying gets the same answer
			r.Refused, r.Err = true, fmt.Sprintf("refusal (category %q)", resp.StopDetails.Category)
			return r
		case anthropic.BetaStopReasonEndTurn:
		default:
			r.Err = "stop_reason " + string(resp.StopReason)
			continue
		}
		var text string
		for _, block := range resp.Content {
			if tb, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
				text += tb.Text
			}
		}
		var l goldLabel
		if err := json.Unmarshal([]byte(text), &l); err != nil {
			r.Err = "bad JSON: " + err.Error()
			continue
		}
		if err := l.check(); err != nil {
			r.Err = "invalid label: " + err.Error()
			continue
		}
		r.goldLabel, r.Err = l, ""
		return r
	}
	return r
}
