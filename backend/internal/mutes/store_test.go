package mutes

import (
	"context"
	"errors"
	"testing"
	"time"

	"leadecho/internal/database"
)

// fakeQ implements only the mute queries; any other call panics (nil embed).
type fakeQ struct {
	database.Querier
	rows      []database.Mute
	listCalls int
	listErr   error
}

func (f *fakeQ) ListMutes(_ context.Context, ws string) ([]database.Mute, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []database.Mute
	for _, r := range f.rows {
		if r.WorkspaceID == ws {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeQ) CreateMute(_ context.Context, p database.CreateMuteParams) (database.Mute, error) {
	m := database.Mute{ID: "m1", WorkspaceID: p.WorkspaceID, Kind: p.Kind, Value: p.Value, CreatedBy: p.CreatedBy}
	f.rows = append(f.rows, m)
	return m, nil
}

func TestStoreAddAndIsMuted(t *testing.T) {
	ctx := context.Background()
	f := &fakeQ{}
	s := NewStore(f)

	if err := s.Add(ctx, "ws1", "bogus", "x"); err == nil {
		t.Fatal("invalid kind must error")
	}
	if err := s.Add(ctx, "ws1", KindAuthor, "@SpamBot"); err != nil {
		t.Fatal(err)
	}
	if f.rows[0].Value != "spambot" {
		t.Errorf("stored value = %q, want normalized spambot", f.rows[0].Value)
	}

	muted, err := s.IsMuted(ctx, "ws1", Candidate{Author: "spambot"})
	if err != nil || !muted {
		t.Fatalf("IsMuted = %v,%v want true", muted, err)
	}
	// Cached: a second lookup inside the TTL does not hit the DB.
	_, _ = s.IsMuted(ctx, "ws1", Candidate{Author: "other"})
	if f.listCalls != 1 {
		t.Errorf("listCalls = %d, want 1 (cached)", f.listCalls)
	}
	// Workspace isolation.
	if muted, _ := s.IsMuted(ctx, "ws2", Candidate{Author: "spambot"}); muted {
		t.Error("mute leaked across workspaces")
	}
	// Add invalidates the cache.
	if err := s.Add(ctx, "ws1", KindKeyword, "crypto"); err != nil {
		t.Fatal(err)
	}
	if muted, _ := s.IsMuted(ctx, "ws1", Candidate{Content: "free CRYPTO"}); !muted {
		t.Error("new mute not visible after Add")
	}
}

func TestStoreCacheExpiry(t *testing.T) {
	f := &fakeQ{}
	s := NewStore(f)
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }
	_, _ = s.IsMuted(context.Background(), "ws", Candidate{})
	now = now.Add(cacheTTL + time.Second)
	_, _ = s.IsMuted(context.Background(), "ws", Candidate{})
	if f.listCalls != 2 {
		t.Errorf("listCalls = %d, want 2 after TTL", f.listCalls)
	}
}

func TestStoreIsMutedError(t *testing.T) {
	s := NewStore(&fakeQ{listErr: errors.New("db down")})
	if muted, err := s.IsMuted(context.Background(), "ws", Candidate{}); err == nil || muted {
		t.Errorf("want (false, err), got (%v, %v)", muted, err)
	}
}
