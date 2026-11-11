package database

import "context"

// MentionPerson is what enrichment learned about a mention's author.
type MentionPerson struct {
	Name      string `json:"name,omitempty"`
	Title     string `json:"title,omitempty"`
	Bio       string `json:"bio,omitempty"`
	Company   string `json:"company,omitempty"`
	Employees string `json:"employees,omitempty"`
	HasEmail  bool   `json:"has_email,omitempty"`
}

// MentionPeople returns the enriched author for each of the given mentions
// that has one, keyed by mention id. Hand-written (no sqlc codegen needed).
func (q *Queries) MentionPeople(ctx context.Context, workspaceID string, mentionIDs []string) (map[string]MentionPerson, error) {
	out := map[string]MentionPerson{}
	if len(mentionIDs) == 0 {
		return out, nil
	}
	rows, err := q.db.Query(ctx, `
SELECT l.mention_id::text,
       COALESCE(NULLIF(l.metadata->'enrichment'->>'full_name', ''), l.contact_name, ''),
       COALESCE(l.metadata->'enrichment'->>'title', ''),
       COALESCE(l.metadata->'enrichment'->>'bio', ''),
       COALESCE(NULLIF(l.metadata->'enrichment'->>'company', ''), l.company, ''),
       COALESCE(l.metadata->'enrichment'->>'employees', ''),
       COALESCE(l.contact_email, '') <> ''
FROM leads l
WHERE l.workspace_id = $1 AND l.mention_id = ANY($2::uuid[]) AND l.metadata ? 'enrichment'`,
		workspaceID, mentionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var p MentionPerson
		if err := rows.Scan(&id, &p.Name, &p.Title, &p.Bio, &p.Company, &p.Employees, &p.HasEmail); err != nil {
			return nil, err
		}
		if p.Name != "" || p.Title != "" || p.Bio != "" || p.Company != "" {
			out[id] = p
		}
	}
	return out, rows.Err()
}
