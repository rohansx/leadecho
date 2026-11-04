package monitor

// enrich (P2 #16): tracked LinkedIn / X profiles. Each tick polls the
// profiles whose last poll is older than the interval, through treg
// user-posts tools; new posts become mentions and go through scoring.

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
	"leadecho/internal/treg"
)

const (
	tregLinkedInUserPostsTool = "crawl4ai.linkedin.user.posts" // $0.0009/call, empty replies free
	tregXUserPostsTool        = "treg.x.user.posts"            // routed, ~$0.0005/hit
	trackedPollBatch          = 50
	// Posts this much older than the tracking start are history, not news.
	trackedHistoryWindow = 7 * 24 * time.Hour
)

// SetTrackedProfilesInterval sets how often each profile is polled; 0 = off.
func (m *Monitor) SetTrackedProfilesInterval(d time.Duration) { m.trackedEvery = d }

// Treg exposes the shared treg client (nil = off) to API handlers.
func (m *Monitor) Treg() *treg.Client {
	if m == nil {
		return nil
	}
	return m.treg
}

// pollTrackedProfiles runs once per tick across workspaces.
//
// ponytail: serial, bounded to trackedPollBatch profiles per tick. Ceiling:
// ~50 profiles per tick interval; upgrade to a worker pool if that's short.
func (m *Monitor) pollTrackedProfiles(ctx context.Context) {
	if !m.treg.Enabled() || m.trackedEvery <= 0 {
		return
	}
	due, err := m.q.ListDueTrackedProfiles(ctx, database.ListDueTrackedProfilesParams{
		Before: time.Now().Add(-m.trackedEvery), MaxRows: trackedPollBatch,
	})
	if err != nil {
		m.logger.Error().Err(err).Msg("tracked profiles: list due")
		return
	}
	byWS := map[string][]mentionAlert{}
	var order []string
	for _, p := range due {
		if ctx.Err() != nil {
			return
		}
		alerts := m.pollTrackedProfile(ctx, p)
		if err := m.q.MarkTrackedProfilePolled(ctx, database.MarkTrackedProfilePolledParams{ID: p.ID, WorkspaceID: p.WorkspaceID}); err != nil {
			m.logger.Warn().Err(err).Str("tracked_profile_id", p.ID).Msg("tracked profiles: mark polled")
		}
		if _, seen := byWS[p.WorkspaceID]; !seen {
			order = append(order, p.WorkspaceID)
		}
		byWS[p.WorkspaceID] = append(byWS[p.WorkspaceID], alerts...)
	}
	for _, ws := range order {
		if len(byWS[ws]) > 0 {
			m.handleNewMentionBatch(ctx, ws, byWS[ws], "tracked_profile")
		}
	}
}

func (m *Monitor) pollTrackedProfile(ctx context.Context, p database.TrackedProfile) []mentionAlert {
	tool, body := trackedRequest(p)
	posts, ok := m.tregSearch(ctx, p.WorkspaceID, tool, body)
	if !ok {
		return nil
	}
	var alerts []mentionAlert
	for _, post := range posts {
		params, ok := trackedMentionParams(p, post)
		if !ok {
			continue
		}
		if a := m.insertMention(ctx, params, "@"+p.Handle); a != nil {
			alerts = append(alerts, *a)
		}
	}
	return alerts
}

func trackedRequest(p database.TrackedProfile) (string, map[string]any) {
	if p.Platform == "x" {
		return tregXUserPostsTool, map[string]any{"username": p.Handle}
	}
	return tregLinkedInUserPostsTool, map[string]any{"profile": p.Handle, "limit": 10}
}

// trackedMentionParams maps one post to a mention. PlatformIDs match the
// keyword crawlers ("twitter_<id>", "linkedin_<activity>") so they dedupe.
func trackedMentionParams(p database.TrackedProfile, post tregPost) (database.CreateMentionParams, bool) {
	if !post.CreatedAt.IsZero() && post.CreatedAt.Before(p.CreatedAt.Add(-trackedHistoryWindow)) {
		return database.CreateMentionParams{}, false
	}
	platform, platformID, url, authorURL := "", "", post.URL, p.Url
	switch p.Platform {
	case "x":
		id := post.ID
		if id == "" {
			id = extractStatusID(post.URL)
		}
		if id == "" {
			return database.CreateMentionParams{}, false
		}
		platform, platformID = string(database.PlatformTypeTwitter), "twitter_"+id
		if url == "" || strings.Contains(url, "/i/web/status/") {
			url = "https://x.com/" + p.Handle + "/status/" + id
		}
		if authorURL == "" {
			authorURL = "https://x.com/" + p.Handle
		}
	default:
		id := extractLinkedInID(post.URL)
		if id == "" || url == "" {
			return database.CreateMentionParams{}, false
		}
		platform, platformID = string(database.PlatformTypeLinkedin), "linkedin_"+id
		if authorURL == "" {
			authorURL = "https://www.linkedin.com/in/" + p.Handle
		}
	}
	created := pgtype.Timestamptz{}
	if !post.CreatedAt.IsZero() {
		created = pgtype.Timestamptz{Time: post.CreatedAt, Valid: true}
	}
	return database.CreateMentionParams{
		WorkspaceID:       p.WorkspaceID,
		Platform:          platform,
		PlatformID:        platformID,
		Url:               url,
		Content:           post.Text,
		AuthorUsername:    pgtextPtr(p.Handle),
		AuthorProfileUrl:  pgtextPtr(authorURL),
		Status:            database.MentionStatusNew,
		PlatformMetadata:  jsonBytes(map[string]any{"source": "tracked_profile", "tracked_profile_id": p.ID}),
		EngagementMetrics: jsonBytes(map[string]any{"likes": post.Likes, "replies": post.Replies, "reposts": post.Reposts}),
		KeywordMatches:    []string{},
		PlatformCreatedAt: created,
	}, true
}
