package ai

import "strings"

// IntentTags is the 13-tag intent taxonomy. A mention can carry several.
var IntentTags = []string{
	"buying_intent", "recommendation_request", "comparison", "seeking_alternative",
	"competitor_complaint", "competitor_mention", "brand_mention", "praise",
	"pain_point", "feature_request", "content_opportunity", "hiring", "promotional",
}

// Sentiments are the allowed mention sentiment values.
var Sentiments = []string{"positive", "neutral", "negative"}

var (
	intentTagSet = toSet(IntentTags)
	sentimentSet = toSet(Sentiments)

	// legacyIntentTag maps the old single-intent values onto the taxonomy
	// (models still emit them; old rows were backfilled the same way).
	legacyIntentTag = map[string]string{
		"buy_signal":         "buying_intent",
		"recommendation_ask": "recommendation_request",
		"complaint":          "competitor_complaint",
		"comparison":         "comparison",
	}

	// tagIntentPriority derives the legacy intent column (which lead
	// qualification still reads) from the tags: first match wins.
	tagIntentPriority = []struct{ tag, intent string }{
		{"buying_intent", "buy_signal"},
		{"seeking_alternative", "buy_signal"},
		{"recommendation_request", "recommendation_ask"},
		{"comparison", "comparison"},
		{"competitor_complaint", "complaint"},
		{"pain_point", "complaint"},
	}

	// leadTags say "this author is shopping". Announcements and skipped
	// posts must never carry them.
	leadTags = toSet([]string{"buying_intent", "seeking_alternative", "recommendation_request"})

	announcementPrefixes = []string{
		"show hn", "launch hn", "introducing", "announcing", "we launched",
		"we just launched", "just launched", "we're launching", "we are launching",
		"i built", "i made", "i created", "we built", "[release]", "[launch]",
		"[announcement]", "release:", "launching",
	}
)

// IsIntentTag reports whether t is one of the 13 taxonomy tags.
func IsIntentTag(t string) bool { return intentTagSet[t] }

// IsSentiment reports whether s is an allowed sentiment value.
func IsSentiment(s string) bool { return sentimentSet[s] }

// LooksLikeAnnouncement is a deterministic guard for launch/announcement
// posts, which the model sometimes tags as seeking_alternative because they
// mention competitors ("an open-source alternative to X").
//
// ponytail: title-prefix heuristic only. Add body checks if launches with
// neutral titles keep slipping through.
func LooksLikeAnnouncement(title string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	for _, p := range announcementPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// maxNonBuyerScore keeps non-buyers under the lead threshold (6).
const maxNonBuyerScore = 4

// NormalizeClassifyResult cleans a raw model result: canonical tags only,
// valid sentiment, announcement and skips rules enforced, and the legacy
// Intent field derived from the tags so existing consumers keep working.
func NormalizeClassifyResult(cr ClassifyResult, title string) ClassifyResult {
	tags := normalizeTags(cr.IntentTags)
	if len(tags) == 0 {
		if t, ok := legacyIntentTag[strings.ToLower(strings.TrimSpace(cr.Intent))]; ok {
			tags = []string{t}
		}
	}

	pt := strings.ToLower(strings.TrimSpace(cr.PostType))
	announcement := pt == "announcement" || LooksLikeAnnouncement(title)
	thought := pt == "thought_leadership"
	cr.AuthorRole = strings.ToLower(strings.TrimSpace(cr.AuthorRole))
	if cr.AuthorRole != "buyer" && cr.AuthorRole != "non_buyer" {
		cr.AuthorRole = "unknown"
	}
	nonBuyer := cr.AuthorRole == "non_buyer"
	if announcement || thought || cr.MatchesSkips || nonBuyer {
		tags = dropLeadTags(tags)
	}
	if nonBuyer { // a candidate or bystander on the right topic is a conversation, not a lead
		cr.RelevanceScore = min(cr.RelevanceScore, maxNonBuyerScore)
		cr.ConversionProbability = min(cr.ConversionProbability, 0.15)
	}
	if announcement {
		tags = appendTag(tags, "promotional")
	} else if thought {
		tags = appendTag(tags, "content_opportunity")
	}
	if announcement || thought {
		cr.RelevanceScore = min(cr.RelevanceScore, 3)
	}
	if cr.MatchesSkips {
		cr.RelevanceScore = min(cr.RelevanceScore, 2)
		cr.ConversionProbability = min(cr.ConversionProbability, 0.1)
	}

	cr.IntentTags = tags
	cr.Intent = intentFromTags(tags)
	if cr.MatchesSkips {
		cr.Intent = "general"
	}
	cr.Sentiment = strings.ToLower(strings.TrimSpace(cr.Sentiment))
	if !IsSentiment(cr.Sentiment) {
		cr.Sentiment = "neutral"
	}
	return cr
}

// LegacyIntentTags maps an old intent value to its taxonomy tags.
func LegacyIntentTags(intent string) []string {
	if t, ok := legacyIntentTag[intent]; ok {
		return []string{t}
	}
	return []string{}
}

func normalizeTags(raw []string) []string {
	out := []string{}
	for _, t := range raw {
		t = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(t)), "-", "_")
		if alias, ok := legacyIntentTag[t]; ok {
			t = alias
		}
		if IsIntentTag(t) {
			out = appendTag(out, t)
		}
	}
	return out
}

func dropLeadTags(tags []string) []string {
	out := []string{}
	for _, t := range tags {
		if !leadTags[t] {
			out = append(out, t)
		}
	}
	return out
}

func appendTag(tags []string, t string) []string {
	for _, x := range tags {
		if x == t {
			return tags
		}
	}
	return append(tags, t)
}

func intentFromTags(tags []string) string {
	set := toSet(tags)
	for _, p := range tagIntentPriority {
		if set[p.tag] {
			return p.intent
		}
	}
	return "general"
}

func toSet(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
