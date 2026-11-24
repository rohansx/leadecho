package ai

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeClassifyResult(t *testing.T) {
	tests := []struct {
		name          string
		in            ClassifyResult
		title         string
		wantTags      []string
		wantSentiment string
		wantIntent    string
		wantMaxScore  float64 // 0 = don't check
	}{
		{
			name:          "tags drive legacy intent",
			in:            ClassifyResult{IntentTags: []string{"recommendation_request", "pain_point"}, Sentiment: "negative", RelevanceScore: 8},
			wantTags:      []string{"recommendation_request", "pain_point"},
			wantSentiment: "negative",
			wantIntent:    "recommendation_ask",
		},
		{
			name:          "buying intent wins over comparison",
			in:            ClassifyResult{IntentTags: []string{"comparison", "buying_intent"}},
			wantTags:      []string{"comparison", "buying_intent"},
			wantSentiment: "neutral",
			wantIntent:    "buy_signal",
		},
		{
			name:          "unknown, duplicate and mis-cased tags are cleaned",
			in:            ClassifyResult{IntentTags: []string{" Praise ", "praise", "made_up", "Competitor-Mention"}, Sentiment: "POSITIVE"},
			wantTags:      []string{"praise", "competitor_mention"},
			wantSentiment: "positive",
			wantIntent:    "general",
		},
		{
			name:          "legacy intent only maps to a tag",
			in:            ClassifyResult{Intent: "complaint"},
			wantTags:      []string{"competitor_complaint"},
			wantSentiment: "neutral",
			wantIntent:    "complaint",
		},
		{
			name:          "legacy buy_signal used as a tag is aliased",
			in:            ClassifyResult{IntentTags: []string{"buy_signal"}},
			wantTags:      []string{"buying_intent"},
			wantSentiment: "neutral",
			wantIntent:    "buy_signal",
		},
		{
			name:          "seeking alternative is a buy signal",
			in:            ClassifyResult{IntentTags: []string{"seeking_alternative", "competitor_complaint"}},
			wantTags:      []string{"seeking_alternative", "competitor_complaint"},
			wantSentiment: "neutral",
			wantIntent:    "buy_signal",
		},
		{
			name:          "nothing known falls back to general with no tags",
			in:            ClassifyResult{Intent: "whatever", Sentiment: "meh"},
			wantTags:      []string{},
			wantSentiment: "neutral",
			wantIntent:    "general",
		},
		{
			name:          "announcement post_type strips buying tags",
			in:            ClassifyResult{IntentTags: []string{"buying_intent", "seeking_alternative", "brand_mention"}, PostType: "announcement", RelevanceScore: 9},
			wantTags:      []string{"brand_mention", "promotional"},
			wantSentiment: "neutral",
			wantIntent:    "general",
			wantMaxScore:  3,
		},
		{
			name:          "thought leadership strips buying tags",
			in:            ClassifyResult{IntentTags: []string{"seeking_alternative"}, PostType: "thought_leadership", RelevanceScore: 7},
			wantTags:      []string{"content_opportunity"},
			wantSentiment: "neutral",
			wantIntent:    "general",
			wantMaxScore:  3,
		},
		{
			name:          "Show HN title is treated as an announcement even if the model disagrees",
			title:         "Show HN: I built an open-source alternative to Datadog",
			in:            ClassifyResult{IntentTags: []string{"seeking_alternative", "competitor_mention"}, PostType: "question", RelevanceScore: 8},
			wantTags:      []string{"competitor_mention", "promotional"},
			wantSentiment: "neutral",
			wantIntent:    "general",
			wantMaxScore:  3,
		},
		{
			name:          "matches_skips caps the score and drops buying tags",
			in:            ClassifyResult{IntentTags: []string{"buying_intent", "hiring"}, MatchesSkips: true, RelevanceScore: 9, ConversionProbability: 0.9},
			wantTags:      []string{"hiring"},
			wantSentiment: "neutral",
			wantIntent:    "general",
			wantMaxScore:  2,
		},
		{
			name:          "question title is not an announcement",
			title:         "Anyone know a good alternative to Datadog?",
			in:            ClassifyResult{IntentTags: []string{"seeking_alternative"}, RelevanceScore: 8},
			wantTags:      []string{"seeking_alternative"},
			wantSentiment: "neutral",
			wantIntent:    "buy_signal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeClassifyResult(tt.in, tt.title)
			if !reflect.DeepEqual(got.IntentTags, tt.wantTags) {
				t.Errorf("tags = %#v, want %#v", got.IntentTags, tt.wantTags)
			}
			if got.Sentiment != tt.wantSentiment {
				t.Errorf("sentiment = %q, want %q", got.Sentiment, tt.wantSentiment)
			}
			if got.Intent != tt.wantIntent {
				t.Errorf("intent = %q, want %q", got.Intent, tt.wantIntent)
			}
			if tt.wantMaxScore > 0 && got.RelevanceScore > tt.wantMaxScore {
				t.Errorf("relevance = %v, want <= %v", got.RelevanceScore, tt.wantMaxScore)
			}
		})
	}
}

func TestNormalizeSkipsCapsConversion(t *testing.T) {
	got := NormalizeClassifyResult(ClassifyResult{IntentTags: []string{"buying_intent"}, MatchesSkips: true, RelevanceScore: 9, ConversionProbability: 0.9}, "")
	if got.ConversionProbability > 0.1 {
		t.Fatalf("conversion = %v, want <= 0.1", got.ConversionProbability)
	}
}

func TestLooksLikeAnnouncement(t *testing.T) {
	tests := []struct {
		title string
		want  bool
	}{
		{"Show HN: Foo, a faster CI", true},
		{"Launch HN: Bar (YC W26)", true},
		{"Introducing Baz 2.0", true},
		{"We just launched our new pricing", true},
		{"I built a tool that replaces Zapier", true},
		{"[Release] v3.1 is out", true},
		{"Announcing the Qux SDK", true},
		{"Looking for a Zapier alternative", false},
		{"What do you use for CI?", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := LooksLikeAnnouncement(tt.title); got != tt.want {
			t.Errorf("LooksLikeAnnouncement(%q) = %v, want %v", tt.title, got, tt.want)
		}
	}
}

func TestValidTagAndSentiment(t *testing.T) {
	if len(IntentTags) != 13 {
		t.Fatalf("taxonomy has %d tags, want 13", len(IntentTags))
	}
	if !IsIntentTag("hiring") || IsIntentTag("buy_signal") {
		t.Fatal("IsIntentTag wrong")
	}
	if !IsSentiment("negative") || IsSentiment("angry") {
		t.Fatal("IsSentiment wrong")
	}
}

// A buyer presenting the process they already built is a proof point: never
// a lead, whatever score the model gave, and its need is kept for Demand.
func TestNormalizeProofPointIsNotALeadButKeepsItsAsk(t *testing.T) {
	got := NormalizeClassifyResult(ClassifyResult{
		IntentTags: []string{"buying_intent", "pain_point"}, RelevanceScore: 8, AuthorRole: "buyer",
		PostType: "proof_point", Ask: "A way to see whether candidates work well with AI",
	}, "Since March we stopped giving candidates a coding test")
	if got.RelevanceScore > 3 {
		t.Errorf("proof point scored %v, want at most 3", got.RelevanceScore)
	}
	for _, tag := range got.IntentTags {
		if leadTags[tag] {
			t.Errorf("proof point kept the lead tag %q", tag)
		}
	}
	if got.Ask == "" || got.AuthorRole != "buyer" {
		t.Errorf("proof point lost its ask or role: %+v", got)
	}
	// The same post told as an unsolved problem is a lead.
	lead := NormalizeClassifyResult(ClassifyResult{IntentTags: []string{"pain_point"}, RelevanceScore: 7, AuthorRole: "buyer", PostType: "discussion"}, "t")
	if lead.RelevanceScore != 7 || lead.Intent != "complaint" {
		t.Errorf("a buyer's first-hand problem must stay a lead: %+v", lead)
	}
}

func TestClassifyPromptCoversFirstHandBuyerAccounts(t *testing.T) {
	p := buildClassifyPrompt(ScoringContext{Product: "- Acme: CRM"})
	for _, want := range []string{
		`"proof_point"`, "reports what happened in their own work", "as unsolved today",
		"Doing that work first-hand is enough", "credits no article, talk or other company",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("classifier prompt is missing %q", want)
		}
	}
}

func TestNormalizeOpenNeed(t *testing.T) {
	for _, c := range []struct {
		name       string
		in         ClassifyResult
		wantScore  float64
		wantIntent string
		wantNeed   string
	}{
		{"no open_need (older prompt, other provider): unchanged", ClassifyResult{IntentTags: []string{"pain_point"}, RelevanceScore: 7, AuthorRole: "buyer"}, 7, "complaint", ""},
		{"unknown value is not said", ClassifyResult{IntentTags: []string{"pain_point"}, RelevanceScore: 7, AuthorRole: "buyer", OpenNeed: "maybe"}, 7, "complaint", ""},
		{"nothing open: under every lead line, lead tags dropped", ClassifyResult{IntentTags: []string{"buying_intent", "competitor_complaint"}, RelevanceScore: 8, AuthorRole: "buyer", OpenNeed: " None "}, MaxNoNeedScore, "complaint", "none"},
		{"nothing open, already low: score kept", ClassifyResult{IntentTags: []string{"content_opportunity"}, RelevanceScore: 3, OpenNeed: "none"}, 3, "general", "none"},
		{"evaluating with no buying tag becomes a comparison", ClassifyResult{IntentTags: []string{"competitor_mention", "feature_request"}, RelevanceScore: 6.5, AuthorRole: "buyer", OpenNeed: "evaluating"}, 6.5, "comparison", "evaluating"},
		{"asking with no tags", ClassifyResult{RelevanceScore: 7, AuthorRole: "buyer", OpenNeed: "asking"}, 7, "recommendation_ask", "asking"},
		{"switching", ClassifyResult{IntentTags: []string{"brand_mention"}, RelevanceScore: 7, AuthorRole: "unknown", OpenNeed: "switching"}, 7, "buy_signal", "switching"},
		{"unsolved pain", ClassifyResult{IntentTags: []string{"content_opportunity"}, RelevanceScore: 6, AuthorRole: "buyer", OpenNeed: "unsolved-pain"}, 6, "complaint", "unsolved_pain"},
		{"the model's own buying tag is kept", ClassifyResult{IntentTags: []string{"buying_intent"}, RelevanceScore: 9, AuthorRole: "buyer", OpenNeed: "evaluating"}, 9, "buy_signal", "evaluating"},
		{"a non-buyer with an open need is still capped", ClassifyResult{RelevanceScore: 8, AuthorRole: "non_buyer", OpenNeed: "asking"}, maxNonBuyerScore, "general", "asking"},
		{"a skipped post stays general", ClassifyResult{RelevanceScore: 8, AuthorRole: "buyer", OpenNeed: "asking", MatchesSkips: true}, 2, "general", "asking"},
	} {
		got := NormalizeClassifyResult(c.in, "a question")
		if got.RelevanceScore != c.wantScore || got.Intent != c.wantIntent || got.OpenNeed != c.wantNeed {
			t.Errorf("%s: score %v intent %q open_need %q, want %v %q %q", c.name, got.RelevanceScore, got.Intent, got.OpenNeed, c.wantScore, c.wantIntent, c.wantNeed)
		}
	}
	// why_now is one line, and empty when nothing is open.
	if got := NormalizeClassifyResult(ClassifyResult{OpenNeed: "asking", WhyNow: " trial ends\n Friday "}, "t"); got.WhyNow != "trial ends Friday" {
		t.Errorf("why_now = %q", got.WhyNow)
	}
	if got := NormalizeClassifyResult(ClassifyResult{OpenNeed: "none", WhyNow: "n/a"}, "t"); got.WhyNow != "" {
		t.Errorf("why_now with nothing open = %q", got.WhyNow)
	}
}
