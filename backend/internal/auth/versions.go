package auth

import (
	"context"
	"sync"
	"time"

	"leadecho/internal/database"
)

// SessionVersions checks a token's session version against the user's current
// one, so a password reset revokes every outstanding session.
//
// ponytail: per-process cache with a short TTL keeps this off the hot path
// (one DB read per user per 30s). Ceiling: another API replica may accept a
// revoked token for up to versionCacheTTL; the replica that did the reset
// forgets immediately. Upgrade: publish revocations over Redis.
type SessionVersions struct {
	q     *database.Queries
	mu    sync.Mutex
	cache map[string]cachedVersion
}

type cachedVersion struct {
	v  int32
	at time.Time
}

const versionCacheTTL = 30 * time.Second

func NewSessionVersions(q *database.Queries) *SessionVersions {
	return &SessionVersions{q: q, cache: map[string]cachedVersion{}}
}

// Valid reports whether claims still match the user's session version. It
// fails closed: an unknown/deleted user or DB error rejects the session.
func (s *SessionVersions) Valid(ctx context.Context, c *Claims) bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	cv, ok := s.cache[c.UserID]
	s.mu.Unlock()
	if !ok || time.Since(cv.at) > versionCacheTTL {
		st, err := s.q.GetUserAuthState(ctx, c.UserID)
		if err != nil {
			return false
		}
		cv = cachedVersion{v: st.SessionVersion, at: time.Now()}
		s.mu.Lock()
		s.cache[c.UserID] = cv
		s.mu.Unlock()
	}
	return cv.v == c.SessionVersion
}

// Forget drops a cached version (call after bumping it).
func (s *SessionVersions) Forget(userID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.cache, userID)
	s.mu.Unlock()
}
