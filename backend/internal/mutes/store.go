package mutes

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// cacheTTL bounds how stale a workspace's mute list may be on the insert path.
// ponytail: per-process TTL cache; a mute added in another process (API vs
// worker) takes up to cacheTTL to apply. Add pub/sub invalidation if that matters.
const cacheTTL = time.Minute

type cached struct {
	list []Mute
	at   time.Time
}

// Store reads and writes workspace mutes. Safe for concurrent use. It is the
// entry point for the API, the monitor insert path and the MCP 'mute' tool.
type Store struct {
	q     database.Querier
	now   func() time.Time
	mu    sync.Mutex
	cache map[string]cached
}

func NewStore(q database.Querier) *Store {
	return &Store{q: q, now: time.Now, cache: map[string]cached{}}
}

// Add mutes value of kind in workspace ws (idempotent).
func (s *Store) Add(ctx context.Context, ws, kind, value string) error {
	_, err := s.AddBy(ctx, ws, "", kind, value)
	return err
}

// AddBy is Add with the acting user recorded; it returns the stored row.
func (s *Store) AddBy(ctx context.Context, ws, userID, kind, value string) (database.Mute, error) {
	v, err := Normalize(kind, value)
	if err != nil {
		return database.Mute{}, err
	}
	var by pgtype.UUID
	if userID != "" {
		_ = by.Scan(userID) // invalid id just leaves created_by NULL
	}
	m, err := s.q.CreateMute(ctx, database.CreateMuteParams{WorkspaceID: ws, Kind: kind, Value: v, CreatedBy: by})
	if err != nil {
		return database.Mute{}, fmt.Errorf("create mute: %w", err)
	}
	s.Invalidate(ws)
	return m, nil
}

// List returns ws's mutes, newest first.
func (s *Store) List(ctx context.Context, ws string) ([]database.Mute, error) {
	return s.q.ListMutes(ctx, ws)
}

// Delete removes a mute; false if it does not exist in ws.
func (s *Store) Delete(ctx context.Context, ws, id string) (bool, error) {
	n, err := s.q.DeleteMute(ctx, database.DeleteMuteParams{ID: id, WorkspaceID: ws})
	if err != nil {
		return false, fmt.Errorf("delete mute: %w", err)
	}
	s.Invalidate(ws)
	return n > 0, nil
}

// Invalidate drops the cached list for ws (call after deletes).
func (s *Store) Invalidate(ws string) {
	s.mu.Lock()
	delete(s.cache, ws)
	s.mu.Unlock()
}

// IsMuted reports whether any of ws's mutes matches c.
func (s *Store) IsMuted(ctx context.Context, ws string, c Candidate) (bool, error) {
	list, err := s.list(ctx, ws)
	if err != nil {
		return false, err
	}
	return Match(list, c), nil
}

func (s *Store) list(ctx context.Context, ws string) ([]Mute, error) {
	s.mu.Lock()
	e, ok := s.cache[ws]
	s.mu.Unlock()
	if ok && s.now().Sub(e.at) < cacheTTL {
		return e.list, nil
	}
	rows, err := s.q.ListMutes(ctx, ws)
	if err != nil {
		return nil, fmt.Errorf("list mutes: %w", err)
	}
	list := make([]Mute, len(rows))
	for i, r := range rows {
		list[i] = Mute{Kind: r.Kind, Value: r.Value}
	}
	s.mu.Lock()
	s.cache[ws] = cached{list: list, at: s.now()}
	s.mu.Unlock()
	return list, nil
}
