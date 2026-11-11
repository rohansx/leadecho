package monitor

import (
	"context"
	"errors"
	"strconv"
	"time"

	"leadecho/internal/billing"
	"leadecho/internal/database"
	"leadecho/internal/treg"
)

// Catalog tools (see docs/treg-integration.md for why these).
const (
	tregXTool        = "treg.x.search.posts"          // routed, $0.00075/hit, misses free
	tregLinkedInTool = "anyapi.linkedin.search.posts" // $0.002/success, misses free
	// anyapi answers in ~3s. crawl4ai.reddit.search.posts (used before) took
	// over 90s per search, past our timeout, so Reddit never returned anything.
	tregRedditTool = "anyapi.reddit.search" // ~$0.0004-0.0012/call
)

// SetTreg enables the hosted X/LinkedIn sources. nil (TREG_TOKEN unset)
// disables X and LinkedIn (Reddit uses its public crawler).
func (m *Monitor) SetTreg(c *treg.Client) { m.treg = c }

// What the X and LinkedIn crawlers remember between polls, so a poll that
// would only return posts we already have costs less or nothing.
const (
	// An X cursor unused this long is dropped; the next poll is a full search.
	xSinceTTL = 7 * 24 * time.Hour
	// LinkedIn bills every call, found or not, so cost follows poll count.
	// ponytail: fixed 12h (was every TREG_CACHE_TTL, 4h); make it an env var
	// if a plan ever needs fresher LinkedIn leads.
	linkedInPollEvery = 12 * time.Hour
	// While the last poll is at most this old, "last-day" covers the gap.
	// Under 24h so a poll right at the edge can't miss posts.
	linkedInDayCover = 22 * time.Hour
)

// SetTregState gives the X/LinkedIn crawlers somewhere to remember their last
// poll (Redis). nil = every poll is a full search, as before.
func (m *Monitor) SetTregState(s treg.Cache) { m.tregState = s }

// Per workspace and keyword; the term is part of the key so an edited keyword
// starts fresh.
func tregStateKey(kind, wsID string, kw database.ListActiveKeywordsRow) string {
	return "treg:state:" + kind + ":" + wsID + ":" + kw.ID + ":" + kw.Term
}

func (m *Monitor) tregStateGet(ctx context.Context, key string) string {
	if m.tregState == nil {
		return ""
	}
	b, _ := m.tregState.Get(ctx, key)
	return string(b)
}

func (m *Monitor) tregStateSet(ctx context.Context, key, val string, ttl time.Duration) {
	if m.tregState != nil {
		m.tregState.Set(ctx, key, []byte(val), ttl)
	}
}

// tregSearch runs one treg search. ok=false means "treg could not answer"
// and the caller falls back (Reddit only); errors never reach users.
func (m *Monitor) tregSearch(ctx context.Context, wsID, tool string, body any) ([]tregPost, bool) {
	return m.tregCall(ctx, treg.Request{WorkspaceID: wsID, Tool: tool, Body: body})
}

func (m *Monitor) tregCall(ctx context.Context, req treg.Request) ([]tregPost, bool) {
	wsID, tool := req.WorkspaceID, req.Tool
	resp, err := m.treg.Call(ctx, req)
	if err != nil {
		// Not allowed / paused / out of credits is expected, not an incident.
		quiet := treg.IsOperatorError(err) || errors.Is(err, treg.ErrCustomerDailyCap) || errors.Is(err, billing.ErrNoPaidData) ||
			errors.Is(err, billing.ErrPaidDataPaused) || errors.Is(err, billing.ErrNoCredits)
		if !quiet {
			m.logger.Warn().Err(err).Str("tool", tool).Str("workspace_id", wsID).Msg("treg: search failed, falling back")
		}
		return nil, false
	}
	return parseTregPosts(resp.Body), true
}

// crawlTwitterTreg returns ok=false when treg is off or failed. After the
// first poll it asks only for posts newer than the newest one already fetched
// (X's since_id operator), so a quiet keyword returns nothing instead of the
// same page of posts again.
func (m *Monitor) crawlTwitterTreg(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) ([]mentionAlert, bool) {
	if !m.treg.Enabled() {
		return nil, false
	}
	key := tregStateKey("x_since", wsID, kw)
	since := m.tregStateGet(ctx, key)
	q := kw.Term
	if since != "" {
		q = "(" + kw.Term + ") since_id:" + since
	}
	// NoWaterfall: an empty answer is the expected result of a cursor query.
	posts, ok := m.tregCall(ctx, treg.Request{WorkspaceID: wsID, Tool: tregXTool, Body: map[string]string{"q": q}, NoWaterfall: since != ""})
	if !ok {
		// Drop the cursor: if a provider rejects the cursor query, the next
		// poll is a plain search again rather than X going silent.
		if since != "" {
			m.tregStateSet(ctx, key, "", xSinceTTL)
		}
		return nil, false
	}
	if newest := newestTweetID(posts, since); newest != since {
		m.tregStateSet(ctx, key, newest, xSinceTTL)
	}
	return m.insertTregPosts(ctx, wsID, kw, tregXTool, posts, tregTwitterParams), true
}

// newestTweetID returns the highest numeric post ID among posts and since
// (tweet IDs grow over time). Rows without a numeric ID are ignored.
func newestTweetID(posts []tregPost, since string) string {
	best, _ := strconv.ParseUint(since, 10, 64)
	for _, p := range posts {
		id, err := strconv.ParseUint(p.ID, 10, 64)
		if err != nil {
			id, err = strconv.ParseUint(extractStatusID(p.URL), 10, 64)
		}
		if err == nil && id > best {
			best = id
		}
	}
	if best == 0 {
		return ""
	}
	return strconv.FormatUint(best, 10)
}

// crawlLinkedInTreg polls a keyword at most once per linkedInPollEvery, and
// searches only the past day when the previous poll is recent enough to have
// covered the rest of the week.
func (m *Monitor) crawlLinkedInTreg(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) ([]mentionAlert, bool) {
	if !m.treg.Enabled() {
		return nil, false
	}
	key := tregStateKey("li_polled", wsID, kw)
	window := "last-week"
	if at, err := strconv.ParseInt(m.tregStateGet(ctx, key), 10, 64); err == nil {
		if time.Since(time.Unix(at, 0)) < linkedInPollEvery {
			return nil, true // polled recently: nothing to do, not a failure
		}
		window = "last-day"
	}
	posts, ok := m.tregSearch(ctx, wsID, tregLinkedInTool, map[string]string{"query": kw.Term, "datePosted": window})
	if !ok {
		return nil, false
	}
	m.tregStateSet(ctx, key, strconv.FormatInt(time.Now().Unix(), 10), linkedInDayCover)
	return m.insertTregPosts(ctx, wsID, kw, tregLinkedInTool, posts, tregLinkedInParams), true
}

type tregMapper func(string, database.ListActiveKeywordsRow, tregPost) (database.CreateMentionParams, bool)

func (m *Monitor) crawlTreg(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow, tool string, body any, toParams tregMapper) ([]mentionAlert, bool) {
	if !m.treg.Enabled() {
		return nil, false
	}
	posts, ok := m.tregSearch(ctx, wsID, tool, body)
	if !ok {
		return nil, false
	}
	return m.insertTregPosts(ctx, wsID, kw, tool, posts, toParams), true
}

// insertTregPosts stores the posts that match the keyword as mentions.
func (m *Monitor) insertTregPosts(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow, tool string, posts []tregPost, toParams tregMapper) []mentionAlert {
	var alerts []mentionAlert
	for _, p := range posts {
		if !filterContent(p.Text, kw) {
			continue
		}
		params, ok := toParams(wsID, kw, p)
		if !ok {
			continue
		}
		if a := m.insertMention(ctx, params, kw.Term); a != nil {
			alerts = append(alerts, *a)
		}
	}
	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Str("tool", tool).Msg("treg: new mentions found")
	}
	return alerts
}

// crawlRedditTreg runs one sitewide search per keyword (newest, past week).
// ponytail: sitewide, not per subreddit: one call instead of one per
// subreddit, and it finds posts outside the suggested subreddits.
func (m *Monitor) crawlRedditTreg(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) ([]mentionAlert, bool) {
	// Relevance, not newest: Reddit's "new" sort matches the words loosely and
	// returns unrelated posts; the week window keeps results fresh anyway.
	body := map[string]any{"query": kw.Term, "sort": "relevance", "timeframe": "week"}
	return m.crawlTreg(ctx, wsID, kw, tregRedditTool, body, tregRedditParams)
}

// redditViaTreg: the official Reddit API (free) wins when configured; else
// treg, ahead of the unauthenticated public crawler.
func (m *Monitor) redditViaTreg() bool {
	return m.treg.Enabled() && m.sources().RedditOAuth == nil
}
