package monitor

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/ai"
	"leadecho/internal/browser"
	"leadecho/internal/database"
	"leadecho/internal/embedding"
)

// Monitor polls social platforms for keyword matches and inserts new mentions.
type Monitor struct {
	q             *database.Queries
	logger        zerolog.Logger
	resendAPIKey  string
	redditBackoff time.Time                // skip Reddit crawls until this time (set on 429)
	embedder      *embedding.Client        // Voyage AI embedder (nil if not configured)
	aiProvider    *ai.Provider             // LLM provider for auto-classification (nil if not configured)
	pinchtab      *browser.PinchtabClient  // browser sidecar (nil if not configured)
	camoufox      *browser.CamoufoxClient  // Pro-tier stealth Firefox sidecar (nil if not configured)
	encKey        []byte                   // AES key for decrypting session cookies
}

func New(q *database.Queries, logger zerolog.Logger, resendAPIKey string, embedder *embedding.Client, aiProvider *ai.Provider, pinchtab *browser.PinchtabClient, camoufox *browser.CamoufoxClient, encKey []byte) *Monitor {
	return &Monitor{
		q:            q,
		logger:       logger,
		resendAPIKey: resendAPIKey,
		embedder:     embedder,
		aiProvider:   aiProvider,
		pinchtab:     pinchtab,
		camoufox:     camoufox,
		encKey:       encKey,
	}
}

// Run starts polling all workspaces on an interval. Blocks until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context, interval time.Duration) {
	m.logger.Info().Dur("interval", interval).Msg("monitor started")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	m.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info().Msg("monitor stopped")
			return
		case <-ticker.C:
			m.tick(ctx)
		}
	}
}

func (m *Monitor) tick(ctx context.Context) {
	// Fetch all active keywords across every workspace
	allKeywords, err := m.q.ListAllActiveKeywords(ctx)
	if err != nil {
		m.logger.Error().Err(err).Msg("failed to list keywords")
		return
	}

	if len(allKeywords) == 0 {
		return
	}

	// Group keywords by workspace so we can send per-workspace notifications
	type wsKeywords struct {
		wsID     string
		keywords []database.ListAllActiveKeywordsRow
	}
	groups := map[string]*wsKeywords{}
	var order []string
	for _, kw := range allKeywords {
		g, ok := groups[kw.WorkspaceID]
		if !ok {
			g = &wsKeywords{wsID: kw.WorkspaceID}
			groups[kw.WorkspaceID] = g
			order = append(order, kw.WorkspaceID)
		}
		g.keywords = append(g.keywords, kw)
	}

	for _, wsID := range order {
		g := groups[wsID]
		var alerts []mentionAlert

		for i, kw := range g.keywords {
			// Convert to ListActiveKeywordsRow (same shape) for crawl functions
			akw := database.ListActiveKeywordsRow{
				ID:            kw.ID,
				WorkspaceID:   kw.WorkspaceID,
				Term:          kw.Term,
				Platforms:     kw.Platforms,
				IsActive:      kw.IsActive,
				MatchType:     kw.MatchType,
				NegativeTerms: kw.NegativeTerms,
				Subreddits:    kw.Subreddits,
				CreatedAt:     kw.CreatedAt,
				UpdatedAt:     kw.UpdatedAt,
			}

			for _, platform := range kw.Platforms {
				switch database.PlatformType(platform) {
				case database.PlatformTypeReddit:
					// Try Pinchtab first (authenticated, no 429s); fall back to unauthenticated
					if m.pinchtab != nil {
						if pinchtabAlerts := m.crawlRedditPinchtab(ctx, wsID, akw); pinchtabAlerts != nil {
							alerts = append(alerts, pinchtabAlerts...)
							break
						}
					}
					// Fallback: existing unauthenticated crawler
					if time.Now().Before(m.redditBackoff) {
						continue
					}
					results := m.crawlReddit(ctx, wsID, akw)
					alerts = append(alerts, results...)
				case database.PlatformTypeHackernews:
					alerts = append(alerts, m.crawlHackerNews(ctx, wsID, akw)...)
				case database.PlatformTypeTwitter:
					if m.pinchtab != nil {
						alerts = append(alerts, m.crawlTwitterPinchtab(ctx, wsID, akw)...)
					}
				case database.PlatformTypeLinkedin:
					if m.camoufox != nil {
						alerts = append(alerts, m.crawlLinkedInCamoufox(ctx, wsID, akw)...)
					} else if m.pinchtab != nil {
						alerts = append(alerts, m.crawlLinkedInPinchtab(ctx, wsID, akw)...)
					}
				default:
					// Phase 2 platforms
					switch platform {
					case "devto":
						alerts = append(alerts, m.crawlDevTo(ctx, wsID, akw)...)
					case "lobsters":
						alerts = append(alerts, m.crawlLobsters(ctx, wsID, akw)...)
					case "indiehackers":
						alerts = append(alerts, m.crawlIndieHackers(ctx, wsID, akw)...)
					}
				}
			}

			// Pause between keywords to avoid hammering APIs
			if i < len(g.keywords)-1 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
				}
			}
		}

		// Auto-score new mentions (4-stage pipeline)
		m.batchScoreMentions(ctx, wsID, alerts)

		// Fire webhook notifications for this workspace
		m.notifyNewMentions(ctx, wsID, alerts)
	}
}

// insertMention creates a mention if it does not already exist.
// Returns a mentionAlert if the mention was new, or nil if it was a duplicate.
func (m *Monitor) insertMention(ctx context.Context, p database.CreateMentionParams, keyword string) *mentionAlert {
	mention, err := m.q.CreateMention(ctx, p)
	if err != nil {
		if isDuplicateError(err) {
			return nil
		}
		m.logger.Error().Err(err).Str("platform_id", p.PlatformID).Msg("failed to insert mention")
		return nil
	}

	title := ""
	if p.Title.Valid {
		title = p.Title.String
	}
	author := ""
	if p.AuthorUsername.Valid {
		author = p.AuthorUsername.String
	}

	return &mentionAlert{
		ID:          mention.ID,
		WorkspaceID: p.WorkspaceID,
		Platform:    string(p.Platform),
		Keyword:     keyword,
		Title:       title,
		URL:         p.Url,
		Author:      author,
		Content:     p.Content,
	}
}

func isDuplicateError(err error) bool {
	return err != nil && (contains(err.Error(), "duplicate key") || contains(err.Error(), "unique constraint"))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func pgUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}
	}
	return u
}

func pgtextPtr(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func pgint4Ptr(n int) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(n), Valid: true}
}

func jsonBytes(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
