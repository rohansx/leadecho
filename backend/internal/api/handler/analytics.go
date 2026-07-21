package handler

import (
	"context"
	"fmt"
	"net/http"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

type AnalyticsHandler struct {
	q *database.Queries
}

func NewAnalyticsHandler(q *database.Queries) *AnalyticsHandler {
	return &AnalyticsHandler{q: q}
}

func (h *AnalyticsHandler) Overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)

	// Surface query errors instead of silently reporting zeros (which would
	// misreport every KPI as 0 during a DB problem).
	counters := []struct {
		key string
		fn  func(context.Context, string) (int32, error)
	}{
		{"mentions_30d", h.q.CountMentions30d},
		{"mentions_new", h.q.CountNewMentions},
		{"total_leads", h.q.CountTotalLeads},
		{"converted_leads", h.q.CountConvertedLeads},
		{"replies_posted", h.q.CountRepliesPosted30d},
		{"active_keywords", h.q.CountActiveKeywords},
	}
	stats := make(map[string]int32, len(counters))
	for _, c := range counters {
		v, err := c.fn(ctx, wsID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load overview stats")
			return
		}
		stats[c.key] = v
	}

	writeJSON(w, http.StatusOK, stats)
}

func (h *AnalyticsHandler) MentionsPerDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)

	rows, err := h.q.MentionsPerDay(ctx, wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get mentions per day")
		return
	}

	type dayCount struct {
		Day   string `json:"day"`
		Count int32  `json:"count"`
	}
	resp := make([]dayCount, len(rows))
	for i, row := range rows {
		day := ""
		if row.Day.Valid {
			day = fmt.Sprintf("%04d-%02d-%02d", row.Day.Time.Year(), row.Day.Time.Month(), row.Day.Time.Day())
		}
		resp[i] = dayCount{Day: day, Count: row.Count}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AnalyticsHandler) MentionsPerPlatform(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)

	rows, err := h.q.MentionsPerPlatform(ctx, wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get platform stats")
		return
	}

	type item struct {
		Platform string `json:"platform"`
		Count    int32  `json:"count"`
	}
	resp := make([]item, len(rows))
	for i, row := range rows {
		resp[i] = item{Platform: string(row.Platform), Count: row.Count}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AnalyticsHandler) MentionsPerIntent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)

	rows, err := h.q.MentionsPerIntent(ctx, wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get intent stats")
		return
	}

	type item struct {
		Intent string `json:"intent"`
		Count  int32  `json:"count"`
	}
	resp := make([]item, len(rows))
	for i, row := range rows {
		resp[i] = item{Intent: string(row.Intent.IntentType), Count: row.Count}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AnalyticsHandler) ConversionFunnel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)

	rows, err := h.q.ConversionFunnel(ctx, wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get funnel")
		return
	}

	type item struct {
		Stage string `json:"stage"`
		Count int32  `json:"count"`
	}
	resp := make([]item, len(rows))
	for i, row := range rows {
		resp[i] = item{Stage: string(row.Stage), Count: row.Count}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AnalyticsHandler) TopKeywords(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)

	rows, err := h.q.TopKeywords(ctx, wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get top keywords")
		return
	}

	type item struct {
		Term         string `json:"term"`
		MentionCount int32  `json:"mention_count"`
	}
	resp := make([]item, len(rows))
	for i, row := range rows {
		resp[i] = item{Term: row.Term, MentionCount: row.MentionCount}
	}
	writeJSON(w, http.StatusOK, resp)
}

// ScoringPrecision returns rejection/spam rates by score band for the last 30 days.
func (h *AnalyticsHandler) ScoringPrecision(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)

	rows, err := h.q.ScoringPrecisionByBand(ctx, wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load scoring precision")
		return
	}
	type item struct {
		ScoreBand     string  `json:"score_band"`
		Total         int32   `json:"total"`
		SpamCount     int32   `json:"spam_count"`
		ArchivedCount int32   `json:"archived_count"`
		RepliedCount  int32   `json:"replied_count"`
		RejectRate    float64 `json:"reject_rate"`
	}
	resp := make([]item, 0, len(rows))
	for _, row := range rows {
		reject := row.SpamCount + row.ArchivedCount
		rate := 0.0
		if row.Total > 0 {
			rate = float64(reject) / float64(row.Total)
		}
		resp = append(resp, item{
			ScoreBand:     row.ScoreBand,
			Total:         row.Total,
			SpamCount:     row.SpamCount,
			ArchivedCount: row.ArchivedCount,
			RepliedCount:  row.RepliedCount,
			RejectRate:    rate,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// ReplyAttribution returns reply → click → signup funnel for the last 30 days.
func (h *AnalyticsHandler) ReplyAttribution(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)

	row, err := h.q.ReplyAttributionFunnel(ctx, wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load reply attribution")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int32{
		"replies_approved": row.RepliesApproved,
		"replies_posted":   row.RepliesPosted,
		"utm_clicks":       row.UtmClicks,
		"utm_signups":      row.UtmSignups,
	})
}

// ReplyStyleAttribution breaks down UTM outcomes by reply template style (30d).
func (h *AnalyticsHandler) ReplyStyleAttribution(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)

	rows, err := h.q.ReplyStyleAttribution(ctx, wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load reply style attribution")
		return
	}
	type item struct {
		TemplateStyle string `json:"template_style"`
		ReplyCount    int32  `json:"reply_count"`
		PostedCount   int32  `json:"posted_count"`
		ClickCount    int32  `json:"click_count"`
		SignupCount   int32  `json:"signup_count"`
	}
	resp := make([]item, len(rows))
	for i, row := range rows {
		resp[i] = item{
			TemplateStyle: row.TemplateStyle,
			ReplyCount:    row.ReplyCount,
			PostedCount:   row.PostedCount,
			ClickCount:    row.ClickCount,
			SignupCount:   row.SignupCount,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
