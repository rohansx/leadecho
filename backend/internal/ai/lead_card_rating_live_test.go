//go:build live

// Rates the lead cards of TestFeedbackEval reports. For every gold lead a
// report found, a stronger model (goldModel, effort high) reads the post, the
// gold ask and why-now, and the card the classifier wrote (ask, why now when
// the variant returns one, reasoning), and scores it 1-5 for specificity and
// for usefulness to the person who will reply. It is not told which prompt
// wrote the card. Run:
//
//	ANTHROPIC_API_KEY=... LEADECHO_EVAL_DIR=~/.config/workz/leadecho-evals EVAL_CASES=gold_test.jsonl \
//	  RATE_REPORTS=report-a.json,report-b.json RATE_MAX_USD=8 \
//	  go test -tags live -run 'TestLeadCardRating$' -v -count=1 -timeout 60m ./internal/ai/
//
// Each report gets a ratings-<report>.json next to it.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

const cardRubric = `You review lead cards written by a classifier for a sales tool. The tool finds public posts in which someone shows a need a product could answer, and shows the salesperson a card instead of the post: the ask, why it is timely (not every card has this line), and the reasoning.

You get the product, the post, a reference ask and why-now written by a careful reviewer, and the card. Rate the card on two scales, 1 to 5.

specificity: does the card carry the post's own facts?
5: names who the author is, the need and the constraint that matters (tool in use, scale, platform, price, deadline), all from the post, close to the reference
4: the need and one concrete fact from the post; a minor fact missing
3: the right need, stated generically; could describe many posts
2: vague or partly wrong about what the author wants
1: wrong, invented, or about the product instead of the author

usefulness: could a salesperson write a good first line of a reply from the card alone, without opening the post?
5: yes: who to address, what they need, why now, and how the product fits or where it falls short
4: yes, with one of those missing
3: only a generic reply is possible
2: the reply would miss what the author cares about
1: the card would mislead the reply

Judge only the card's content against the post and the reference. Do not reward length. A fact not in the post counts against the card. "note" is one sentence naming the main thing the card lacks or gets wrong ("" if nothing).`

type cardRating struct {
	ID          string  `json:"id"`
	Workspace   string  `json:"workspace"`
	Specificity int     `json:"specificity"`
	Usefulness  int     `json:"usefulness"`
	Note        string  `json:"note"`
	Refused     bool    `json:"refused,omitempty"`
	Err         string  `json:"error,omitempty"`
	CostUSD     float64 `json:"cost_usd"`
}

func TestLeadCardRating(t *testing.T) {
	key, dir, reports := os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("LEADECHO_EVAL_DIR"), os.Getenv("RATE_REPORTS")
	if key == "" || dir == "" || reports == "" {
		t.Skip("ANTHROPIC_API_KEY, LEADECHO_EVAL_DIR and RATE_REPORTS must be set")
	}
	maxUSD, err := strconv.ParseFloat(envOr("RATE_MAX_USD", "5"), 64)
	if err != nil || maxUSD <= 0 {
		t.Fatalf("RATE_MAX_USD = %q: want a positive number of dollars", os.Getenv("RATE_MAX_USD"))
	}
	_, contexts := loadEvalSet(t, dir)
	type goldCard struct {
		ID     string `json:"id"`
		Ask    string `json:"ask"`
		WhyNow string `json:"why_now"`
	}
	rows, err := readJSONL[goldCard](filepath.Join(dir, envOr("EVAL_CASES", "gold_test.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	gold := map[string]goldCard{}
	for _, g := range rows {
		gold[g.ID] = g
	}
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"specificity", "usefulness", "note"},
		"properties": map[string]any{"specificity": map[string]any{"type": "integer"}, "usefulness": map[string]any{"type": "integer"}, "note": map[string]any{"type": "string"}}}

	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Minute)
	defer cancel()
	client := anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(5)) // SDK retries 429, 5xx and connection errors with backoff
	var (
		mu    sync.Mutex
		spent float64
	)
	for _, name := range strings.Split(reports, ",") {
		name = strings.TrimSpace(name)
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		var rep struct {
			Variant string       `json:"variant"`
			Tag     string       `json:"tag"`
			Cases   []evalResult `json:"cases"`
		}
		if err := json.Unmarshal(raw, &rep); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var out []cardRating
		var wg sync.WaitGroup
		sem := make(chan struct{}, 8)
		cut := 0
		for _, r := range rep.Cases {
			g, ok := gold[r.ID]
			if r.Label != "lead" || !r.Got || !ok {
				continue // only gold leads the run found have a reference
			}
			mu.Lock()
			over := spent >= maxUSD
			mu.Unlock()
			if over {
				cut++
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				card := "ask: " + r.Ask
				if r.WhyNow != "" {
					card += "\nwhy now: " + r.WhyNow
				}
				card += "\nreasoning: " + r.Reasoning
				user := fmt.Sprintf("%s\n\n# Reference\nask: %s\nwhy now: %s\n\n# The card\n%s", goldPost(goldCase{evalCase: r.evalCase}), g.Ask, g.WhyNow, card)
				system := []anthropic.BetaTextBlockParam{
					{Text: cardRubric, CacheControl: anthropic.NewBetaCacheControlEphemeralParam()},
					{Text: goldWorkspace(contexts[r.Context])},
				}
				cr := cardRating{ID: r.ID, Workspace: r.Workspace}
				var u Usage
				_, cr.Refused, cr.Err = fableJSON(ctx, client, system, user, "high", schema, &u, func(text string) error {
					if err := json.Unmarshal([]byte(text), &cr); err != nil {
						return fmt.Errorf("bad JSON: %w", err)
					}
					if cr.Specificity < 1 || cr.Specificity > 5 || cr.Usefulness < 1 || cr.Usefulness > 5 {
						return fmt.Errorf("rating out of range: %d, %d", cr.Specificity, cr.Usefulness)
					}
					return nil
				})
				cr.CostUSD = goldCostUSD(u)
				mu.Lock()
				spent += cr.CostUSD
				out = append(out, cr)
				mu.Unlock()
			}()
		}
		wg.Wait()
		var n, spec, use, failed int
		for _, c := range out {
			if c.Err != "" { // refused or failed: unrated, logged, not in the mean
				failed++
				t.Logf("UNRATED %s %s", c.ID, c.Err)
				continue
			}
			n, spec, use = n+1, spec+c.Specificity, use+c.Usefulness
		}
		mean := func(sum int) float64 {
			if n == 0 {
				return 0
			}
			return float64(sum) / float64(n)
		}
		res := map[string]any{"report": name, "variant": rep.Variant, "tag": rep.Tag, "rated": n, "unrated": failed, "not_run_spend_cap": cut,
			"mean_specificity": mean(spec), "mean_usefulness": mean(use), "ratings": out}
		t.Logf("%-70s variant=%-7s tag=%-10s rated=%d unrated=%d specificity=%.2f usefulness=%.2f  (spent $%.2f)", name, rep.Variant, rep.Tag, n, failed, mean(spec), mean(use), spent)
		buf, _ := json.MarshalIndent(res, "", " ")
		if err := os.WriteFile(filepath.Join(dir, "ratings-"+name), buf, 0o600); err != nil {
			t.Fatal(err)
		}
		if cut > 0 {
			t.Errorf("%s: spend cap $%.2f reached with %d cards unrated", name, maxUSD, cut)
		}
	}
}
