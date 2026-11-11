// Package treg is LeadEcho's single gateway to treg.to (hosted data tools:
// X/LinkedIn search, enrichment, ...). Every call goes through Client.Call so
// customer tagging, cost ceilings, idempotency and our ledger can't drift
// between call sites. See docs/treg-integration.md for the contract.
package treg

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

const (
	DefaultBaseURL = "https://treg.to"
	maxAttempts    = 3
	pauseOnAlert   = 5 * time.Minute
	maxBodyBytes   = 8 << 20
)

// Operator-only errors: they describe OUR treg account (balance, team cap)
// and must never reach an end user. Callers degrade silently.
var (
	ErrInsufficientBalance = errors.New("treg: insufficient balance")
	ErrPlatformDailyCap    = errors.New("treg: platform daily cap reached")
	ErrPaused              = errors.New("treg: paused after an account-level refusal")
)

// ErrCustomerDailyCap: this one workspace reached its daily spend cap at treg.
// Other workspaces are unaffected; it lifts at the next UTC midnight.
var ErrCustomerDailyCap = errors.New("treg: workspace daily spend cap reached")

var (
	ErrNotConfigured = errors.New("treg: not configured")
	ErrNoOrg         = errors.New("treg: TREG_ORG_ID not set")
)

// IsOperatorError reports whether err is an account-level treg problem that
// should alert operators rather than be shown to a user.
func IsOperatorError(err error) bool {
	return errors.Is(err, ErrInsufficientBalance) || errors.Is(err, ErrPlatformDailyCap) || errors.Is(err, ErrPaused)
}

// APIError is any other non-2xx answer (treg's own refusal or the provider's,
// relayed verbatim). Code is the body's "error" field, or "http_<status>".
type APIError struct {
	Status int
	Code   string
	Treg   bool // X-Treg-Error: 1 — treg refused, not the provider
}

func (e *APIError) Error() string { return fmt.Sprintf("treg: %d %s", e.Status, e.Code) }

// Ledger stores one row per treg response; *database.Queries satisfies it.
type Ledger interface {
	RecordTregCall(ctx context.Context, arg database.RecordTregCallParams) error
}

type Config struct {
	Token        string
	OrgID        string
	BaseURL      string
	MaxCostMicro int64 // per-call ceiling sent as X-Treg-Route-Max-Cost; 0 = none
}

type Client struct {
	cfg         Config
	http        *http.Client
	ledger      Ledger
	logger      zerolog.Logger
	backoff     func(attempt int) time.Duration
	pausedUntil atomic.Int64 // unix nano
	capped      sync.Map     // workspace id -> time.Time its daily spend cap lifts
	meter       Meter        // nil: calls are not metered
	cache       Cache        // nil: no response cache
	cacheTTL    time.Duration
}

// New returns nil when no token is configured; a nil *Client is valid and
// reports Enabled() == false, so callers need no extra wiring checks.
func New(cfg Config, ledger Ledger, logger zerolog.Logger) *Client {
	if cfg.Token == "" {
		return nil
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Client{
		cfg:     cfg,
		http:    &http.Client{Timeout: 60 * time.Second},
		ledger:  ledger,
		logger:  logger,
		backoff: func(a int) time.Duration { return time.Duration(a*a) * time.Second },
	}
}

func (c *Client) Enabled() bool { return c != nil }

type Request struct {
	WorkspaceID    string
	Tool           string // catalog endpoint id, e.g. treg.x.search.posts
	Body           any    // JSON body (POST)
	IdempotencyKey string // optional; generated once per Call and reused on retries
}

type Response struct {
	Status    int
	Body      []byte
	CallID    string
	CostMicro int64
	ServedBy  string
}

var (
	tagValueRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	toolRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

func validTag(v string) bool { return tagValueRe.MatchString(v) && !strings.Contains(v, "@") }

// Call POSTs to /call/<tool>, retrying 5xx/timeouts/409 with the same
// Idempotency-Key so a lost answer is replayed rather than paid twice.
// Meter charges a workspace's prepaid balance for paid calls (billing.Credits).
type Meter interface {
	Spend(ctx context.Context, workspaceID string, cost int, reason string) error
	Refund(ctx context.Context, workspaceID string, n int, reason string)
}

// SetMeter makes every call cost the workspace one credit, refunded when the
// call fails or treg doesn't bill it (a miss).
func (c *Client) SetMeter(m Meter) {
	if c != nil {
		c.meter = m
	}
}

// Cache stores successful responses so repeated searches cost nothing.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration)
}

// SetCache serves identical calls (same tool and body, any workspace: the
// data is public) from cache for ttl.
func (c *Client) SetCache(cache Cache, ttl time.Duration) {
	if c != nil {
		c.cache, c.cacheTTL = cache, ttl
	}
}

func cacheKey(req Request) (string, bool) {
	body, err := json.Marshal(req.Body) // map keys marshal sorted: stable
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(append([]byte(req.Tool+"\x00"), body...))
	return "treg:v1:" + hex.EncodeToString(sum[:]), true
}

// Call runs one catalog tool: served from cache when possible, otherwise
// metered (when a Meter is set) and cached on success.
func (c *Client) Call(ctx context.Context, req Request) (*Response, error) {
	var key string
	if c != nil && c.cache != nil && c.cacheTTL > 0 {
		if k, ok := cacheKey(req); ok {
			key = k
			if body, hit := c.cache.Get(ctx, key); hit {
				return &Response{Status: http.StatusOK, Body: body, ServedBy: "cache"}, nil
			}
		}
	}
	resp, err := c.meteredCall(ctx, req)
	if err == nil && key != "" && resp != nil && resp.Status >= 200 && resp.Status < 300 {
		c.cache.Set(ctx, key, resp.Body, c.cacheTTL)
	}
	return resp, err
}

func (c *Client) meteredCall(ctx context.Context, req Request) (*Response, error) {
	if c == nil || c.meter == nil {
		return c.call(ctx, req)
	}
	if err := c.meter.Spend(ctx, req.WorkspaceID, 1, "search"); err != nil {
		return nil, err
	}
	resp, err := c.call(ctx, req)
	if err != nil || resp == nil || resp.CostMicro == 0 {
		c.meter.Refund(ctx, req.WorkspaceID, 1, "search")
	}
	return resp, err
}

func (c *Client) call(ctx context.Context, req Request) (*Response, error) {
	if !c.Enabled() {
		return nil, ErrNotConfigured
	}
	if !validTag(req.WorkspaceID) {
		return nil, fmt.Errorf("treg: invalid workspace id %q", req.WorkspaceID)
	}
	if !toolRe.MatchString(req.Tool) {
		return nil, fmt.Errorf("treg: invalid tool id %q", req.Tool)
	}
	if time.Now().UnixNano() < c.pausedUntil.Load() {
		return nil, ErrPaused
	}
	if until, ok := c.capped.Load(req.WorkspaceID); ok {
		if time.Now().Before(until.(time.Time)) {
			return nil, ErrCustomerDailyCap // every call would be refused: don't make them
		}
		c.capped.Delete(req.WorkspaceID)
	}
	payload, err := json.Marshal(req.Body)
	if err != nil {
		return nil, fmt.Errorf("treg: encode body: %w", err)
	}
	key := req.IdempotencyKey
	if key == "" {
		key = newKey()
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoff(attempt)):
			}
		}
		resp, retry, err := c.attempt(ctx, req, payload, key)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retry || ctx.Err() != nil {
			break
		}
	}
	return nil, lastErr
}

func (c *Client) attempt(ctx context.Context, req Request, payload []byte, key string) (*Response, bool, error) {
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/call/"+req.Tool, bytes.NewReader(payload))
	if err != nil {
		return nil, false, fmt.Errorf("treg: build request: %w", err)
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("X-Treg-Token", c.cfg.Token)
	hr.Header.Set("X-Treg-Meta", "customer="+req.WorkspaceID)
	hr.Header.Set("Idempotency-Key", key)
	if c.cfg.MaxCostMicro > 0 {
		hr.Header.Set("X-Treg-Route-Max-Cost", microToUSD(c.cfg.MaxCostMicro))
	}

	res, err := c.http.Do(hr)
	if err != nil {
		// Timeout / dropped connection: the provider may have answered and
		// been paid, so retry with the same key (replay is free).
		return nil, true, fmt.Errorf("treg: %s: %w", req.Tool, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBodyBytes))
	if err != nil {
		return nil, true, fmt.Errorf("treg: read %s: %w", req.Tool, err)
	}

	out := &Response{
		Status:    res.StatusCode,
		Body:      body,
		CallID:    res.Header.Get("X-Treg-Call-Id"),
		CostMicro: parseMicro(res.Header.Get("X-Treg-Cost-Micro")),
		ServedBy:  res.Header.Get("X-Treg-Served-By"),
	}
	callErr, retry := c.classify(res, body)
	if errors.Is(callErr, ErrCustomerDailyCap) {
		until := nextUTCMidnight(time.Now())
		if _, already := c.capped.LoadOrStore(req.WorkspaceID, until); !already {
			// Once per workspace per day; raise its budget at treg (or
			// TREG_CUSTOMER_DAILY_CAP_MICRO) if this is a paying customer.
			c.logger.Warn().Str("alert", "treg").Str("workspace_id", req.WorkspaceID).Time("until", until).
				Msg("treg: workspace reached its daily spend cap; paid data is off for it until the cap resets")
		}
	}
	c.record(ctx, req, out, callErr)
	if callErr != nil {
		return nil, retry, callErr
	}
	return out, false, nil
}

// classify maps a response to an error (nil on 2xx) and whether to retry.
func (c *Client) classify(res *http.Response, body []byte) (error, bool) {
	if res.StatusCode < 300 {
		return nil, false
	}
	code := errorCode(body)
	switch {
	case code == "insufficient_balance":
		c.alert(ErrInsufficientBalance)
		return ErrInsufficientBalance, false
	case code == "platform_daily_cap_reached":
		c.alert(ErrPlatformDailyCap)
		return ErrPlatformDailyCap, false
	case code == "tag_spend_cap_reached": // one workspace's budget, not the account
		return ErrCustomerDailyCap, false
	}
	isTreg := res.Header.Get("X-Treg-Error") == "1"
	if code == "" {
		code = "http_" + strconv.Itoa(res.StatusCode)
	}
	retry := (res.StatusCode >= 500 && code != "route_failed") ||
		(res.StatusCode == http.StatusConflict && isTreg) // original still running
	return &APIError{Status: res.StatusCode, Code: code, Treg: isTreg}, retry
}

func (c *Client) alert(err error) {
	c.pausedUntil.Store(time.Now().Add(pauseOnAlert).UnixNano())
	// alert=treg is the hook for log-based alerting; never forward to users.
	c.logger.Error().Err(err).Str("alert", "treg").Dur("pause", pauseOnAlert).
		Msg("treg: account-level refusal — top up or raise the cap; fetchers fall back meanwhile")
}

func (c *Client) record(ctx context.Context, req Request, out *Response, callErr error) {
	if c.ledger == nil {
		return
	}
	status := "ok"
	var apiErr *APIError
	switch {
	case errors.Is(callErr, ErrInsufficientBalance):
		status = "insufficient_balance"
	case errors.Is(callErr, ErrPlatformDailyCap):
		status = "platform_daily_cap_reached"
	case errors.As(callErr, &apiErr):
		status = apiErr.Code
	}
	// Detached ctx: a paid call must be recorded even if the caller gave up.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := c.ledger.RecordTregCall(rctx, database.RecordTregCallParams{
		WorkspaceID: req.WorkspaceID, Tool: req.Tool, CallID: out.CallID, CostMicro: out.CostMicro, Status: status,
	}); err != nil {
		c.logger.Error().Err(err).Str("call_id", out.CallID).Int64("cost_micro", out.CostMicro).Msg("treg: failed to record call in ledger")
	}
}

// errorCode reads the refusal code from {"error":"..."} or, for budget
// refusals, {"detail":{"error":"..."}}.
func errorCode(body []byte) string {
	var e struct {
		Error  any `json:"error"`
		Detail struct {
			Error string `json:"error"`
		} `json:"detail"`
	}
	if json.Unmarshal(body, &e) != nil {
		return ""
	}
	if s, _ := e.Error.(string); s != "" {
		return s
	}
	return e.Detail.Error
}

// nextUTCMidnight is when treg's per-day caps reset.
func nextUTCMidnight(now time.Time) time.Time {
	y, m, d := now.UTC().Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC)
}

func parseMicro(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

func microToUSD(micro int64) string {
	return strconv.FormatFloat(float64(micro)/1e6, 'f', -1, 64)
}

func newKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "le-" + hex.EncodeToString(b)
}
