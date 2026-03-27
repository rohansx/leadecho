package monitor

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// CheckReplyEngagements checks engagement metrics for recently posted replies.
// Should be called on a 30-minute timer, separate from the 5-minute crawl tick.
func (m *Monitor) CheckReplyEngagements(ctx context.Context) {
	since := pgtype.Timestamptz{Time: time.Now().Add(-7 * 24 * time.Hour), Valid: true}

	replies, err := m.q.ListPostedRepliesSince(ctx, since)
	if err != nil {
		m.logger.Error().Err(err).Msg("engagement: failed to list posted replies")
		return
	}

	if len(replies) == 0 {
		return
	}

	for _, r := range replies {
		// Fetch engagement data by platform
		var upvotes, downvotes, replyCount int
		var isRemoved bool

		switch r.Platform {
		case "reddit":
			upvotes, downvotes, replyCount, isRemoved = m.checkRedditEngagement(ctx, r.Url)
		default:
			// Skip unsupported platforms for now
			continue
		}

		// Store engagement snapshot
		m.q.CreateReplyEngagement(ctx, database.CreateReplyEngagementParams{
			ReplyID:     r.ID,
			WorkspaceID: r.WorkspaceID,
			Upvotes:     int32(upvotes),
			Downvotes:   int32(downvotes),
			ReplyCount:  int32(replyCount),
			IsRemoved:   isRemoved,
		})
	}

	m.logger.Info().Int("checked", len(replies)).Msg("engagement: check complete")
}

// checkRedditEngagement fetches engagement metrics for a Reddit comment.
func (m *Monitor) checkRedditEngagement(ctx context.Context, url string) (upvotes, downvotes, replyCount int, isRemoved bool) {
	if m.scrapling == nil {
		return 0, 0, 0, false
	}

	// Fetch the thread JSON to check comment status
	comments, err := fetchRedditThread(ctx, url)
	if err != nil {
		// If we can't fetch, might be removed
		return 0, 0, 0, true
	}

	// Basic heuristic: if we got comments, thread is accessible
	_ = comments
	return 0, 0, len(comments), false
}
