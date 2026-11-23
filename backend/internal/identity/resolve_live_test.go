//go:build live

package identity

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// go test -tags live -run TestLiveResolve -v ./internal/identity/
// IDENTITY_SEEDS_FILE=seeds.json ([{"Platform":"hackernews","Handle":"simonw"}]) overrides the seeds.
func TestLiveResolve(t *testing.T) {
	seeds := []Seed{
		{Platform: "hackernews", Handle: "simonw"},
		{Platform: "hackernews", Handle: "patio11"},
		{Platform: "devto", Handle: "ben"},
		{Platform: "bluesky", Handle: "jay.bsky.team"},
	}
	if f := os.Getenv("IDENTITY_SEEDS_FILE"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &seeds); err != nil {
			t.Fatal(err)
		}
	}
	r := New(os.Getenv("GITHUB_TOKEN"), nil)
	for _, s := range seeds {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		start := time.Now()
		p, err := r.Resolve(ctx, s)
		cancel()
		if err != nil {
			t.Logf("%s/%s: %v", s.Platform, s.Handle, err)
			continue
		}
		b, _ := json.MarshalIndent(p, "", "  ")
		t.Logf("%s/%s (%s):\n%s", s.Platform, s.Handle, time.Since(start).Round(time.Millisecond), b)
	}
}
