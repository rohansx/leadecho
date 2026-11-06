package treg

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// RedisCache is the shared response cache (one per deployment, all workspaces).
type RedisCache struct{ c *goredis.Client }

// NewRedisCache returns nil when Redis isn't available (calls go uncached).
func NewRedisCache(c *goredis.Client) Cache {
	if c == nil {
		return nil
	}
	return &RedisCache{c: c}
}

func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, bool) {
	b, err := r.c.Get(ctx, key).Bytes()
	return b, err == nil
}

// Set is best effort: a failed write only means the next identical call pays.
func (r *RedisCache) Set(ctx context.Context, key string, val []byte, ttl time.Duration) {
	_ = r.c.Set(ctx, key, val, ttl).Err()
}
