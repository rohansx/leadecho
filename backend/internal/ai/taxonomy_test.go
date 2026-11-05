package ai

import (
	"reflect"
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
