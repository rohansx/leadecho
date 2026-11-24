package database

import (
	"context"
	"time"
)

// Threads that rank in web search: open Leads Ready leads on Reddit, Hacker
// News and Dev.to are looked up weekly in web search for the keywords that
// found them (see monitor/search_rank.go). The result lives in the mention's
// platform_metadata.search_rank.

// searchRankThreadsSQL: the open Leads Ready threads of workspace w (alias).
const searchRankThreadsSQL = `
	FROM leads l
	JOIN mentions m ON m.id = l.mention_id
	JOIN keywords k ON k.id = m.keyword_id AND k.is_active
	WHERE l.stage::text NOT IN ('lost', 'converted')
	  AND m.platform::text IN ('reddit', 'hackernews', 'devto')
	  AND m.status::text NOT IN ('archived', 'spam')`

// SearchRankThread is one open lead thread and the keyword that found it.
type SearchRankThread struct {
	MentionID, URL, Keyword string
	// RankQuery is the query of the stored search_rank ("" when none).
	RankQuery string
}

// ListSearchRankThreads returns the workspace's newest open Leads Ready
// threads with the active keyword that found each.
func (q *Queries) ListSearchRankThreads(ctx context.Context, workspaceID string, limit int32) ([]SearchRankThread, error) {
	rows, err := q.db.Query(ctx, `
		SELECT m.id::text, m.url, k.term, coalesce(m.platform_metadata->'search_rank'->>'query', '')
		`+searchRankThreadsSQL+`
		  AND l.workspace_id::text = $1
		  AND `+leadScoreSQL("m.", "m.workspace_id")+`
		ORDER BY l.created_at DESC
		LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchRankThread
	for rows.Next() {
		var t SearchRankThread
		if err := rows.Scan(&t.MentionID, &t.URL, &t.Keyword, &t.RankQuery); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListWorkspacesDueSearchRank returns workspaces with open lead threads whose
// last search-rank check is older than before (or never ran). Only a
// candidate list: claim each with ClaimSettingsRun before searching.
func (q *Queries) ListWorkspacesDueSearchRank(ctx context.Context, before time.Time, limit int) ([]string, error) {
	rows, err := q.db.Query(ctx, `
		SELECT w.id::text FROM workspaces w
		WHERE NOT w.scanning_paused
		  AND coalesce((w.settings->'`+SearchRankKey+`'->>'last_run')::timestamptz, 'epoch') < $1
		  AND EXISTS (SELECT 1 `+searchRankThreadsSQL+` AND l.workspace_id = w.id
		              AND `+leadScoreSQL("m.", "m.workspace_id")+`)
		ORDER BY w.id
		LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SearchRankKey is the workspaces.settings key whose last_run is claimed
// with ClaimSettingsRun before a check, so one instance searches a
// workspace per interval.
const SearchRankKey = "search_rank"

// SetMentionSearchRank stores raw as platform_metadata.search_rank, or
// removes the key when raw is nil.
func (q *Queries) SetMentionSearchRank(ctx context.Context, workspaceID, mentionID string, raw []byte) error {
	if raw == nil {
		_, err := q.db.Exec(ctx, `
			UPDATE mentions SET platform_metadata = platform_metadata - 'search_rank'
			WHERE id = $1 AND workspace_id = $2`, mentionID, workspaceID)
		return err
	}
	_, err := q.db.Exec(ctx, `
		UPDATE mentions SET platform_metadata = coalesce(platform_metadata, '{}'::jsonb) || jsonb_build_object('search_rank', $3::jsonb)
		WHERE id = $1 AND workspace_id = $2`, mentionID, workspaceID, raw)
	return err
}
