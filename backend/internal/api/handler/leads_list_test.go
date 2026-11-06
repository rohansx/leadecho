package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

func TestLeadRowToListItem(t *testing.T) {
	base := database.Lead{ID: "l1", Stage: database.LeadStageProspect, Tags: []string{}, Metadata: []byte(`{}`)}

	t.Run("no mention keeps the legacy shape with mention null", func(t *testing.T) {
		item := leadRowToListItem(database.ListLeadsWithMentionRow{Lead: base})
		if item.Mention != nil {
			t.Fatalf("want nil mention, got %+v", item.Mention)
		}
		b, _ := json.Marshal(item)
		if !strings.Contains(string(b), `"id":"l1"`) || !strings.Contains(string(b), `"mention":null`) {
			t.Fatalf("unexpected json: %s", b)
		}
	})

	t.Run("mention fields mapped and snippet truncated", func(t *testing.T) {
		long := strings.Repeat("é", mentionSnippetRunes+10)
		item := leadRowToListItem(database.ListLeadsWithMentionRow{
			Lead:           base,
			MentionTitle:   pgtype.Text{String: "Need a CRM", Valid: true},
			MentionContent: pgtype.Text{String: long, Valid: true},
			MentionUrl:     pgtype.Text{String: "https://reddit.com/x", Valid: true},
			MentionIntent:  database.NullIntentType{IntentType: database.IntentTypeRecommendationAsk, Valid: true},
			MentionScore:   pgtype.Float4{Float32: 8.5, Valid: true},
		})
		m := item.Mention
		if m == nil || m.URL != "https://reddit.com/x" || *m.Title != "Need a CRM" || *m.Intent != "recommendation_ask" || *m.Score != 8.5 {
			t.Fatalf("bad mapping: %+v", m)
		}
		if got := len([]rune(m.Snippet)); got != mentionSnippetRunes+1 {
			t.Fatalf("snippet runes = %d, want %d", got, mentionSnippetRunes+1)
		}
		if m.PostedAt != nil {
			t.Fatalf("want nil posted_at")
		}
	})
}
