package auth

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Counter is a fixed-window counter store (Redis in prod, a map in tests).
type Counter interface {
	// Incr increments key and returns the new count and the window's remaining TTL.
	Incr(ctx context.Context, key string, window time.Duration) (int64, time.Duration, error)
	// Reset clears key (e.g. after a successful login).
	Reset(ctx context.Context, key string) error
}

type redisCounter struct{ c *goredis.Client }

func NewRedisCounter(c *goredis.Client) Counter { return redisCounter{c: c} }

func (r redisCounter) Incr(ctx context.Context, key string, window time.Duration) (int64, time.Duration, error) {
	pipe := r.c.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, window) // set TTL only on the first hit of the window
	ttl := pipe.TTL(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, 0, err
	}
	return incr.Val(), ttl.Val(), nil
}

func (r redisCounter) Reset(ctx context.Context, key string) error {
	return r.c.Del(ctx, key).Err()
}

// rule is a fixed-window limit.
type rule struct {
	limit  int64
	window time.Duration
}

var (
	ipRule          = rule{limit: 20, window: 10 * time.Minute} // per IP, per auth endpoint
	emailRule       = rule{limit: 5, window: 10 * time.Minute}  // per email: failed logins, reset/verify emails
	rateLimitPrefix = "auth_rl:"
)
