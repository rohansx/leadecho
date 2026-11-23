package researcher

import (
	"strings"
	"testing"
	"time"

	"leadecho/internal/database"
	"leadecho/internal/enrich"
	"leadecho/internal/identity"
)

func TestDossierInputChannelsAreOnlyVerifiedRoutes(t *testing.T) {
	prof := &identity.Profile{Name: "Jane Doe", Notes: []string{"I'm the founder of Acme"}, Links: []identity.Link{
		{Kind: identity.KindHackerNews, URL: "https://news.ycombinator.com/user?id=janed", Confidence: 1, Method: "origin"},
		{Kind: identity.KindLinkedIn, URL: "https://www.linkedin.com/in/jane-doe", Confidence: 0.9},
		{Kind: identity.KindX, URL: "https://x.com/janed", Confidence: 0.6}, // a candidate, not a channel
	}}
	ent := &enrich.Result{FullName: "Jane Doe", Title: "CTO", Email: "jane@acme.io", EmailStatus: "verified", Steps: map[string]string{}}
	posts := []database.PersonMention{{MentionID: "m1", Platform: "hackernews", Title: "CI is slow", PostedAt: time.Now()}}

	in := dossierInput("hackernews", "janed", prof, ent, posts)
	if strings.Join(in.Channels, ",") != "email,linkedin" {
		t.Errorf("channels = %v, want email,linkedin", in.Channels)
	}
	if !strings.Contains(in.Identity, "founder of Acme") || !strings.Contains(in.Enrichment, "Title: CTO") {
		t.Errorf("identity/enrichment text missing: %q / %q", in.Identity, in.Enrichment)
	}
	if len(in.Posts) != 1 || in.Posts[0].ID != "m1" {
		t.Errorf("posts = %+v", in.Posts)
	}
}

func TestDossierInputHashChangesOnlyWithInput(t *testing.T) {
	posts := []database.PersonMention{{MentionID: "m1", Title: "a", PostedAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}}
	a := inputHash(dossierInput("hackernews", "janed", nil, nil, posts))
	if a != inputHash(dossierInput("hackernews", "janed", nil, nil, posts)) {
		t.Fatal("same input must hash the same (no rebuild)")
	}
	more := append(posts, database.PersonMention{MentionID: "m2", Title: "b", PostedAt: posts[0].PostedAt})
	if a == inputHash(dossierInput("hackernews", "janed", nil, nil, more)) {
		t.Fatal("a new post must change the hash (rebuild)")
	}
}
