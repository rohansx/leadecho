package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// Bluesky search via the AT Protocol AppView. api.bsky.app answers
// app.bsky.feed.searchPosts unauthenticated (public.api.bsky.app returns 403
// for search as of 2026-11). With BLUESKY_HANDLE/BLUESKY_APP_PASSWORD set we
// search through the PDS with a session instead, which is less rate limited.
var (
	blueskyPublicBase = "https://api.bsky.app/xrpc"
	blueskyAuthBase   = "https://bsky.social/xrpc"
	blueskyHTTP       = &http.Client{Timeout: 20 * time.Second}
)

const (
	blueskyLimit = 25
	// Access JWTs live ~2h; refresh well before that.
	blueskySessionTTL = 90 * time.Minute
)

type blueskyAuth struct {
	handle, password string
	mu               sync.Mutex
	token            string
	expires          time.Time
}

type bskyPost struct {
	URI    string `json:"uri"`
	Author struct {
		DID         string `json:"did"`
		Handle      string `json:"handle"`
		DisplayName string `json:"displayName"`
	} `json:"author"`
	Record struct {
		Text      string `json:"text"`
		CreatedAt string `json:"createdAt"`
	} `json:"record"`
	LikeCount   int `json:"likeCount"`
	ReplyCount  int `json:"replyCount"`
	RepostCount int `json:"repostCount"`
}

// SetBlueskyCredentials enables authenticated search (optional).
func (m *Monitor) SetBlueskyCredentials(handle, appPassword string) {
	m.bluesky.mu.Lock()
	defer m.bluesky.mu.Unlock()
	m.bluesky.handle, m.bluesky.password = handle, appPassword
	m.bluesky.token = ""
}

// blueskyToken returns a cached access JWT, creating a session if needed.
// Empty token with nil error means "no credentials: search anonymously".
func (m *Monitor) blueskyToken(ctx context.Context) (string, error) {
	a := &m.bluesky
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.handle == "" || a.password == "" {
		return "", nil
	}
	if a.token != "" && time.Now().Before(a.expires) {
		return a.token, nil
	}
	body, _ := json.Marshal(map[string]string{"identifier": a.handle, "password": a.password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, blueskyAuthBase+"/com.atproto.server.createSession", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := blueskyHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("bluesky createSession: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bluesky createSession: status %d", resp.StatusCode)
	}
	var s struct {
		AccessJwt string `json:"accessJwt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil || s.AccessJwt == "" {
		return "", fmt.Errorf("bluesky createSession: bad response: %v", err)
	}
	a.token, a.expires = s.AccessJwt, time.Now().Add(blueskySessionTTL)
	return a.token, nil
}

func (m *Monitor) clearBlueskyToken() {
	m.bluesky.mu.Lock()
	m.bluesky.token = ""
	m.bluesky.mu.Unlock()
}

func (m *Monitor) fetchBluesky(ctx context.Context, term string) ([]bskyPost, error) {
	token, err := m.blueskyToken(ctx)
	if err != nil {
		return nil, err
	}
	base := blueskyPublicBase
	if token != "" {
		base = blueskyAuthBase
	}
	q := url.Values{"q": {term}, "limit": {fmt.Sprint(blueskyLimit)}, "sort": {"latest"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/app.bsky.feed.searchPosts?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "LeadEcho/1.0")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := blueskyHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bluesky search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized && token != "" {
		m.clearBlueskyToken() // expired early; next tick re-authenticates
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bluesky search: status %d", resp.StatusCode)
	}
	var out struct {
		Posts []bskyPost `json:"posts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("bluesky search: decode: %w", err)
	}
	return out.Posts, nil
}

// blueskyPostURL maps at://<did>/app.bsky.feed.post/<rkey> to its bsky.app
// web URL ("" if the URI is not a post).
func blueskyPostURL(uri, handle string) string {
	parts := strings.Split(strings.TrimPrefix(uri, "at://"), "/")
	if !strings.HasPrefix(uri, "at://") || len(parts) != 3 || parts[1] != "app.bsky.feed.post" || parts[2] == "" {
		return ""
	}
	who := handle
	if who == "" {
		who = parts[0]
	}
	return "https://bsky.app/profile/" + who + "/post/" + parts[2]
}

func (m *Monitor) crawlBluesky(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	posts, err := m.fetchBluesky(ctx, kw.Term)
	if err != nil {
		m.logger.Warn().Err(err).Str("keyword", kw.Term).Msg("bluesky: search failed")
		return nil
	}
	var alerts []mentionAlert
	for _, p := range posts {
		postURL := blueskyPostURL(p.URI, p.Author.Handle)
		if postURL == "" || p.Record.Text == "" || !filterContent(p.Record.Text, kw) {
			continue
		}
		var created pgtype.Timestamptz
		if t, err := time.Parse(time.RFC3339, p.Record.CreatedAt); err == nil {
			created = pgtype.Timestamptz{Time: t, Valid: true}
		}
		alert := m.insertMention(ctx, database.CreateMentionParams{
			WorkspaceID:       wsID,
			KeywordID:         pgUUID(kw.ID),
			Platform:          string(database.PlatformTypeBluesky),
			PlatformID:        "bsky_" + p.URI,
			Url:               postURL,
			Content:           p.Record.Text,
			AuthorUsername:    pgtextPtr(p.Author.Handle),
			AuthorProfileUrl:  pgtextPtr("https://bsky.app/profile/" + p.Author.Handle),
			Status:            database.MentionStatusNew,
			PlatformMetadata:  jsonBytes(map[string]any{"uri": p.URI, "did": p.Author.DID, "display_name": p.Author.DisplayName}),
			EngagementMetrics: jsonBytes(map[string]any{"likes": p.LikeCount, "replies": p.ReplyCount, "reposts": p.RepostCount}),
			KeywordMatches:    []string{kw.Term},
			PlatformCreatedAt: created,
		}, kw.Term)
		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}
	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("bluesky: new mentions found")
	}
	return alerts
}
