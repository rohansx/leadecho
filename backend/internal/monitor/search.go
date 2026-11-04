package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// Post is one item returned by a public source search, before (and
// independent of) mention insertion. The onboarding listener builder uses it
// to live-test keywords without touching the inbox; the crawlers turn it into
// a mention.
type Post struct {
	Platform   string         `json:"platform"`
	ExternalID string         `json:"-"`
	URL        string         `json:"url"`
	Title      string         `json:"title"`
	Content    string         `json:"text"`
	Author     string         `json:"author"`
	AuthorURL  string         `json:"-"`
	CreatedAt  time.Time      `json:"created_at"`
	Metadata   map[string]any `json:"-"`
	Engagement map[string]any `json:"-"`
}

// StatusError is a non-200 answer from a public source; Reddit's 403/429 drive
// the crawler's backoff.
type StatusError struct{ Code int }

func (e *StatusError) Error() string { return fmt.Sprintf("non-200 response (status %d)", e.Code) }

const maxSourceBody = 5 << 20

// PublicSources searches the free, keyless public APIs (Reddit JSON, HN
// Algolia, Dev.to). Base URLs are fields so tests point them at httptest.
type PublicSources struct {
	Client     *http.Client
	RedditBase string
	HNBase     string
	DevToBase  string
}

// DefaultPublicSources returns the production endpoints. The client has no
// timeout of its own: callers bound each search with a context deadline.
func DefaultPublicSources() *PublicSources {
	return &PublicSources{
		Client:     http.DefaultClient,
		RedditBase: "https://www.reddit.com",
		HNBase:     "https://hn.algolia.com",
		DevToBase:  "https://dev.to",
	}
}

// Search dispatches to the platform's search. Platforms without a free public
// search return an error so callers can report them as untestable.
func (s *PublicSources) Search(ctx context.Context, platform, term string, since time.Time) ([]Post, error) {
	switch platform {
	case string(database.PlatformTypeReddit):
		return s.SearchReddit(ctx, term, since)
	case string(database.PlatformTypeHackernews):
		return s.SearchHackerNews(ctx, term, since)
	case "devto":
		return s.SearchDevTo(ctx, term, since)
	}
	return nil, fmt.Errorf("no public search for platform %q", platform)
}

func (s *PublicSources) getJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; LeadEcho/1.0)")
	resp, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &StatusError{Code: resp.StatusCode}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxSourceBody)).Decode(out)
}

// SearchHackerNews queries Algolia's by-date index for stories and comments.
// A non-zero since becomes a server-side created_at_i filter.
func (s *PublicSources) SearchHackerNews(ctx context.Context, term string, since time.Time) ([]Post, error) {
	apiURL := fmt.Sprintf("%s/api/v1/search_by_date?query=%s&tags=(story,comment)&hitsPerPage=25", s.HNBase, url.QueryEscape(term))
	if !since.IsZero() {
		apiURL += "&numericFilters=" + url.QueryEscape(fmt.Sprintf("created_at_i>%d", since.Unix()))
	}
	var result hnSearchResult
	if err := s.getJSON(ctx, apiURL, &result); err != nil {
		return nil, fmt.Errorf("hn: %w", err)
	}
	posts := make([]Post, 0, len(result.Hits))
	for _, hit := range result.Hits {
		if p, ok := hnHitPost(hit); ok {
			posts = append(posts, p)
		}
	}
	return posts, nil
}

func hnHitPost(hit hnHit) (Post, bool) {
	content := hit.Title
	if hit.StoryText != "" {
		content = hit.Title + "\n\n" + cleanHNText(hit.StoryText)
	}
	if hit.CommentText != "" {
		content = cleanHNText(hit.CommentText)
	}
	if content == "" {
		return Post{}, false
	}
	return Post{
		Platform:   string(database.PlatformTypeHackernews),
		ExternalID: "hn_" + hit.ObjectID,
		URL:        "https://news.ycombinator.com/item?id=" + hit.ObjectID,
		Title:      hit.Title,
		Content:    content,
		Author:     hit.Author,
		AuthorURL:  "https://news.ycombinator.com/user?id=" + hit.Author,
		CreatedAt:  time.Unix(hit.CreatedAtI, 0),
		Metadata:   map[string]any{"points": hit.Points, "external_url": hit.URL},
		Engagement: map[string]any{"points": hit.Points, "num_comments": hit.NumComments},
	}, true
}

// redditTimeWindow maps a since cutoff onto Reddit search's coarse t= values.
func redditTimeWindow(since time.Time) string {
	if since.IsZero() {
		return "week"
	}
	age := time.Since(since) - time.Hour // slack: "7 days ago" computed a moment earlier is still a week
	switch {
	case age <= 24*time.Hour:
		return "day"
	case age <= 7*24*time.Hour:
		return "week"
	case age <= 31*24*time.Hour:
		return "month"
	}
	return "year"
}

// SearchReddit uses Reddit's public site-wide search. The crawler prefers
// per-subreddit listings (gentler on rate limits); this is for one-off
// live tests and backfills.
func (s *PublicSources) SearchReddit(ctx context.Context, term string, since time.Time) ([]Post, error) {
	apiURL := fmt.Sprintf("%s/search.json?q=%s&sort=new&limit=25&t=%s", s.RedditBase, url.QueryEscape(term), redditTimeWindow(since))
	return s.redditListing(ctx, apiURL, since)
}

// SubredditNew fetches a subreddit's newest posts.
func (s *PublicSources) SubredditNew(ctx context.Context, subreddit string, since time.Time) ([]Post, error) {
	return s.redditListing(ctx, fmt.Sprintf("%s/r/%s/new.json?limit=25", s.RedditBase, url.PathEscape(subreddit)), since)
}

func (s *PublicSources) redditListing(ctx context.Context, apiURL string, since time.Time) ([]Post, error) {
	var listing redditListing
	if err := s.getJSON(ctx, apiURL, &listing); err != nil {
		return nil, err
	}
	var posts []Post
	for _, child := range listing.Data.Children {
		post := child.Data
		if post.Author == "[deleted]" || post.Author == "AutoModerator" {
			continue
		}
		createdAt := time.Unix(int64(post.CreatedUTC), 0)
		if !since.IsZero() && createdAt.Before(since) {
			continue
		}
		content := post.Title
		if post.Selftext != "" {
			content = post.Title + "\n\n" + post.Selftext
		}
		posts = append(posts, Post{
			Platform:   string(database.PlatformTypeReddit),
			ExternalID: "reddit_" + post.ID,
			URL:        "https://reddit.com" + post.Permalink,
			Title:      post.Title,
			Content:    content,
			Author:     post.Author,
			AuthorURL:  "https://reddit.com/u/" + post.Author,
			CreatedAt:  createdAt,
			Metadata:   map[string]any{"subreddit": post.Subreddit, "score": post.Score},
			Engagement: map[string]any{"score": post.Score, "num_comments": post.NumComments},
		})
	}
	return posts, nil
}

// SearchDevTo lists fresh articles for the term's first usable word (Dev.to
// only filters by single-word tags). since is applied locally.
func (s *PublicSources) SearchDevTo(ctx context.Context, term string, since time.Time) ([]Post, error) {
	tag := extractFirstWord(term)
	if tag == "" {
		return nil, nil
	}
	var articles []devtoArticle
	apiURL := fmt.Sprintf("%s/api/articles?tag=%s&per_page=25&state=fresh", s.DevToBase, url.QueryEscape(tag))
	if err := s.getJSON(ctx, apiURL, &articles); err != nil {
		return nil, fmt.Errorf("devto: %w", err)
	}
	var posts []Post
	for _, a := range articles {
		content := a.Title
		if a.Description != "" {
			content = a.Title + "\n\n" + a.Description
		}
		if content == "" {
			continue
		}
		var published time.Time
		if t, err := time.Parse(time.RFC3339, a.PublishedAt); err == nil {
			published = t
		}
		if !since.IsZero() && !published.IsZero() && published.Before(since) {
			continue
		}
		link := a.URL
		if a.CanonicalURL != "" {
			link = a.CanonicalURL
		}
		posts = append(posts, Post{
			Platform:   "devto",
			ExternalID: "devto_" + strconv.Itoa(a.ID),
			URL:        link,
			Title:      a.Title,
			Content:    content,
			Author:     a.User.Username,
			AuthorURL:  "https://dev.to/" + a.User.Username,
			CreatedAt:  published,
			Metadata:   map[string]any{"tags": a.TagList},
			Engagement: map[string]any{"reactions": a.PositiveReactionsCount, "comments": a.CommentsCount},
		})
	}
	return posts, nil
}

// mentionParams converts a searched post into the insert params for a keyword.
func (p Post) mentionParams(wsID string, kw database.ListActiveKeywordsRow) database.CreateMentionParams {
	var created pgtype.Timestamptz
	if !p.CreatedAt.IsZero() {
		created = pgtype.Timestamptz{Time: p.CreatedAt, Valid: true}
	}
	return database.CreateMentionParams{
		WorkspaceID:       wsID,
		KeywordID:         pgUUID(kw.ID),
		Platform:          p.Platform,
		PlatformID:        p.ExternalID,
		Url:               p.URL,
		Title:             pgtextPtr(p.Title),
		Content:           p.Content,
		AuthorUsername:    pgtextPtr(p.Author),
		AuthorProfileUrl:  pgtextPtr(p.AuthorURL),
		Status:            database.MentionStatusNew,
		PlatformMetadata:  jsonBytes(p.Metadata),
		EngagementMetrics: jsonBytes(p.Engagement),
		KeywordMatches:    []string{kw.Term},
		PlatformCreatedAt: created,
	}
}

// insertPosts filters posts for the keyword and inserts the new ones.
func (m *Monitor) insertPosts(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow, posts []Post) []mentionAlert {
	var alerts []mentionAlert
	for _, p := range posts {
		if !filterContent(p.Content, kw) {
			continue
		}
		if alert := m.insertMention(ctx, p.mentionParams(wsID, kw), kw.Term); alert != nil {
			alerts = append(alerts, *alert)
		}
	}
	return alerts
}

type sinceKey struct{}

// WithSince asks the crawlers that support time filters (HN, Reddit, Dev.to)
// to only return posts newer than t. Carried on the context so the existing
// crawl signatures stay unchanged.
func WithSince(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, sinceKey{}, t)
}

func sinceFrom(ctx context.Context) time.Time {
	t, _ := ctx.Value(sinceKey{}).(time.Time)
	return t
}

func (m *Monitor) sources() *PublicSources {
	if m.public != nil {
		return m.public
	}
	return DefaultPublicSources()
}
