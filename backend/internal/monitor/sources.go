package monitor

// sources feature: per-platform scan cadence and mute lists on the insert path.

import (
	"context"
	"encoding/json"
	"time"

	"leadecho/internal/cadence"
	"leadecho/internal/database"
	"leadecho/internal/mutes"
)

// Mutes exposes the monitor's mute store so the API shares its cache.
func (m *Monitor) Mutes() *mutes.Store { return m.mutes }

// keywordsDue narrows each keyword's platforms to those due per its cadence
// and drops keywords with nothing due. Inputs are not mutated.
func keywordsDue(kws []database.ListActiveKeywordsRow, cad map[string]cadence.Map, last map[string]map[string]time.Time, now time.Time) []database.ListActiveKeywordsRow {
	out := make([]database.ListActiveKeywordsRow, 0, len(kws))
	for _, kw := range kws {
		due := cadence.Due(kw.Platforms, cad[kw.ID], last[kw.ID], now)
		if len(due) == 0 {
			continue
		}
		kw.Platforms = due // kw is a copy; due is a fresh slice
		out = append(out, kw)
	}
	return out
}

// applyCadence filters kws to what is due now. On a lookup error it scans
// everything (the pre-cadence behaviour) rather than skipping the workspace.
func (m *Monitor) applyCadence(ctx context.Context, wsID string, kws []database.ListActiveKeywordsRow) []database.ListActiveKeywordsRow {
	rows, err := m.q.ListKeywordCadences(ctx, wsID)
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("cadence: load failed, scanning all platforms")
		return kws
	}
	scans, err := m.q.ListKeywordPlatformScans(ctx, wsID)
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("cadence: load scans failed, scanning all platforms")
		return kws
	}
	cad := make(map[string]cadence.Map, len(rows))
	for _, r := range rows {
		if c, err := cadence.Parse(r.Cadence); err == nil {
			cad[r.ID] = c
		}
	}
	last := map[string]map[string]time.Time{}
	for _, s := range scans {
		if last[s.KeywordID] == nil {
			last[s.KeywordID] = map[string]time.Time{}
		}
		last[s.KeywordID][s.Platform] = s.LastScannedAt
	}
	return keywordsDue(kws, cad, last, time.Now())
}

// recordScans stamps last_scanned for each platform just crawled.
func (m *Monitor) recordScans(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) {
	for _, p := range kw.Platforms {
		if err := m.q.UpsertKeywordPlatformScan(ctx, database.UpsertKeywordPlatformScanParams{Platform: p, KeywordID: kw.ID, WorkspaceID: wsID}); err != nil {
			m.logger.Warn().Err(err).Str("keyword_id", kw.ID).Str("platform", p).Msg("cadence: record scan failed")
		}
	}
}

// muteCandidate extracts the fields mutes match on from a pending mention.
func muteCandidate(p database.CreateMentionParams) mutes.Candidate {
	var meta struct {
		Subreddit string `json:"subreddit"`
	}
	_ = json.Unmarshal(p.PlatformMetadata, &meta) // best effort: no subreddit on bad JSON
	return mutes.Candidate{
		Platform:  string(p.Platform),
		Author:    p.AuthorUsername.String,
		URL:       p.Url,
		Title:     p.Title.String,
		Content:   p.Content,
		Subreddit: meta.Subreddit,
	}
}

// isMuted reports whether the mention should be dropped. Fails open: a mute
// lookup error must never stop ingestion.
func (m *Monitor) isMuted(ctx context.Context, p database.CreateMentionParams) bool {
	if m.mutes == nil {
		return false
	}
	muted, err := m.mutes.IsMuted(ctx, p.WorkspaceID, muteCandidate(p))
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", p.WorkspaceID).Msg("mutes: lookup failed, not muting")
		return false
	}
	return muted
}

// IsMutedMention lets other ingest paths (extension signals) apply mutes.
func (m *Monitor) IsMutedMention(ctx context.Context, p database.CreateMentionParams) bool {
	return m.isMuted(ctx, p)
}
