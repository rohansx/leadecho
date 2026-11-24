package handler

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// Every lead the queue lists has a badge: the badge uses the queue's rule
// (score 6 and up; a comparison counts), not a stricter one of its own.
func TestNextActionMatchesTheQueueRule(t *testing.T) {
	mention := func(score float32, intent database.IntentType) database.Mention {
		return database.Mention{Status: database.MentionStatusNew,
			RelevanceScore: pgtype.Float4{Float32: score, Valid: true},
			Intent:         database.NullIntentType{IntentType: intent, Valid: true}}
	}
	balanced, strict, loose := database.LeadMinScore, database.LeadStrict.MinScore(), database.LeadLoose.MinScore()
	for _, c := range []struct {
		m     database.Mention
		draft bool
		min   float64
		want  string
	}{
		{mention(6.5, database.IntentTypeBuySignal), true, balanced, "ready_to_send"},
		{mention(6.5, database.IntentTypeBuySignal), false, balanced, "needs_draft"},
		{mention(6.0, database.IntentTypeComparison), false, balanced, "needs_draft"},
		{mention(8.0, database.IntentTypeComparison), true, balanced, "ready_to_send"},
		{mention(5.9, database.IntentTypeBuySignal), true, balanced, ""},
		{mention(9.0, database.IntentTypeGeneral), true, balanced, ""},
		{database.Mention{Status: database.MentionStatusNew}, true, balanced, ""},
		// the workspace's strictness moves the line
		{mention(6.5, database.IntentTypeBuySignal), false, strict, ""},
		{mention(5.5, database.IntentTypeBuySignal), false, loose, "needs_draft"},
	} {
		if got := nextActionFor(c.m, c.draft, c.min); got != c.want {
			t.Errorf("score %.1f intent %s draft=%v min=%v: %q, want %q", c.m.RelevanceScore.Float32, c.m.Intent.IntentType, c.draft, c.min, got, c.want)
		}
	}
}
