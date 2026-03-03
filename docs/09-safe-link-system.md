# LeadEcho - Safe Link & Engagement System

## Overview

The Safe Link system prevents spam detection and account bans by controlling how, when, and where replies are posted. It implements a 7-step engagement safety loop, trust scoring per platform account, UTM-based click tracking, A/B testing for reply variants, and anti-spam safeguards.

**Core principle:** A single spam report or account ban destroys months of built-up credibility. Safety is never optional.

---

## 7-Step Engagement Safety Loop

```
1. DETECT   → Mention matched by Signal Engine
2. SCORE    → AI relevance scoring (Haiku) + intent classification
3. FILTER   → Rules engine: platform rules, rate limits, trust score
4. DRAFT    → RAG pipeline generates 3 reply variants
5. REVIEW   → Human approval (or auto-approve for high-trust)
6. POST     → Human-mimicry posting with delays
7. TRACK    → UTM clicks, reply reception, outcome learning
```

### Flow Implementation

```go
type SafeEngagementPipeline struct {
    rules      *RulesEngine
    trust      *TrustScorer
    rateLimit  *EngagementRateLimiter
    utmBuilder *UTMBuilder
    poster     *SafePoster
    tracker    *OutcomeTracker
}

func (p *SafeEngagementPipeline) Process(ctx context.Context, mention ScoredMention) (*EngagementResult, error) {
    // Step 3: Rules engine check
    decision, err := p.rules.Evaluate(ctx, mention)
    if err != nil {
        return nil, fmt.Errorf("rules evaluation: %w", err)
    }
    if decision.Action == "skip" {
        return &EngagementResult{Status: "skipped", Reason: decision.Reason}, nil
    }

    // Step 3b: Rate limit check
    if !p.rateLimit.Allow(ctx, mention.Platform, mention.WorkspaceID) {
        return &EngagementResult{Status: "rate_limited"}, nil
    }

    // Steps 4-5 handled by Workflow Engine (AI draft + approval gate)
    // Step 6-7 handled below after approval
    return nil, nil
}
```

---

## Rules Engine

### Platform-Specific Rules

```go
type RulesEngine struct {
    rules []Rule
    db    *database.Queries
}

type Rule interface {
    Name() string
    Evaluate(ctx context.Context, mention ScoredMention) (*RuleResult, error)
}

type RuleResult struct {
    Passed  bool
    Action  string // "allow", "skip", "flag"
    Reason  string
}
```

### Built-in Rules

```go
// Rule: Don't engage with very old posts
type MaxAgeRule struct {
    maxAge time.Duration // Default: 24h for Reddit, 6h for Twitter
}

func (r *MaxAgeRule) Evaluate(ctx context.Context, m ScoredMention) (*RuleResult, error) {
    age := time.Since(m.PlatformTime)
    if age > r.maxAge {
        return &RuleResult{Passed: false, Action: "skip", Reason: fmt.Sprintf("post too old: %s", age)}, nil
    }
    return &RuleResult{Passed: true, Action: "allow"}, nil
}

// Rule: Don't reply to the same author more than once per week
type AuthorCooldownRule struct {
    db       *database.Queries
    cooldown time.Duration // 7 days
}

func (r *AuthorCooldownRule) Evaluate(ctx context.Context, m ScoredMention) (*RuleResult, error) {
    lastReply, err := r.db.GetLastReplyToAuthor(ctx, m.WorkspaceID, m.Author.Username, m.Platform)
    if err != nil {
        return &RuleResult{Passed: true, Action: "allow"}, nil // Allow if no history
    }
    if time.Since(lastReply.PostedAt) < r.cooldown {
        return &RuleResult{Passed: false, Action: "skip", Reason: "author cooldown active"}, nil
    }
    return &RuleResult{Passed: true, Action: "allow"}, nil
}

// Rule: Don't reply to threads where we already replied
type ThreadDeduplicationRule struct {
    db *database.Queries
}

func (r *ThreadDeduplicationRule) Evaluate(ctx context.Context, m ScoredMention) (*RuleResult, error) {
    exists, _ := r.db.HasReplyInThread(ctx, m.WorkspaceID, m.ThreadID, m.Platform)
    if exists {
        return &RuleResult{Passed: false, Action: "skip", Reason: "already replied in thread"}, nil
    }
    return &RuleResult{Passed: true, Action: "allow"}, nil
}

// Rule: Don't engage in hostile threads
type SentimentGuardRule struct{}

func (r *SentimentGuardRule) Evaluate(ctx context.Context, m ScoredMention) (*RuleResult, error) {
    if m.ThreadAnalysis != nil && m.ThreadAnalysis.Sentiment == "hostile" {
        return &RuleResult{Passed: false, Action: "skip", Reason: "hostile thread detected"}, nil
    }
    return &RuleResult{Passed: true, Action: "allow"}, nil
}

// Rule: Don't reply if our product is already mentioned negatively
type ProductMentionGuardRule struct{}

func (r *ProductMentionGuardRule) Evaluate(ctx context.Context, m ScoredMention) (*RuleResult, error) {
    if m.ThreadAnalysis != nil && m.ThreadAnalysis.OurProductMentioned {
        return &RuleResult{Passed: false, Action: "flag", Reason: "product already mentioned in thread"}, nil
    }
    return &RuleResult{Passed: true, Action: "allow"}, nil
}

// Rule: Subreddit/community blocklist
type CommunityBlocklistRule struct {
    db *database.Queries
}

func (r *CommunityBlocklistRule) Evaluate(ctx context.Context, m ScoredMention) (*RuleResult, error) {
    blocked, _ := r.db.IsCommunityBlocked(ctx, m.WorkspaceID, m.Platform, m.Subreddit)
    if blocked {
        return &RuleResult{Passed: false, Action: "skip", Reason: "community is blocklisted"}, nil
    }
    return &RuleResult{Passed: true, Action: "allow"}, nil
}
```

### Rules Evaluation

```go
func (e *RulesEngine) Evaluate(ctx context.Context, mention ScoredMention) (*EngagementDecision, error) {
    for _, rule := range e.rules {
        result, err := rule.Evaluate(ctx, mention)
        if err != nil {
            // Log error but don't block - fail open for evaluation errors
            continue
        }
        if !result.Passed {
            return &EngagementDecision{
                Action: result.Action,
                Reason: fmt.Sprintf("rule '%s': %s", rule.Name(), result.Reason),
            }, nil
        }
    }
    return &EngagementDecision{Action: "allow"}, nil
}
```

---

## Trust Scoring

### Per-Account Trust Score

Each platform account used for posting has a trust score that affects rate limits and auto-approval thresholds.

```go
type TrustScore struct {
    AccountID       string
    Platform        string
    Score           float64  // 0.0 - 1.0
    TotalReplies    int
    SuccessfulPosts int      // Not removed/flagged
    RemovedPosts    int      // Detected as removed
    UpvoteRatio     float64  // Average upvotes/downvotes
    AccountAge      int      // Days since account creation
    LastUpdated     time.Time
}

func (t *TrustScorer) CalculateScore(account PlatformAccount) float64 {
    score := 0.0

    // Account age (25% weight)
    ageDays := time.Since(account.CreatedAt).Hours() / 24
    if ageDays > 365 {
        score += 0.25
    } else if ageDays > 90 {
        score += 0.20
    } else if ageDays > 30 {
        score += 0.10
    } else {
        score += 0.02 // New accounts are very risky
    }

    // Post success rate (30% weight)
    if account.TotalReplies > 0 {
        successRate := float64(account.SuccessfulPosts) / float64(account.TotalReplies)
        score += successRate * 0.30
    }

    // Removal penalty (20% weight, inverted)
    if account.TotalReplies > 0 {
        removalRate := float64(account.RemovedPosts) / float64(account.TotalReplies)
        score += (1.0 - removalRate) * 0.20
    } else {
        score += 0.10 // Neutral for no history
    }

    // Karma/reputation (15% weight)
    if account.Karma > 5000 {
        score += 0.15
    } else if account.Karma > 1000 {
        score += 0.12
    } else if account.Karma > 100 {
        score += 0.08
    } else {
        score += 0.03
    }

    // Engagement quality (10% weight)
    if account.UpvoteRatio > 0.8 {
        score += 0.10
    } else if account.UpvoteRatio > 0.5 {
        score += 0.06
    } else {
        score += 0.02
    }

    return math.Min(score, 1.0)
}
```

### Trust-Based Behavior

| Trust Score | Rate Limit | Auto-Approve | Posting Delay |
|-------------|-----------|--------------|---------------|
| 0.0 - 0.3 | 2 replies/day | Never | 15-30 min |
| 0.3 - 0.5 | 5 replies/day | Never | 5-15 min |
| 0.5 - 0.7 | 10 replies/day | Score >= 9.0 | 2-8 min |
| 0.7 - 0.9 | 20 replies/day | Score >= 7.0 | 1-5 min |
| 0.9 - 1.0 | 30 replies/day | Score >= 6.0 | 30s-3 min |

---

## Engagement Rate Limiter

```go
type EngagementRateLimiter struct {
    redis *redis.Client
    db    *database.Queries
}

func (r *EngagementRateLimiter) Allow(ctx context.Context, platform, workspaceID string) bool {
    // Get trust score for the workspace's platform account
    account, err := r.db.GetPlatformAccount(ctx, workspaceID, platform)
    if err != nil {
        return false
    }

    trustScore := r.calculateTrust(account)
    dailyLimit := r.getDailyLimit(trustScore)

    // Redis counter: replies posted today
    key := fmt.Sprintf("ratelimit:reply:%s:%s:%s",
        workspaceID, platform, time.Now().Format("2006-01-02"))

    count, _ := r.redis.Incr(ctx, key).Result()
    if count == 1 {
        r.redis.Expire(ctx, key, 25*time.Hour) // Expire after the day
    }

    if count > int64(dailyLimit) {
        return false
    }

    return true
}

func (r *EngagementRateLimiter) getDailyLimit(trustScore float64) int {
    switch {
    case trustScore >= 0.9:
        return 30
    case trustScore >= 0.7:
        return 20
    case trustScore >= 0.5:
        return 10
    case trustScore >= 0.3:
        return 5
    default:
        return 2
    }
}
```

### Per-Platform Posting Intervals

```go
func (r *EngagementRateLimiter) GetPostingDelay(trustScore float64) time.Duration {
    switch {
    case trustScore >= 0.9:
        return randomBetween(30*time.Second, 3*time.Minute)
    case trustScore >= 0.7:
        return randomBetween(1*time.Minute, 5*time.Minute)
    case trustScore >= 0.5:
        return randomBetween(2*time.Minute, 8*time.Minute)
    case trustScore >= 0.3:
        return randomBetween(5*time.Minute, 15*time.Minute)
    default:
        return randomBetween(15*time.Minute, 30*time.Minute)
    }
}
```

---

## UTM Link Tracking

### UTM Builder

```go
type UTMBuilder struct {
    baseRedirectURL string // e.g., "https://r.leadecho.app"
    db              *database.Queries
}

type UTMConfig struct {
    DestinationURL string
    Source         string // "reddit", "hackernews", "twitter", "linkedin"
    Medium         string // "social_reply"
    Campaign       string // workspace-defined or auto-generated
    Term           string // keyword that triggered the mention
    Content        string // reply variant: "value_only", "technical", "soft_sell"
}

func (b *UTMBuilder) BuildTrackedLink(ctx context.Context, config UTMConfig, replyID string) (string, error) {
    // Generate short code
    code := generateShortCode(8) // e.g., "a3f8k2m9"

    // Store in database
    link := &UTMLink{
        ID:             ulid.Make().String(),
        Code:           code,
        ReplyID:        replyID,
        DestinationURL: config.DestinationURL,
        UTMSource:      config.Source,
        UTMMedium:      config.Medium,
        UTMCampaign:    config.Campaign,
        UTMTerm:        config.Term,
        UTMContent:     config.Content,
        CreatedAt:      time.Now(),
    }

    if err := b.db.CreateUTMLink(ctx, link); err != nil {
        return "", fmt.Errorf("create utm link: %w", err)
    }

    return fmt.Sprintf("%s/%s", b.baseRedirectURL, code), nil
}

func generateShortCode(length int) string {
    const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
    b := make([]byte, length)
    for i := range b {
        n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
        b[i] = charset[n.Int64()]
    }
    return string(b)
}
```

### UTM Redirect Handler

```go
// GET /api/v1/webhooks/utm/:code
// Public endpoint - no auth required
func (h *Handler) HandleUTMRedirect(w http.ResponseWriter, r *http.Request) {
    code := chi.URLParam(r, "code")

    link, err := h.db.GetUTMLinkByCode(r.Context(), code)
    if err != nil {
        http.Redirect(w, r, h.fallbackURL, http.StatusFound)
        return
    }

    // Record click event (async)
    go func() {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        event := &UTMEvent{
            ID:        ulid.Make().String(),
            LinkID:    link.ID,
            EventType: "click",
            IP:        hashIP(r.RemoteAddr), // Hash for privacy
            UserAgent: r.UserAgent(),
            Referrer:  r.Referer(),
            CreatedAt: time.Now(),
        }
        h.db.CreateUTMEvent(ctx, event)

        // Update click count
        h.db.IncrementLinkClicks(ctx, link.ID)

        // Publish event for real-time dashboard
        h.redis.Publish(ctx, "utm.click", event)
    }()

    // Build full destination URL with UTM params
    destURL := appendUTMParams(link.DestinationURL, link)
    http.Redirect(w, r, destURL, http.StatusFound)
}

func appendUTMParams(baseURL string, link *UTMLink) string {
    u, err := url.Parse(baseURL)
    if err != nil {
        return baseURL
    }

    q := u.Query()
    q.Set("utm_source", link.UTMSource)
    q.Set("utm_medium", link.UTMMedium)
    q.Set("utm_campaign", link.UTMCampaign)
    if link.UTMTerm != "" {
        q.Set("utm_term", link.UTMTerm)
    }
    if link.UTMContent != "" {
        q.Set("utm_content", link.UTMContent)
    }
    u.RawQuery = q.Encode()
    return u.String()
}
```

### Link Injection in Replies

```go
func (g *Generator) InjectTrackedLink(ctx context.Context, reply string, config UTMConfig, replyID string) (string, error) {
    // Only inject if reply already contains a natural link placeholder
    // The AI is instructed to include [product_link] placeholder when appropriate
    if !strings.Contains(reply, "[product_link]") {
        return reply, nil // No link injection - keep it as pure value reply
    }

    trackedURL, err := g.utmBuilder.BuildTrackedLink(ctx, config, replyID)
    if err != nil {
        return reply, nil // Fail open - return reply without link
    }

    return strings.Replace(reply, "[product_link]", trackedURL, 1), nil
}
```

---

## A/B Testing Framework

### Variant Selection

```go
type ABTestManager struct {
    db *database.Queries
}

type ABTest struct {
    ID          string
    WorkspaceID string
    Name        string
    Variants    []string           // ["value_only", "technical", "soft_sell"]
    Weights     map[string]float64 // {"value_only": 0.5, "technical": 0.3, "soft_sell": 0.2}
    IsActive    bool
    Results     map[string]VariantMetrics
}

type VariantMetrics struct {
    Impressions int
    Clicks      int
    Conversions int
    CTR         float64
    CVR         float64
    Removals    int
}

func (m *ABTestManager) SelectVariant(ctx context.Context, workspaceID string, replyVariants []ReplyVariant) (*ReplyVariant, error) {
    test, err := m.db.GetActiveABTest(ctx, workspaceID)
    if err != nil || test == nil {
        // No active test - use AI recommended variant
        return &replyVariants[0], nil
    }

    // Weighted random selection
    selected := weightedRandom(test.Variants, test.Weights)

    for _, v := range replyVariants {
        if v.Type == selected {
            return &v, nil
        }
    }

    return &replyVariants[0], nil // Fallback
}

func weightedRandom(variants []string, weights map[string]float64) string {
    total := 0.0
    for _, w := range weights {
        total += w
    }

    r := rand.Float64() * total
    cumulative := 0.0
    for _, v := range variants {
        cumulative += weights[v]
        if r <= cumulative {
            return v
        }
    }
    return variants[0]
}
```

### Tracking Outcomes

```go
func (m *ABTestManager) RecordOutcome(ctx context.Context, replyID string, eventType string) error {
    reply, err := m.db.GetReply(ctx, replyID)
    if err != nil {
        return err
    }

    switch eventType {
    case "posted":
        return m.db.IncrementVariantImpressions(ctx, reply.WorkspaceID, reply.Variant)
    case "clicked":
        return m.db.IncrementVariantClicks(ctx, reply.WorkspaceID, reply.Variant)
    case "converted":
        return m.db.IncrementVariantConversions(ctx, reply.WorkspaceID, reply.Variant)
    case "removed":
        return m.db.IncrementVariantRemovals(ctx, reply.WorkspaceID, reply.Variant)
    }
    return nil
}
```

---

## Anti-Spam Safeguards

### Content Safety Checks

```go
type ContentSafetyChecker struct{}

func (c *ContentSafetyChecker) Check(reply string) []SafetyIssue {
    var issues []SafetyIssue

    // No direct product links in value_only variants
    if containsURL(reply) {
        issues = append(issues, SafetyIssue{
            Severity: "warning",
            Rule:     "no_raw_urls",
            Message:  "Reply contains raw URL - consider using tracked link or removing",
        })
    }

    // Check for overly promotional language
    promoPatterns := []string{
        "check out our", "sign up", "try our", "get started",
        "limited time", "discount", "promo code", "free trial",
    }
    for _, pattern := range promoPatterns {
        if strings.Contains(strings.ToLower(reply), pattern) {
            issues = append(issues, SafetyIssue{
                Severity: "warning",
                Rule:     "promotional_language",
                Message:  fmt.Sprintf("Promotional language detected: '%s'", pattern),
            })
        }
    }

    // Check reply length (too short = low value, too long = suspicious)
    words := len(strings.Fields(reply))
    if words < 15 {
        issues = append(issues, SafetyIssue{
            Severity: "info",
            Rule:     "too_short",
            Message:  "Reply may be too short to provide genuine value",
        })
    }
    if words > 300 {
        issues = append(issues, SafetyIssue{
            Severity: "warning",
            Rule:     "too_long",
            Message:  "Very long replies may look automated",
        })
    }

    return issues
}
```

### Global Safety Limits

```go
// Hard limits that cannot be overridden by workspace settings
const (
    MaxRepliesPerAccountPerDay    = 50
    MaxRepliesPerSubredditPerDay  = 5
    MaxRepliesPerThreadPerDay     = 1
    MinTimeBetweenReplies         = 30 * time.Second
    MaxConsecutiveSamePlatform    = 3  // Vary platforms
    MaxRepliesPerAuthorPerWeek    = 1
)

type GlobalSafetyLimiter struct {
    redis *redis.Client
}

func (g *GlobalSafetyLimiter) CheckGlobalLimits(ctx context.Context, req PostReplyRequest) error {
    checks := []struct {
        key    string
        limit  int64
        ttl    time.Duration
        errMsg string
    }{
        {
            key:    fmt.Sprintf("safety:account:%s:day:%s", req.AccountID, today()),
            limit:  MaxRepliesPerAccountPerDay,
            ttl:    25 * time.Hour,
            errMsg: "daily account reply limit reached",
        },
        {
            key:    fmt.Sprintf("safety:subreddit:%s:%s:day:%s", req.AccountID, req.Subreddit, today()),
            limit:  MaxRepliesPerSubredditPerDay,
            ttl:    25 * time.Hour,
            errMsg: "daily subreddit reply limit reached",
        },
        {
            key:    fmt.Sprintf("safety:thread:%s:%s", req.AccountID, req.ThreadID),
            limit:  MaxRepliesPerThreadPerDay,
            ttl:    25 * time.Hour,
            errMsg: "already replied in this thread today",
        },
    }

    for _, check := range checks {
        count, _ := g.redis.Incr(ctx, check.key).Result()
        if count == 1 {
            g.redis.Expire(ctx, check.key, check.ttl)
        }
        if count > check.limit {
            g.redis.Decr(ctx, check.key) // Undo increment
            return fmt.Errorf(check.errMsg)
        }
    }

    // Minimum time between replies
    lastKey := fmt.Sprintf("safety:last_reply:%s", req.AccountID)
    lastReply, err := g.redis.Get(ctx, lastKey).Time()
    if err == nil && time.Since(lastReply) < MinTimeBetweenReplies {
        return fmt.Errorf("too soon since last reply: wait %s", MinTimeBetweenReplies-time.Since(lastReply))
    }
    g.redis.Set(ctx, lastKey, time.Now(), 24*time.Hour)

    return nil
}
```

---

## Outcome Learning System

### Track Post Reception

```go
type OutcomeTracker struct {
    db      *database.Queries
    redis   *redis.Client
    adapters map[string]platform.Adapter
}

// Run every hour to check on posted replies
func (t *OutcomeTracker) CheckOutcomes(ctx context.Context) error {
    // Get replies posted in the last 24-48 hours
    replies, err := t.db.GetRecentPostedReplies(ctx, 48*time.Hour)
    if err != nil {
        return err
    }

    for _, reply := range replies {
        outcome, err := t.checkReplyOutcome(ctx, reply)
        if err != nil {
            continue
        }

        // Update reply record
        t.db.UpdateReplyOutcome(ctx, reply.ID, outcome)

        // Update trust score
        if outcome.WasRemoved {
            t.adjustTrustScore(ctx, reply.PlatformAccountID, -0.05)
        } else if outcome.Upvotes > 3 {
            t.adjustTrustScore(ctx, reply.PlatformAccountID, +0.02)
        }

        // Promote successful replies as RAG exemplars
        if outcome.Upvotes > 5 && !outcome.WasRemoved {
            t.promoteToExemplar(ctx, reply)
        }
    }
    return nil
}

func (t *OutcomeTracker) checkReplyOutcome(ctx context.Context, reply Reply) (*ReplyOutcome, error) {
    adapter := t.adapters[reply.Platform]

    // Check if reply still exists / its metrics
    thread, err := adapter.GetThread(ctx, reply.ThreadPlatformID)
    if err != nil {
        return &ReplyOutcome{CheckFailed: true}, nil
    }

    // Find our reply in the thread
    for _, msg := range thread.Messages {
        if msg.PlatformID == reply.PlatformPostID {
            return &ReplyOutcome{
                Upvotes:     msg.Upvotes,
                Downvotes:   msg.Downvotes,
                WasRemoved:  false,
                Replies:     msg.ReplyCount,
                CheckedAt:   time.Now(),
            }, nil
        }
    }

    // Reply not found - likely removed
    return &ReplyOutcome{WasRemoved: true, CheckedAt: time.Now()}, nil
}
```

---

## Safe Posting with Human Mimicry

### Safe Poster

```go
type SafePoster struct {
    adapters       map[string]platform.Adapter
    safetyLimiter  *GlobalSafetyLimiter
    contentChecker *ContentSafetyChecker
    trustScorer    *TrustScorer
}

func (p *SafePoster) Post(ctx context.Context, req PostReplyRequest) error {
    // 1. Global safety limits
    if err := p.safetyLimiter.CheckGlobalLimits(ctx, req); err != nil {
        return fmt.Errorf("safety limit: %w", err)
    }

    // 2. Content safety check
    issues := p.contentChecker.Check(req.Content)
    for _, issue := range issues {
        if issue.Severity == "error" {
            return fmt.Errorf("content safety: %s", issue.Message)
        }
    }

    // 3. Calculate posting delay based on trust
    trust := p.trustScorer.GetScore(ctx, req.AccountID)
    delay := p.getPostingDelay(trust)

    // 4. Wait for delay
    select {
    case <-time.After(delay):
    case <-ctx.Done():
        return ctx.Err()
    }

    // 5. Post via platform adapter
    adapter := p.adapters[req.Platform]
    if err := adapter.PostReply(ctx, req.ThreadID, req.Content); err != nil {
        return fmt.Errorf("post reply: %w", err)
    }

    return nil
}
```

### Chrome Extension Posting (LinkedIn)

For LinkedIn, posting goes through the Chrome extension's human-mimicry engine:

```
Dashboard → API (queue reply) → Extension polls queue → Content script navigates to post
→ Human-mimicry types reply → Extension confirms posting → API updates status
```

See [07-chrome-extension.md](07-chrome-extension.md) for the human-mimicry typing simulation implementation.

---

## Monitoring & Alerting

### Safety Metrics Dashboard

```sql
-- Daily safety metrics
SELECT
    date_trunc('day', created_at) AS day,
    COUNT(*) FILTER (WHERE status = 'posted') AS replies_posted,
    COUNT(*) FILTER (WHERE status = 'skipped') AS replies_skipped,
    COUNT(*) FILTER (WHERE outcome_removed = true) AS replies_removed,
    AVG(outcome_upvotes) FILTER (WHERE outcome_upvotes IS NOT NULL) AS avg_upvotes,
    COUNT(*) FILTER (WHERE outcome_removed = true)::float /
        NULLIF(COUNT(*) FILTER (WHERE status = 'posted'), 0) AS removal_rate
FROM replies
WHERE workspace_id = @workspace_id
    AND created_at > now() - interval '30 days'
GROUP BY 1
ORDER BY 1 DESC;
```

### Alert Thresholds

| Metric | Warning | Critical | Action |
|--------|---------|----------|--------|
| Removal rate | > 5% | > 15% | Pause posting, reduce trust |
| Reply downvotes avg | < -2 | < -5 | Flag for review |
| Account karma drop | > 10%/week | > 25%/week | Pause account |
| Safety limit hits | > 50%/day | > 80%/day | Reduce rate limits |
