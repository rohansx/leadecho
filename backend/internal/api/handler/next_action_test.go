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
	for _, c := range []struct {
		m     database.Mention
		draft bool
		want  string
	}{
		{mention(6.5, database.IntentTypeBuySignal), true, "ready_to_send"},
		{mention(6.5, database.IntentTypeBuySignal), false, "needs_draft"},
		{mention(6.0, database.IntentTypeComparison), false, "needs_draft"},
		{mention(8.0, database.IntentTypeComparison), true, "ready_to_send"},
		{mention(5.9, database.IntentTypeBuySignal), true, ""},
		{mention(9.0, database.IntentTypeGeneral), true, ""},
		{database.Mention{Status: database.MentionStatusNew}, true, ""},
	} {
		if got := nextActionFor(c.m, c.draft); got != c.want {
			t.Errorf("score %.1f intent %s draft=%v: %q, want %q", c.m.RelevanceScore.Float32, c.m.Intent.IntentType, c.draft, got, c.want)
		}
	}
}
