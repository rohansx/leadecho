package ai

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// buyerShapes is testdata/buyer_shapes.json: real posts (authors anonymised)
// the classifier must put on the right side of the lead line. The live eval
// (classify_eval_live_test.go) scores them with a model.
type buyerShapes struct {
	Product string `json:"product"`
	Buyers  string `json:"buyers"`
	Cases   []struct {
		ID, Author, Platform, Title, Content, Note string
		Want                                       string  `json:"want"` // "lead" | "not_lead"
		MinScore                                   float64 `json:"min_score"`
		WantAsk                                    bool    `json:"want_ask"`
	} `json:"cases"`
}

func loadBuyerShapes(t *testing.T) buyerShapes {
	t.Helper()
	raw, err := os.ReadFile("testdata/buyer_shapes.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc buyerShapes
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// classifyEvalModel is the model production classifies on: llm routes
// TaskClassify to the strong tier, which on Anthropic (the only chat key in
// prod) is strongDefaultModel["anthropic"]. Live evals default to it.
const classifyEvalModel = "claude-opus-5-5"

// isLead mirrors monitor.qualifiesAsLead: 6 or more with a buying-type intent.
// NormalizeClassifyResult has already put the open need into both: nothing
// open is capped under the line, something open carries a buying-type tag.
func isLead(cr *ClassifyResult) bool {
	switch cr.Intent {
	case "buy_signal", "recommendation_ask", "complaint", "comparison":
		return cr.RelevanceScore >= 6
	}
	return false
}

var anonAuthor = regexp.MustCompile(`^author_\d+$`)

func TestBuyerShapesFixture(t *testing.T) {
	doc := loadBuyerShapes(t)
	if doc.Product == "" || doc.Buyers == "" || len(doc.Cases) < 10 {
		t.Fatalf("fixture incomplete: %d cases", len(doc.Cases))
	}
	leads := 0
	for _, c := range doc.Cases {
		if c.Want != "lead" && c.Want != "not_lead" {
			t.Errorf("%s: want = %q", c.ID, c.Want)
		}
		if c.Want == "lead" {
			leads++
		}
		if !anonAuthor.MatchString(c.Author) {
			t.Errorf("%s: author %q is not anonymised", c.ID, c.Author)
		}
		if len(c.Content) < 50 || strings.Contains(c.Content, "http") {
			t.Errorf("%s: content too short or carries a link", c.ID)
		}
	}
	if leads < 4 || leads == len(doc.Cases) {
		t.Errorf("want both leads and noise in the set, got %d leads of %d", leads, len(doc.Cases))
	}
}
