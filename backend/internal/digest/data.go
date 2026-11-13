package digest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"leadecho/internal/database"
	"leadecho/internal/demand"
)

// Build loads a workspace's digest data for one period. Every query is
// scoped to wsID.
func (s *Service) Build(ctx context.Context, wsID string, kind Kind, p Period) (Data, error) {
	d := Data{Kind: kind, Period: p, DashboardURL: s.frontendURL}

	top, err := s.q.DigestTopMentions(ctx, database.DigestTopMentionsParams{
		WorkspaceID: wsID, PeriodStart: p.Start, PeriodEnd: p.End, MaxRows: MaxTopLeads,
	})
	if err != nil {
		return Data{}, fmt.Errorf("digest top mentions: %w", err)
	}
	for _, m := range top {
		d.TopLeads = append(d.TopLeads, Lead{
			MentionID: m.ID,
			Title:     firstNonEmpty(m.Title.String, truncate(m.Content, 120)), URL: m.Url, Platform: m.Platform,
			Author: m.AuthorUsername.String, Intent: leadIntent(string(m.Intent.IntentType), m.IntentTags), Score: float64(m.RelevanceScore.Float32),
		})
	}

	platforms, err := s.q.DigestPlatformCounts(ctx, database.DigestPlatformCountsParams{WorkspaceID: wsID, PeriodStart: p.Start, PeriodEnd: p.End})
	if err != nil {
		return Data{}, fmt.Errorf("digest platform counts: %w", err)
	}
	for _, c := range platforms {
		d.Platforms = append(d.Platforms, Count{c.Platform, int(c.Count)})
	}

	intents, err := s.q.DigestIntentCounts(ctx, database.DigestIntentCountsParams{WorkspaceID: wsID, PeriodStart: p.Start, PeriodEnd: p.End})
	if err != nil {
		return Data{}, fmt.Errorf("digest intent counts: %w", err)
	}
	for _, c := range intents {
		d.Intents = append(d.Intents, Count{c.Intent, int(c.Count)})
	}

	kws, err := s.q.DigestKeywordStats(ctx, database.DigestKeywordStatsParams{WorkspaceID: wsID, PeriodStart: p.Start, PeriodEnd: p.End})
	if err != nil {
		return Data{}, fmt.Errorf("digest keyword stats: %w", err)
	}
	stats := make([]KeywordStat, 0, len(kws))
	for _, k := range kws {
		stats = append(stats, KeywordStat{k.Term, int(k.MentionCount), k.AvgScore})
	}
	d.NoisyKeywords = NoisyKeywords(stats)

	replies, err := s.q.DigestRepliesPosted(ctx, database.DigestRepliesPostedParams{WorkspaceID: wsID, PeriodStart: p.Start, PeriodEnd: p.End})
	if err != nil {
		return Data{}, fmt.Errorf("digest replies: %w", err)
	}
	d.RepliesPosted = int(replies)

	attr, err := s.q.DigestAttribution(ctx, database.DigestAttributionParams{WorkspaceID: wsID, PeriodStart: p.Start, PeriodEnd: p.End})
	if err != nil {
		return Data{}, fmt.Errorf("digest attribution: %w", err)
	}
	d.Clicks, d.Signups, d.RevenueCents = int(attr.Clicks), int(attr.Signups), attr.RevenueCents

	d.HiringConfirmed, d.HiringHeld, err = s.q.DigestHiringSignals(ctx, wsID, p.Start, p.End)
	if err != nil {
		return Data{}, fmt.Errorf("digest hiring signals: %w", err)
	}

	if kind == KindWeekly {
		d.Demand = s.demandThemes(ctx, wsID, p.End)
	}

	funnel, err := s.q.ConversionFunnel(ctx, wsID)
	if err != nil {
		return Data{}, fmt.Errorf("digest pipeline: %w", err)
	}
	byStage := map[string]int{}
	for _, f := range funnel {
		byStage[string(f.Stage)] = int(f.Count)
	}
	d.Pipeline = PipelineCounts(byStage)
	return d, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// leadIntent labels a lead by its intent tags (P0 #6), falling back to the
// legacy single intent for mentions scored before tags existed.
func leadIntent(intent string, tags []string) string {
	if len(tags) > 0 {
		return strings.Join(tags, ", ")
	}
	return intent
}

// demandThemes returns the open themes with the most new asks this week.
// A read failure leaves the section out; the digest still goes.
func (s *Service) demandThemes(ctx context.Context, wsID string, now time.Time) []DemandTheme {
	state, err := demand.DBStore{Q: s.q}.LoadState(ctx, wsID)
	if err != nil {
		return nil
	}
	var out []DemandTheme
	for _, t := range state.View(now) {
		// One person is not a theme, and a need for a different kind of
		// product is not news for this team.
		if t.Status != demand.StatusOpen || t.NewThisWeek == 0 || t.People < demand.MinThemePeople || t.Coverage == demand.CoverageOutside {
			continue
		}
		out = append(out, DemandTheme{Title: t.Title, People: t.People, Buyers: t.Buyers, NewThisWeek: t.NewThisWeek, Coverage: t.Coverage})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].NewThisWeek > out[j].NewThisWeek })
	if len(out) > maxDigestThemes {
		out = out[:maxDigestThemes]
	}
	return out
}
