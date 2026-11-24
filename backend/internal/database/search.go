package database

import (
	"context"
	"strings"
	"time"
)

// SearchPerson is a person matching a search, with the post to open them from.
type SearchPerson struct {
	PersonID  string `json:"person_id"`
	Name      string `json:"name"`
	Headline  string `json:"headline"`
	Company   string `json:"company"`
	Handles   string `json:"handles"`    // their usernames/addresses, comma separated
	MentionID string `json:"mention_id"` // their newest post that is a lead ("" if none)
}

// SearchPost is a post (mention) matching a search.
type SearchPost struct {
	MentionID string    `json:"mention_id"`
	Platform  string    `json:"platform"`
	Title     string    `json:"title"`
	Excerpt   string    `json:"excerpt"`
	Author    string    `json:"author"`
	Score     float32   `json:"score"`
	PostedAt  time.Time `json:"posted_at"`
}

// likePattern escapes LIKE wildcards so the query is matched literally.
func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(strings.TrimSpace(q)) + "%"
}

// SearchPeople finds people in the workspace by name, company, bio, any
// username/email/profile URL, or their Person360 headline and summary.
func (q *Queries) SearchPeople(ctx context.Context, workspaceID, query string, limit int32) ([]SearchPerson, error) {
	rows, err := q.db.Query(ctx, `
		WITH hit AS (
		  SELECT p.id
		  FROM persons p
		  WHERE p.workspace_id = $1 AND (
		        p.display_name ILIKE $2 OR p.company ILIKE $2 OR p.bio ILIKE $2 OR p.location ILIKE $2
		     OR p.metadata->'dossier'->>'headline' ILIKE $2 OR p.metadata->'dossier'->>'about' ILIKE $2
		     OR p.metadata->'identity'->>'name' ILIKE $2
		     OR EXISTS (SELECT 1 FROM person_identities pi
		                WHERE pi.person_id = p.id AND (pi.handle ILIKE $2 OR pi.profile_url ILIKE $2)))
		  ORDER BY p.updated_at DESC
		  LIMIT $3
		)
		SELECT p.id::text,
		       coalesce(nullif(p.metadata->'identity'->>'name', ''), p.display_name, ''),
		       coalesce(p.metadata->'dossier'->>'headline', left(p.bio, 160), ''),
		       coalesce(p.company, ''),
		       coalesce((SELECT string_agg(DISTINCT pi.handle, ', ') FROM person_identities pi
		                 WHERE pi.person_id = p.id AND pi.platform NOT IN ('website', 'booking')), ''),
		       coalesce((SELECT l.mention_id::text FROM leads l
		                 WHERE l.person_id = p.id AND l.workspace_id = $1 AND l.mention_id IS NOT NULL
		                 ORDER BY l.created_at DESC LIMIT 1), '')
		FROM persons p JOIN hit ON hit.id = p.id
		ORDER BY p.updated_at DESC`, workspaceID, likePattern(query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SearchPerson{}
	for rows.Next() {
		var s SearchPerson
		if err := rows.Scan(&s.PersonID, &s.Name, &s.Headline, &s.Company, &s.Handles, &s.MentionID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SearchPosts finds posts in the workspace by their text (full text, then a
// literal match on title or author), best scored first.
func (q *Queries) SearchPosts(ctx context.Context, workspaceID, query string, limit int32) ([]SearchPost, error) {
	rows, err := q.db.Query(ctx, `
		SELECT m.id::text, m.platform::text, coalesce(m.title, ''), left(m.content, 220),
		       coalesce(m.author_username, ''), coalesce(m.relevance_score, 0),
		       coalesce(m.platform_created_at, m.created_at)
		FROM mentions m
		WHERE m.workspace_id = $1
		  AND (m.content_tsv @@ plainto_tsquery('english', $2)
		       OR m.title ILIKE $3 OR m.author_username ILIKE $3)
		ORDER BY (m.title ILIKE $3 OR m.author_username ILIKE $3) DESC,
		         coalesce(m.relevance_score, 0) DESC, m.created_at DESC
		LIMIT $4`, workspaceID, strings.TrimSpace(query), likePattern(query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SearchPost{}
	for rows.Next() {
		var s SearchPost
		if err := rows.Scan(&s.MentionID, &s.Platform, &s.Title, &s.Excerpt, &s.Author, &s.Score, &s.PostedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
