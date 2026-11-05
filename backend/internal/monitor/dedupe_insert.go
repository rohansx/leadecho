package monitor

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// scoring: insert-path wiring for the bot filter and dedupe.

type dedupeKeys struct {
	canonicalURL string
	simhash      int64
	maxDistance  int
}

func dedupeKeysFor(p database.CreateMentionParams) dedupeKeys {
	norm := NormalizeText(p.Title.String + " " + p.Content)
	return dedupeKeys{
		canonicalURL: CanonicalURL(p.Url),
		simhash:      int64(SimHash(norm)),
		maxDistance:  nearDupMaxDistance(norm),
	}
}

// dropBeforeInsert reports whether a fetched mention should be skipped: bot
// author/body, or a post already seen in this workspace within 7 days under
// another URL form or with near-identical text. A failed duplicate lookup
// fails open (inserts) so a DB hiccup never loses mentions.
func (m *Monitor) dropBeforeInsert(ctx context.Context, p database.CreateMentionParams) (dedupeKeys, bool) {
	keys := dedupeKeysFor(p)
	if IsBotMention(p.AuthorUsername.String, p.Content) {
		m.logger.Debug().Str("author", p.AuthorUsername.String).Str("url", p.Url).Msg("monitor: dropped bot mention")
		return keys, true
	}
	dup, err := m.q.FindRecentDuplicateMention(ctx, database.FindRecentDuplicateMentionParams{
		WorkspaceID:  p.WorkspaceID,
		LookbackDays: dedupeLookbackDays,
		CanonicalUrl: keys.canonicalURL,
		MaxDistance:  int32(keys.maxDistance),
		Simhash:      keys.simhash,
	})
	if err != nil {
		m.logger.Warn().Err(err).Str("url", p.Url).Msg("monitor: dedupe lookup failed, inserting")
		return keys, false
	}
	if dup {
		m.logger.Debug().Str("url", p.Url).Msg("monitor: dropped duplicate mention")
	}
	return keys, dup
}

func (m *Monitor) recordDedupeKeys(ctx context.Context, wsID, mentionID string, k dedupeKeys) {
	if err := m.q.SetMentionDedupeKeys(ctx, database.SetMentionDedupeKeysParams{
		CanonicalUrl:   pgtype.Text{String: k.canonicalURL, Valid: k.canonicalURL != ""},
		ContentSimhash: pgtype.Int8{Int64: k.simhash, Valid: k.maxDistance >= 0},
		ID:             mentionID,
		WorkspaceID:    wsID,
	}); err != nil {
		m.logger.Error().Err(err).Str("mention_id", mentionID).Msg("monitor: store dedupe keys")
	}
}
