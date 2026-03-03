# LeadEcho - Signal Engine Implementation Guide

## Overview

The Signal Engine is the core monitoring service. It polls HN, Reddit, and X/Twitter APIs for keyword mentions, scores them with AI, and publishes events for downstream processing. LinkedIn signals come from the Chrome extension.

**Design goals:** Platform adapter pattern, independent goroutines per platform, graceful degradation with fallbacks, rate-limit-aware polling.

---

## Platform Adapter Interface

```go
package platform

import "context"

type RawMention struct {
    PlatformID   string
    Platform     string
    URL          string
    Title        string
    Content      string
    Author       Author
    Metadata     map[string]any
    Engagement   Engagement
    PlatformTime time.Time
}

type Author struct {
    Username     string
    ProfileURL   string
    Karma        int
    AccountAge   int // days
}

type Engagement struct {
    Upvotes   int
    Comments  int
    Shares    int
}

type Thread struct {
    ID       string
    Messages []ThreadMessage
    Summary  string
}

type Adapter interface {
    Name() string
    Poll(ctx context.Context, keywords []string) ([]RawMention, error)
    GetThread(ctx context.Context, mentionURL string) (*Thread, error)
    PostReply(ctx context.Context, threadID string, content string) error
    HealthCheck(ctx context.Context) error
}
```

---

## Hacker News Adapter

### Firebase API Polling

```go
const (
    hnBaseURL    = "https://hacker-news.firebaseio.com/v0"
    hnPollInterval = 30 * time.Second
)

func (h *HNAdapter) Poll(ctx context.Context, keywords []string) ([]RawMention, error) {
    // Strategy: Use Algolia search API for keyword matching
    // More efficient than fetching all new stories and filtering
    var mentions []RawMention

    for _, kw := range keywords {
        results, err := h.searchAlgolia(ctx, kw)
        if err != nil {
            h.logger.Warn().Err(err).Str("keyword", kw).Msg("algolia search failed")
            continue
        }
        mentions = append(mentions, results...)
    }
    return mentions, nil
}
```

### Algolia HN Search API

```go
const algoliaURL = "https://hn.algolia.com/api/v1"

func (h *HNAdapter) searchAlgolia(ctx context.Context, keyword string) ([]RawMention, error) {
    // search_by_date returns results sorted by date (most recent first)
    // numericFilters: created_at_i > lastPollTimestamp
    params := url.Values{
        "query":          {keyword},
        "tags":           {"(story,comment)"},
        "numericFilters": {fmt.Sprintf("created_at_i>%d", h.lastPollTime.Unix())},
        "hitsPerPage":    {"50"},
    }

    resp, err := h.client.Get(ctx, algoliaURL+"/search_by_date?"+params.Encode())
    if err != nil {
        return nil, fmt.Errorf("algolia search: %w", err)
    }

    var result AlgoliaResponse
    if err := json.Decode(resp.Body, &result); err != nil {
        return nil, fmt.Errorf("decode algolia: %w", err)
    }

    var mentions []RawMention
    for _, hit := range result.Hits {
        mentions = append(mentions, h.hitToMention(hit))
    }

    h.lastPollTime = time.Now()
    return mentions, nil
}
```

### Firebase API (Thread Fetching)

```go
func (h *HNAdapter) GetThread(ctx context.Context, itemID string) (*Thread, error) {
    // GET https://hacker-news.firebaseio.com/v0/item/{id}.json
    // Recursively fetch kids for full thread
    item, err := h.fetchItem(ctx, itemID)
    if err != nil {
        return nil, err
    }

    thread := &Thread{ID: itemID}
    thread.Messages = append(thread.Messages, itemToMessage(item))

    // Fetch child comments (limited to 50 to control API calls)
    for _, kidID := range item.Kids[:min(len(item.Kids), 50)] {
        kid, err := h.fetchItem(ctx, strconv.Itoa(kidID))
        if err != nil {
            continue // Skip failed fetches
        }
        thread.Messages = append(thread.Messages, itemToMessage(kid))
    }
    return thread, nil
}
```

**HN is INFORM-ONLY.** `PostReply` returns `ErrNotSupported`. HN's culture is hostile to any automated engagement.

---

## Reddit Adapter

### OAuth2 Authentication

```go
func (r *RedditAdapter) refreshToken(ctx context.Context) error {
    data := url.Values{
        "grant_type":    {"refresh_token"},
        "refresh_token": {r.refreshToken},
    }

    req, _ := http.NewRequestWithContext(ctx, "POST",
        "https://www.reddit.com/api/v1/access_token", strings.NewReader(data.Encode()))
    req.SetBasicAuth(r.clientID, r.clientSecret)
    req.Header.Set("User-Agent", "LeadEcho/1.0 (by /u/leadecho)")

    resp, err := r.httpClient.Do(req)
    // ... parse response, update r.accessToken, schedule next refresh
}
```

### Polling via Search API

```go
func (r *RedditAdapter) Poll(ctx context.Context, keywords []string) ([]RawMention, error) {
    var mentions []RawMention

    for _, kw := range keywords {
        // GET https://oauth.reddit.com/search.json?q=keyword&sort=new&t=hour&limit=25
        params := url.Values{
            "q":     {kw},
            "sort":  {"new"},
            "t":     {"hour"},
            "limit": {"25"},
        }

        resp, err := r.authenticatedGet(ctx, "/search.json?"+params.Encode())
        if err != nil {
            return nil, fmt.Errorf("reddit search: %w", err)
        }

        for _, post := range resp.Data.Children {
            mentions = append(mentions, r.postToMention(post, kw))
        }
    }
    return mentions, nil
}
```

### .json Fallback

```go
// If OAuth approval is pending, use the .json endpoint fallback
func (r *RedditAdapter) pollFallback(ctx context.Context, subreddit, keyword string) ([]RawMention, error) {
    url := fmt.Sprintf("https://www.reddit.com/r/%s/search.json?q=%s&sort=new&restrict_sr=on&t=day",
        subreddit, url.QueryEscape(keyword))

    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
    req.Header.Set("User-Agent", "LeadEcho/1.0")
    // Note: No auth needed for .json endpoints, but rate limited to ~10 req/min
}
```

### Reply Posting

```go
func (r *RedditAdapter) PostReply(ctx context.Context, thingID, content string) error {
    data := url.Values{
        "thing_id": {thingID}, // e.g., "t1_abc123" or "t3_xyz789"
        "text":     {content},
        "api_type": {"json"},
    }

    resp, err := r.authenticatedPost(ctx, "/api/comment", data)
    // ... handle rate limits, check for errors
    // Rate limit: 1 comment per 10 minutes for new accounts
}
```

---

## X/Twitter Adapter

### API v2 Search

```go
func (x *XAdapter) Poll(ctx context.Context, keywords []string) ([]RawMention, error) {
    query := strings.Join(keywords, " OR ") + " -is:retweet lang:en"

    // GET https://api.twitter.com/2/tweets/search/recent
    params := url.Values{
        "query":        {query},
        "max_results":  {"25"},
        "tweet.fields": {"created_at,public_metrics,author_id,conversation_id"},
        "user.fields":  {"username,public_metrics,verified,created_at"},
        "expansions":   {"author_id"},
        "since_id":     {x.lastSeenID},
    }
    // ... fetch and parse
}
```

### Playwright Fallback

```go
// When API costs are too high, use Playwright for monitoring
// This runs as a separate process, not in the main Go binary
// Go launches it via exec.Command and reads JSON output

func (x *XAdapter) pollPlaywright(ctx context.Context, keywords []string) ([]RawMention, error) {
    args := []string{
        "node", "scripts/x-scraper.js",
        "--keywords", strings.Join(keywords, ","),
        "--since", x.lastPollTime.Format(time.RFC3339),
    }

    cmd := exec.CommandContext(ctx, args[0], args[1:]...)
    output, err := cmd.Output()
    // ... parse JSON output into RawMention structs
}
```

---

## LinkedIn Adapter (Extension Receiver)

```go
// LinkedIn signals come from the Chrome extension, not server-side
func (l *LinkedInAdapter) Poll(ctx context.Context, keywords []string) ([]RawMention, error) {
    return nil, ErrExtensionOnly // No server-side polling
}

// Signals are received via the HTTP API endpoint
// POST /api/v1/extension/signals → handled by API handler → published to Redis
```

---

## AI Scoring Pipeline

### Relevance Scoring (Claude Haiku)

```go
func (s *Scorer) ScoreBatch(ctx context.Context, mentions []RawMention) ([]ScoredMention, error) {
    // Batch up to 10 mentions per API call for cost efficiency
    prompt := buildScoringPrompt(mentions)

    resp, err := s.claude.Messages.New(ctx, anthropic.MessageNewParams{
        Model:     anthropic.ModelClaudeHaiku4_5,
        MaxTokens: 1000,
        System:    []anthropic.TextBlockParam{{Text: scoringSystemPrompt}},
        Messages:  []anthropic.MessageParam{{Role: "user", Content: prompt}},
    })
    // ... parse structured JSON output with scores and intents
}

const scoringSystemPrompt = `You are a lead scoring engine. For each mention, provide:
1. relevance_score (1-10): How relevant is this to the user's product/keywords?
2. intent: One of: buy_signal, complaint, recommendation_ask, comparison, general
3. conversion_probability (0.0-1.0): How likely is this to convert to a customer?

Consider: specificity of the ask, urgency signals, budget mentions, comparison language.
Respond as JSON array.`
```

### Conversion Probability Formula

```go
func calculateConversionProbability(mention ScoredMention) float64 {
    score := 0.0

    // Relevance is the strongest signal (40% weight)
    score += float64(mention.RelevanceScore) / 10.0 * 0.40

    // Intent type weight (25%)
    intentWeights := map[string]float64{
        "buy_signal":        1.0,
        "recommendation_ask": 0.8,
        "comparison":        0.7,
        "complaint":         0.5,
        "general":           0.2,
    }
    score += intentWeights[mention.Intent] * 0.25

    // Platform authority (20%)
    if mention.Author.Karma > 1000 { score += 0.20 }
    else if mention.Author.Karma > 100 { score += 0.15 }
    else { score += 0.05 }

    // Recency (15%)
    age := time.Since(mention.PlatformTime)
    if age < 1*time.Hour { score += 0.15 }
    else if age < 6*time.Hour { score += 0.10 }
    else { score += 0.05 }

    return math.Min(score, 1.0)
}
```

---

## Goroutine Management

### Engine Orchestrator

```go
func (e *Engine) Start(ctx context.Context) error {
    g, ctx := errgroup.WithContext(ctx)

    for _, adapter := range e.adapters {
        adapter := adapter // capture loop var
        g.Go(func() error {
            return e.runPoller(ctx, adapter)
        })
    }

    // AI scoring worker
    g.Go(func() error {
        return e.runScorer(ctx)
    })

    return g.Wait()
}

func (e *Engine) runPoller(ctx context.Context, adapter platform.Adapter) error {
    ticker := time.NewTicker(e.pollInterval(adapter.Name()))
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            e.logger.Info().Str("platform", adapter.Name()).Msg("poller shutting down")
            return nil
        case <-ticker.C:
            if err := e.poll(ctx, adapter); err != nil {
                e.logger.Error().Err(err).Str("platform", adapter.Name()).Msg("poll failed")
                // Don't return error - keep polling
            }
        }
    }
}

func (e *Engine) poll(ctx context.Context, adapter platform.Adapter) error {
    keywords, err := e.db.GetActiveKeywords(ctx, e.workspaceIDs...)
    if err != nil {
        return fmt.Errorf("get keywords: %w", err)
    }

    mentions, err := adapter.Poll(ctx, keywordsToStrings(keywords))
    if err != nil {
        return fmt.Errorf("poll %s: %w", adapter.Name(), err)
    }

    for _, m := range mentions {
        if e.dedup.IsSeen(m.PlatformID) {
            continue
        }
        e.dedup.MarkSeen(m.PlatformID)

        // Publish to Redis for scoring and dashboard
        e.redis.Publish(ctx, "mention.detected", m)
    }
    return nil
}
```

### Graceful Shutdown

```go
func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()

    engine := signal.NewEngine(adapters, db, redis, scorer)

    if err := engine.Start(ctx); err != nil {
        log.Fatal().Err(err).Msg("engine failed")
    }
    log.Info().Msg("engine stopped gracefully")
}
```

---

## Deduplication

```go
type Deduplicator struct {
    redis *redis.Client
    ttl   time.Duration // 30 days
}

func (d *Deduplicator) IsSeen(platformID string) bool {
    key := "dedup:" + platformID
    exists, _ := d.redis.Exists(context.Background(), key).Result()
    return exists > 0
}

func (d *Deduplicator) MarkSeen(platformID string) {
    key := "dedup:" + platformID
    d.redis.Set(context.Background(), key, "1", d.ttl)
}
```

---

## Rate Limiting

```go
// Per-platform rate limiters
type RateLimitedClient struct {
    client  *http.Client
    limiter *rate.Limiter
}

func NewRateLimitedClient(reqPerSecond float64, burst int) *RateLimitedClient {
    return &RateLimitedClient{
        client:  &http.Client{Timeout: 10 * time.Second},
        limiter: rate.NewLimiter(rate.Limit(reqPerSecond), burst),
    }
}

func (r *RateLimitedClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
    if err := r.limiter.Wait(ctx); err != nil {
        return nil, fmt.Errorf("rate limit wait: %w", err)
    }
    return r.client.Do(req)
}

// Platform-specific rate limits:
// HN Algolia: 1 req/s (be respectful)
// Reddit: 60 req/min (1 req/s)
// X API v2: 60 req/15min (0.067 req/s)
```

---

## Error Handling & Resilience

### Circuit Breaker

```go
type CircuitBreaker struct {
    failures    int
    threshold   int // e.g., 5
    resetAfter  time.Duration
    lastFailure time.Time
    mu          sync.Mutex
}

func (cb *CircuitBreaker) Allow() bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    if cb.failures >= cb.threshold {
        if time.Since(cb.lastFailure) > cb.resetAfter {
            cb.failures = 0 // Reset after cooldown
            return true
        }
        return false
    }
    return true
}

func (cb *CircuitBreaker) RecordFailure() {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    cb.failures++
    cb.lastFailure = time.Now()
}

func (cb *CircuitBreaker) RecordSuccess() {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    cb.failures = 0
}
```

### Exponential Backoff with Jitter

```go
func backoff(attempt int) time.Duration {
    base := time.Second * time.Duration(math.Pow(2, float64(attempt)))
    jitter := time.Duration(rand.Int63n(int64(base / 2)))
    return base + jitter
}
```

---

## Polling Intervals

| Platform | Interval | Rationale |
|----------|----------|-----------|
| HN (Algolia) | 30s | Free API, fast results, low volume |
| Reddit | 60s | 60 req/min limit, moderate volume |
| X/Twitter | 60s | 60 req/15min limit, expensive |
| LinkedIn | N/A | Extension-pushed, not polled |

Intervals are configurable per workspace. Higher tiers get faster polling.
