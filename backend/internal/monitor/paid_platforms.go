package monitor

import (
	"context"

	"leadecho/internal/database"
)

// withoutPlatforms returns copies of kws with the locked platforms removed,
// dropping keywords left with none. Free workspaces would otherwise have
// Reddit fall back to the public crawler, which datacenter IPs rarely reach.
func withoutPlatforms(kws []database.ListActiveKeywordsRow, locked map[string]bool) []database.ListActiveKeywordsRow {
	out := make([]database.ListActiveKeywordsRow, 0, len(kws))
	for _, kw := range kws {
		free := make([]string, 0, len(kw.Platforms))
		for _, p := range kw.Platforms {
			if !locked[p] {
				free = append(free, p)
			}
		}
		if len(free) == 0 {
			continue
		}
		kw.Platforms = free // kw is a copy; the caller's slice is untouched
		out = append(out, kw)
	}
	return out
}

// linkedInLocked: the workspace's plan has no LinkedIn data, so nothing built
// on it runs (hiring signals, prospecting). Solo has Reddit but not LinkedIn.
func (m *Monitor) linkedInLocked(ctx context.Context, wsID string) bool {
	return m.credits.LockedPlatforms(ctx, wsID)["linkedin"]
}
