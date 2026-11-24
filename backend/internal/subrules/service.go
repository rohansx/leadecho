package subrules

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Cache is the slice of the shared Redis cache used here (treg.RedisCache
// satisfies it). Rules are public, so one entry serves every workspace.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration)
}

// FetchFunc reads a subreddit's rules from Reddit
// (monitor.SubredditRulesFetcher). wsID tags a treg fallback call; the
// result is shared by every workspace.
type FetchFunc func(ctx context.Context, wsID, sub string) (Raw, error)

// ErrRefused wraps a fetch error that is about the caller or our account (a
// meter, cap or pause refusal), not the subreddit. It is never cached.
var ErrRefused = errors.New("subrules: fetch refused")

const (
	cacheTTL     = 7 * 24 * time.Hour
	failTTL      = 6 * time.Hour    // Reddit refused: try again later, not on every view
	fetchTimeout = 25 * time.Second // a treg page scrape runs a real browser
	cacheVersion = "subrules:v1:"
)

// Service fetches and caches subreddit rules. A nil Service knows nothing.
type Service struct {
	fetch FetchFunc
	cache Cache
}

// New returns a Service. cache may be nil (every Get fetches).
func New(fetch FetchFunc, cache Cache) *Service { return &Service{fetch: fetch, cache: cache} }

type cached struct {
	OK  bool `json:"ok"`
	Raw Raw  `json:"raw"`
}

var subRe = regexp.MustCompile(`^[A-Za-z0-9_]{2,21}$`)

// Get returns the subreddit's summary, or nil for a nil Service or an
// invalid name. A Reddit failure gives an Unavailable summary, never an error.
func (s *Service) Get(ctx context.Context, wsID, sub string) *Summary {
	sub = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(sub), "/"), "r/")
	if s == nil || s.fetch == nil || !subRe.MatchString(sub) {
		return nil
	}
	key := cacheVersion + strings.ToLower(sub)
	if s.cache != nil {
		if b, ok := s.cache.Get(ctx, key); ok {
			var c cached
			if json.Unmarshal(b, &c) == nil {
				return summaryOf(sub, c)
			}
		}
	}
	fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	raw, err := s.fetch(fctx, wsID, sub)
	c := cached{OK: err == nil, Raw: raw}
	// A refusal says nothing about the subreddit: the next caller may be allowed.
	if s.cache != nil && ctx.Err() == nil && !errors.Is(err, ErrRefused) {
		ttl := cacheTTL
		if err != nil {
			ttl = failTTL
		}
		if b, mErr := json.Marshal(c); mErr == nil {
			s.cache.Set(context.WithoutCancel(ctx), key, b, ttl)
		}
	}
	return summaryOf(sub, c)
}

func summaryOf(sub string, c cached) *Summary {
	var s Summary
	if c.OK {
		s = Summarize(sub, c.Raw)
	} else {
		s = Unavailable(sub)
	}
	return &s
}

var urlSubRe = regexp.MustCompile(`(?i)/r/([A-Za-z0-9_]{2,21})(?:/|$)`)

// SubredditOf finds a Reddit mention's subreddit: platform_metadata first,
// then the post URL.
func SubredditOf(postURL string, metadata []byte) string {
	var meta struct {
		Subreddit string `json:"subreddit"`
	}
	if json.Unmarshal(metadata, &meta) == nil && meta.Subreddit != "" {
		return strings.TrimPrefix(meta.Subreddit, "r/")
	}
	if u, err := url.Parse(postURL); err == nil {
		if m := urlSubRe.FindStringSubmatch(u.Path); m != nil {
			return m[1]
		}
	}
	return ""
}
