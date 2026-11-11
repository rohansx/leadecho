package monitor

import (
	"leadecho/internal/billing"
	"leadecho/internal/database"
)

// withoutPaidPlatforms returns copies of kws with the paid platforms removed,
// dropping keywords left with none. Free workspaces would otherwise have
// Reddit fall back to the public crawler, which datacenter IPs rarely reach.
func withoutPaidPlatforms(kws []database.ListActiveKeywordsRow) []database.ListActiveKeywordsRow {
	out := make([]database.ListActiveKeywordsRow, 0, len(kws))
	for _, kw := range kws {
		free := make([]string, 0, len(kw.Platforms))
		for _, p := range kw.Platforms {
			if !billing.PaidPlatforms[p] {
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
