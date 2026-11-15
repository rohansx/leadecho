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
	// A keyword that fills the inbox on a platform without ever finding a
	// lead there is searched once a day, whatever its cadence says.
	for id, platforms := range throttledPlatforms(m.keywordYield(ctx, wsID)) {
		cad[id] = cad[id].Throttle(platforms)
	}
	return keywordsDue(kws, cad, last, time.Now())
}

// keywordYield is what each keyword brought in per platform over the yield
// window. A failed lookup returns nothing: no keyword is throttled on a guess.
func (m *Monitor) keywordYield(ctx context.Context, wsID string) []database.KeywordYield {
	rows, err := m.q.ListKeywordYield(ctx, wsID, time.Now().Add(-cadence.YieldWindow))
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("yield: load failed, nothing throttled")
		return nil
	}
	return rows
}

// throttledPlatforms lists, per keyword id, the platforms where it brought
// many posts and no lead. The first lead there takes the platform off the
// list again on the next scan.
func throttledPlatforms(rows []database.KeywordYield) map[string][]string {
	out := map[string][]string{}
	for _, r := range rows {
		if (cadence.Yield{Judged: r.Judged, Leads: r.Leads}).Throttled() {
			out[r.KeywordID] = append(out[r.KeywordID], r.Platform)
		}
	}
	return out
}

// bulkFilterPairs is the same list keyed the way a stored post knows its
// keyword: by term and platform. Posts from these pairs go through the
// cheapest noise filter.
func bulkFilterPairs(rows []database.KeywordYield) map[[2]string]bool {
	out := map[[2]string]bool{}
	for _, r := range rows {
		if (cadence.Yield{Judged: r.Judged, Leads: r.Leads}).Throttled() {
			out[[2]string{r.Term, r.Platform}] = true
		}
	}
	return out
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
