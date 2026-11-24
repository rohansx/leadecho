package database

import "context"

// LeadRef names one lead in one workspace.
type LeadRef struct {
	ID, WorkspaceID string
}

// ListLeadsWithoutIdentity returns the newest leads whose author's identity
// was never resolved, on the platforms that have a public profile to start
// from (LinkedIn authors go to the paid lookup as they are). Only Leads Ready
// (the workspace's lead line) and open leads: the backfill spends the same as
// live leads.
// workspaceID "" means every workspace.
func (q *Queries) ListLeadsWithoutIdentity(ctx context.Context, workspaceID string, limit int32) ([]LeadRef, error) {
	rows, err := q.db.Query(ctx, `
		SELECT l.id::text, l.workspace_id::text
		FROM leads l
		JOIN mentions m ON m.id = l.mention_id
		WHERE NOT (l.metadata ? 'identity')
		  AND coalesce((l.metadata->>'identity_attempt_at')::timestamptz, 'epoch') < now() - interval '7 days'
		  AND l.stage::text NOT IN ('lost', 'converted')
		  AND m.platform::text IN ('hackernews', 'devto', 'bluesky')
		  AND coalesce(m.author_username, '') <> ''
		  AND `+leadScoreSQL("m.", "m.workspace_id")+` -- Leads Ready, as the live path enriches
		  AND ($2 = '' OR l.workspace_id::text = $2)
		ORDER BY l.created_at DESC
		LIMIT $1`, limit, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeadRef
	for rows.Next() {
		var r LeadRef
		if err := rows.Scan(&r.ID, &r.WorkspaceID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FillLeadContact sets contact name, email and company only where the lead
// has none, in one statement: a value typed meanwhile is never overwritten,
// and the stage is not touched. Empty arguments are ignored.
func (q *Queries) FillLeadContact(ctx context.Context, workspaceID, leadID, name, email, company string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE leads SET
		  contact_name  = CASE WHEN coalesce(contact_name, '') = ''  THEN coalesce(nullif($3, ''), contact_name)  ELSE contact_name END,
		  contact_email = CASE WHEN coalesce(contact_email, '') = '' THEN coalesce(nullif($4, ''), contact_email) ELSE contact_email END,
		  company       = CASE WHEN coalesce(company, '') = ''       THEN coalesce(nullif($5, ''), company)       ELSE company END
		WHERE id = $1 AND workspace_id = $2`,
		leadID, workspaceID, name, email, company)
	return err
}

// ListLeadsForPerson360 returns the newest open Leads Ready leads whose
// author has no Person360 profile yet, is identified but was never mapped in
// depth, or whose identity found a LinkedIn or email that the paid lookup has
// not used. Skips leads tried in the last day.
func (q *Queries) ListLeadsForPerson360(ctx context.Context, workspaceID string, limit int32) ([]LeadRef, error) {
	rows, err := q.db.Query(ctx, `
		SELECT l.id::text, l.workspace_id::text
		FROM leads l
		JOIN mentions m ON m.id = l.mention_id
		LEFT JOIN persons p ON p.id = l.person_id
		WHERE l.workspace_id::text = $2
		  AND l.stage::text NOT IN ('lost', 'converted')
		  AND `+leadScoreSQL("m.", "m.workspace_id")+`
		  AND coalesce((l.metadata->>'person360_attempt_at')::timestamptz, 'epoch') < now() - interval '1 day'
		  AND (
		        p.id IS NULL OR NOT (p.metadata ? 'dossier')
		     -- identified (a full name and a verified link) but never mapped in depth
		     OR (l.metadata->'identity'->>'deep_at' IS NULL
		         AND l.metadata->'identity'->>'name' LIKE '% %'
		         AND EXISTS (SELECT 1 FROM jsonb_array_elements(l.metadata->'identity'->'links') y
		                     WHERE (y->>'confidence')::float >= 0.8 AND y->>'method' <> 'origin'))
		     OR (NOT (l.metadata ? 'enrichment') AND EXISTS (
		           SELECT 1 FROM jsonb_array_elements(l.metadata->'identity'->'links') x
		           WHERE (x->>'confidence')::float >= 0.8 AND x->>'kind' IN ('linkedin', 'email')))
		  )
		ORDER BY l.created_at DESC
		LIMIT $1`, limit, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeadRef
	for rows.Next() {
		var r LeadRef
		if err := rows.Scan(&r.ID, &r.WorkspaceID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
