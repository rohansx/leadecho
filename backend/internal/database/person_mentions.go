package database

import (
	"context"
	"time"
)

// PersonMention is one post by a person that became a lead in the workspace.
type PersonMention struct {
	MentionID string    `json:"mention_id"`
	LeadID    string    `json:"lead_id"`
	Platform  string    `json:"platform"`
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Excerpt   string    `json:"excerpt"`
	Intent    string    `json:"intent,omitempty"`
	Score     float32   `json:"score"`
	PostedAt  time.Time `json:"posted_at"`
}

// ListPersonMentions returns the newest posts of a person that are leads in
// the workspace (one person can author several).
func (q *Queries) ListPersonMentions(ctx context.Context, workspaceID, personID string, limit int32) ([]PersonMention, error) {
	rows, err := q.db.Query(ctx, `
		SELECT m.id::text, l.id::text, m.platform::text, m.url, coalesce(m.title, ''),
		       left(m.content, 600), coalesce(m.intent::text, ''), coalesce(m.relevance_score, 0),
		       coalesce(m.platform_created_at, m.created_at)
		FROM leads l
		JOIN mentions m ON m.id = l.mention_id
		WHERE l.workspace_id = $1 AND l.person_id = $2
		ORDER BY coalesce(m.platform_created_at, m.created_at) DESC
		LIMIT $3`, workspaceID, personID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PersonMention{}
	for rows.Next() {
		var p PersonMention
		if err := rows.Scan(&p.MentionID, &p.LeadID, &p.Platform, &p.URL, &p.Title, &p.Excerpt, &p.Intent, &p.Score, &p.PostedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
