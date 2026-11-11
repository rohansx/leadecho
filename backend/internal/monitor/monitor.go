package monitor

import (
	"context"
	"encoding/json"
	"html"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/billing"
	"leadecho/internal/browser"
	"leadecho/internal/crmsync"
	"leadecho/internal/database"
	"leadecho/internal/events/publishers"
	"leadecho/internal/llm"
	"leadecho/internal/mutes"
	"leadecho/internal/treg" // treg: hosted X/LinkedIn sources
	"leadecho/internal/workflow"
)

// LeadEnricher runs Person360 enrichment after lead qualification.
type LeadEnricher interface {
	EnrichLeadAsync(leadID, workspaceID string)
}

// Monitor polls social platforms for keyword matches and inserts new mentions.
type Monitor struct {
	crm               *crmsync.Dispatcher
	workflows         WorkflowRunner
	q                 *database.Queries
	logger            zerolog.Logger
	resendAPIKey      string
	redditBackoffNs   atomic.Int64             // unix-nano; skip Reddit crawls until then (set on 429/403). Atomic: scans run concurrently.
	scanning          sync.Map                 // workspaceID -> struct{}; prevents overlapping scans of one workspace
	painPointsChecked sync.Map                 // workspaceID -> struct{}; pain-point embedding backfill done
	llmRouter         llm.MentionScorer        // workspace-aware LLM/embedding router
	scrapling         *browser.ScraplingClient // Scrapling website-reading sidecar (nil if not configured)
	exaAPIKey         string                   // Exa web-search API key (empty disables the exa source)
	eventPublisher    *publishers.Publisher
	enricher          LeadEnricher
	streamsEnabled    bool
	streamsDualWrite  bool
	inlineFallback    bool
	qualifierAsync    bool

	// googleNewsEnabled opts in to the Google News RSS source. Off by default:
	// Google's feed terms limit it to personal, non-commercial use, so it must
	// be a deliberate operator choice rather than a default in a shipped build.
	googleNewsEnabled bool

	// onboarding: public search endpoints (nil = production defaults; tests override).
	public *PublicSources
	// scoring: extra few-shot example sources (e.g. onboarding calibration)
	extraExamples []ExampleSource
	// treg: hosted X/LinkedIn search (nil = X/LinkedIn off). Reddit comment
	// search via the public JSON stream (see reddit_comments.go).
	treg                  *treg.Client
	hiringPlanner         HiringPlanner    // optional: company hiring signals
	credits               *billing.Credits // billing: nil or unenforced never blocks
	redditCommentsEnabled bool
	// sources: workspace mute lists applied before insert (nil = none).
	mutes *mutes.Store

	// sources: optional Bluesky session (BLUESKY_HANDLE/BLUESKY_APP_PASSWORD).
	bluesky blueskyAuth

	// enrich: tracked LinkedIn/X profile poll interval (0 = off).
	trackedEvery time.Duration
}

// SetGoogleNewsEnabled toggles the opt-in Google News source. Kept as a setter
// rather than another positional argument on an already 14-parameter New().
func (m *Monitor) SetGoogleNewsEnabled(v bool) { m.googleNewsEnabled = v }

// SetCRMSync wires outbound lead events (webhooks/HubSpot/Attio). Optional.
func (m *Monitor) SetCRMSync(d *crmsync.Dispatcher) { m.crm = d }

// WorkflowRunner runs real-time workflows for a scored lead (*workflow.Engine).
type WorkflowRunner interface {
	Run(ctx context.Context, l workflow.Lead)
}

func (m *Monitor) SetWorkflows(w WorkflowRunner) { m.workflows = w }

func New(q *database.Queries, logger zerolog.Logger, resendAPIKey string, llmRouter llm.MentionScorer, scrapling *browser.ScraplingClient, exaAPIKey string, eventPublisher *publishers.Publisher, streamsEnabled, streamsDualWrite, inlineFallback, qualifierAsync bool, enricher LeadEnricher) *Monitor {
	return &Monitor{
		q:                q,
		logger:           logger,
		resendAPIKey:     resendAPIKey,
		llmRouter:        llmRouter,
		scrapling:        scrapling,
		exaAPIKey:        exaAPIKey,
		eventPublisher:   eventPublisher,
		enricher:         enricher,
		streamsEnabled:   streamsEnabled,
		streamsDualWrite: streamsDualWrite,
		inlineFallback:   inlineFallback,
		qualifierAsync:   qualifierAsync,
		mutes:            mutes.NewStore(q), // sources
	}
}

// SetCredits meters scoring and pauses scans at an empty balance.
func (m *Monitor) SetCredits(c *billing.Credits) { m.credits = c }

// Run starts polling all workspaces on an interval. Blocks until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context, interval time.Duration) {
	m.logger.Info().Dur("interval", interval).Msg("monitor started")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	engagementTicker := time.NewTicker(30 * time.Minute)
	defer engagementTicker.Stop()

	// Run immediately on start
	m.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info().Msg("monitor stopped")
			return
		case <-ticker.C:
			m.tick(ctx)
		case <-engagementTicker.C:
			m.CheckReplyEngagements(ctx)
		}
	}
}

func (m *Monitor) tick(ctx context.Context) {
	// Top-level safety net: a panic anywhere in a tick (crawl, scoring, notify)
	// must never take down the API process — log it and wait for the next tick.
	defer func() {
		if r := recover(); r != nil {
			m.logger.Error().
				Interface("panic", r).
				Str("stack", string(debug.Stack())).
				Msg("monitor: recovered from panic in tick")
		}
	}()

	if _, err := m.credits.RefreshFree(ctx); err != nil { // billing: free plan monthly top-up
		m.logger.Warn().Err(err).Msg("billing: free credit refresh failed")
	}
	m.pollTrackedProfiles(ctx) // enrich: tracked LinkedIn/X profiles

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
		kws := make([]database.ListActiveKeywordsRow, 0, len(g.keywords))
		for _, kw := range g.keywords {
			// Convert to ListActiveKeywordsRow (same shape) for crawl functions
			kws = append(kws, database.ListActiveKeywordsRow{
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
			})
		}
		if ctx.Err() != nil {
			return
		}
		m.scanWorkspace(ctx, wsID, kws)
	}
}

// scanWorkspace crawls one workspace's keywords and processes the new mentions.
// Skips (returns false) if that workspace is already being scanned.
//
// ponytail: the periodic tick still walks workspaces serially, so one large
// workspace delays the rest. Upgrade path: a bounded worker pool over
// workspaces (the shared state is already concurrency-safe).
func (m *Monitor) scanWorkspace(ctx context.Context, wsID string, kws []database.ListActiveKeywordsRow) bool {
	if _, busy := m.scanning.LoadOrStore(wsID, struct{}{}); busy {
		return false
	}
	defer m.scanning.Delete(wsID)

	// billing/ops: paused in /ops, or out of credits; posts already found stay.
	if !m.credits.ScanAllowed(ctx, wsID) {
		m.logger.Debug().Str("workspace_id", wsID).Msg("monitor: scan paused (ops or out of credits)")
		return false
	}

	if m.credits.PaidPlatformsLocked(ctx, wsID) { // billing: X/Reddit/LinkedIn need a paid plan
		kws = withoutPaidPlatforms(kws)
	}
	kws = m.applyCadence(ctx, wsID, kws) // sources: per-platform cadence
	var alerts []mentionAlert
	for i, kw := range kws {
		alerts = append(alerts, m.crawlKeyword(ctx, wsID, kw)...)
		m.recordScans(ctx, wsID, kw) // sources
		// Pause between keywords to avoid hammering APIs
		if i < len(kws)-1 {
			select {
			case <-ctx.Done():
				return true
			case <-time.After(3 * time.Second):
			}
		}
	}
	m.handleNewMentionBatch(ctx, wsID, alerts, "monitor")
	m.scanHiring(ctx, wsID) // company hiring signals (daily, paid data only)
	return true
}

// ScanWorkspaceNow starts an immediate background scan of one workspace, e.g.
// right after onboarding, so new users don't wait for the next tick (which can
// be many minutes behind on a busy instance).
func (m *Monitor) ScanWorkspaceNow(wsID string) { m.ScanWorkspaceSince(wsID, 0) }

// onboarding: ScanWorkspaceSince is ScanWorkspaceNow with a backfill window:
// sources that support time filters (HN, Reddit, Dev.to) pull posts from the
// last lookback instead of only the newest page. lookback 0 = normal scan.
func (m *Monitor) ScanWorkspaceSince(wsID string, lookback time.Duration) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error().Interface("panic", r).Str("workspace_id", wsID).Msg("monitor: recovered from panic in immediate scan")
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if lookback > 0 {
			ctx = WithSince(ctx, time.Now().Add(-lookback))
		}
		kws, err := m.q.ListActiveKeywords(ctx, wsID)
		if err != nil {
			m.logger.Error().Err(err).Str("workspace_id", wsID).Msg("monitor: immediate scan: list keywords")
			return
		}
		if len(kws) == 0 {
			return
		}
		m.logger.Info().Str("workspace_id", wsID).Int("keywords", len(kws)).Msg("monitor: immediate workspace scan")
		m.scanWorkspace(ctx, wsID, kws)
	}()
}

// onboarding: IsScanning reports whether a scan of the workspace is running.
func (m *Monitor) IsScanning(wsID string) bool {
	_, busy := m.scanning.Load(wsID)
	return busy
}

func (m *Monitor) redditBackedOff() bool { return time.Now().UnixNano() < m.redditBackoffNs.Load() }

func (m *Monitor) setRedditBackoff(d time.Duration) {
	m.redditBackoffNs.Store(time.Now().Add(d).UnixNano())
}

// crawlKeyword runs every configured platform crawler for a single keyword.
// It recovers from panics so that one misbehaving platform/keyword cannot abort
// the whole tick or crash the process; the offending keyword is simply skipped.
func (m *Monitor) crawlKeyword(ctx context.Context, wsID string, akw database.ListActiveKeywordsRow) (alerts []mentionAlert) {
	defer func() {
		if r := recover(); r != nil {
			m.logger.Error().
				Interface("panic", r).
				Str("keyword", akw.Term).
				Str("stack", string(debug.Stack())).
				Msg("monitor: recovered from crawl panic — skipping keyword")
			alerts = nil
		}
	}()

	for _, platform := range akw.Platforms {
		switch database.PlatformType(platform) {
		case database.PlatformTypeReddit:
			// treg feature: keyword hits inside comments (free public JSON).
			if m.redditCommentsEnabled {
				alerts = append(alerts, m.crawlRedditComments(ctx, wsID, akw)...)
			}
			// Hosted treg search unless official Reddit OAuth is set; then
			// the public crawler (OAuth-aware).
			if m.redditViaTreg() {
				if tregAlerts, ok := m.crawlRedditTreg(ctx, wsID, akw); ok {
					alerts = append(alerts, tregAlerts...)
					break
				}
			}
			// Fallback: existing unauthenticated crawler
			if m.redditBackedOff() {
				continue
			}
			results := m.crawlReddit(ctx, wsID, akw)
			alerts = append(alerts, results...)
		case database.PlatformTypeHackernews:
			alerts = append(alerts, m.crawlHackerNews(ctx, wsID, akw)...)
		case database.PlatformTypeExa:
			if m.exaAPIKey != "" {
				alerts = append(alerts, m.crawlExa(ctx, wsID, akw)...)
			}
		case database.PlatformTypeGooglenews:
			// Opt-in: Google's RSS terms restrict the feed to personal,
			// non-commercial use, so it stays off unless an operator enables it.
			if m.googleNewsEnabled {
				alerts = append(alerts, m.crawlGoogleNews(ctx, wsID, akw)...)
			}
		case database.PlatformTypeTwitter:
			// treg feature: hosted search is the only X source.
			if a, ok := m.crawlTwitterTreg(ctx, wsID, akw); ok {
				alerts = append(alerts, a...)
			}
		case database.PlatformTypeLinkedin:
			// treg feature: hosted search is the only LinkedIn source.
			if a, ok := m.crawlLinkedInTreg(ctx, wsID, akw); ok {
				alerts = append(alerts, a...)
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
			case "googlenews":
				alerts = append(alerts, m.crawlGoogleNews(ctx, wsID, akw)...)
			case "bluesky": // sources
				alerts = append(alerts, m.crawlBluesky(ctx, wsID, akw)...)
			}
		}
	}

	return alerts
}

// insertMention creates a mention if it does not already exist.
// Returns a mentionAlert if the mention was new, or nil if it was a duplicate.
func (m *Monitor) insertMention(ctx context.Context, p database.CreateMentionParams, keyword string) *mentionAlert {
	// Defense-in-depth: several authenticated/sidecar crawlers build
	// CreateMentionParams without a Status; the Go zero value "" is rejected by
	// the mention_status enum and would fail every insert. Default to "new".
	if p.Status == "" {
		p.Status = database.MentionStatusNew
	}
	// Platform APIs (dev.to, Reddit, HN) return entity-escaped text; decode
	// once here so every crawler stores readable text.
	p.Content = html.UnescapeString(p.Content)
	p.Title.String = html.UnescapeString(p.Title.String)
	if tooOld(ctx, p.PlatformCreatedAt, time.Now()) {
		return nil
	}
	if m.isMuted(ctx, p) { // sources: mute lists
		return nil
	}
	// scoring: drop bots and cross-link / near-duplicate posts before insert
	keys, drop := m.dropBeforeInsert(ctx, p)
	if drop {
		return nil
	}
	mention, err := m.q.CreateMention(ctx, p)
	if err != nil {
		if isDuplicateError(err) {
			return nil
		}
		m.logger.Error().Err(err).Str("platform_id", p.PlatformID).Msg("failed to insert mention")
		return nil
	}
	m.recordDedupeKeys(ctx, p.WorkspaceID, mention.ID, keys)

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
