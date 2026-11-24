package handler

import (
	"cmp"
	"net/http"
	"slices"
	"time"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

const (
	payoffWindow  = 90 * 24 * time.Hour
	payoffTopRows = 50
)

type payoffTotal struct {
	Key     string `json:"key"`
	Replies int    `json:"replies"`
	Clicks  int    `json:"clicks"`
	Signups int    `json:"signups"`
}

type replyPayoff struct {
	Totals     payoffTotal               `json:"totals"`
	Replies    []database.ReplyPayoffRow `json:"replies"`
	ByPlatform []payoffTotal             `json:"by_platform"`
	ByKeyword  []payoffTotal             `json:"by_keyword"`
}

// ReplyPayoff: "Replies that paid off" — per posted reply its thread, posted
// date, clicks and sign-ups, plus totals per platform and keyword (90 days).
// GET /api/v1/analytics/reply-payoff
func (h *AnalyticsHandler) ReplyPayoff(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	rows, err := h.q.ListReplyPayoff(r.Context(), wsID, time.Now().Add(-payoffWindow))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load reply payoff")
		return
	}
	writeJSON(w, http.StatusOK, summarizePayoff(rows, payoffTopRows))
}

// summarizePayoff totals every row and keeps the top replies: most sign-ups,
// then most clicks, then the input (newest-first) order.
func summarizePayoff(rows []database.ReplyPayoffRow, top int) replyPayoff {
	out := replyPayoff{Totals: payoffTotal{Key: "all"}, Replies: []database.ReplyPayoffRow{}}
	byPlatform, byKeyword := map[string]*payoffTotal{}, map[string]*payoffTotal{}
	add := func(m map[string]*payoffTotal, key string, p database.ReplyPayoffRow) {
		t := m[key]
		if t == nil {
			t = &payoffTotal{Key: key}
			m[key] = t
		}
		t.Replies++
		t.Clicks += p.Clicks
		t.Signups += p.Signups
	}
	for _, p := range rows {
		out.Totals.Replies++
		out.Totals.Clicks += p.Clicks
		out.Totals.Signups += p.Signups
		add(byPlatform, p.Platform, p)
		kw := p.Keyword
		if kw == "" {
			kw = "(no keyword)"
		}
		add(byKeyword, kw, p)
	}

	ranked := slices.Clone(rows)
	slices.SortStableFunc(ranked, func(a, b database.ReplyPayoffRow) int {
		return cmp.Or(cmp.Compare(b.Signups, a.Signups), cmp.Compare(b.Clicks, a.Clicks))
	})
	if len(ranked) > top {
		ranked = ranked[:top]
	}
	out.Replies = append(out.Replies, ranked...)
	out.ByPlatform = sortedTotals(byPlatform)
	out.ByKeyword = sortedTotals(byKeyword)
	return out
}

func sortedTotals(m map[string]*payoffTotal) []payoffTotal {
	out := make([]payoffTotal, 0, len(m))
	for _, t := range m {
		out = append(out, *t)
	}
	slices.SortFunc(out, func(a, b payoffTotal) int {
		return cmp.Or(cmp.Compare(b.Signups, a.Signups), cmp.Compare(b.Clicks, a.Clicks),
			cmp.Compare(b.Replies, a.Replies), cmp.Compare(a.Key, b.Key))
	})
	return out
}
