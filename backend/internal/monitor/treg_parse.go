package monitor

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// tregPost is one search hit from a treg tool, normalised. treg relays
// provider-native rows (it does not normalise), so parsing is tolerant: each
// field takes the first of several known key names. See docs/treg-integration.md.
type tregPost struct {
	ID, Text, URL, Author, AuthorURL string
	CreatedAt                        time.Time
	Likes, Replies, Reposts          int
}

// ponytail: key lists cover anyapi/tikhub/justoneapi-style rows as documented;
// tighten against captured traffic once a TREG_TOKEN is live.
var (
	rowContainerKeys = []string{"output", "posts", "results", "data", "items", "elements", "tweets"}
	idKeys           = []string{"id_str", "rest_id", "tweetId", "tweet_id", "urn", "activityId", "postId", "id"}
	textKeys         = []string{"full_text", "fullText", "text", "content", "commentary", "body", "description", "snippet"}
	urlKeys          = []string{"url", "twitterUrl", "tweetUrl", "postUrl", "post_url", "shareUrl", "link"}
	timeKeys         = []string{"createdAt", "created_at", "publishedAt", "published_at", "postedAt", "posted_at", "publishDate", "date", "timestamp", "time"}
	userKeys         = []string{"userName", "username", "screen_name", "handle", "publicIdentifier", "name"}
	authorURLKeys    = []string{"url", "profileUrl", "profile_url", "linkedinUrl"}
)

func parseTregPosts(body []byte) []tregPost {
	var v any
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	var out []tregPost
	for _, row := range findRows(v, 0) {
		if p := toTregPost(row); p.Text != "" {
			out = append(out, p)
		}
	}
	return out
}

// findRows returns the first array of objects found under the usual
// container keys ({output:{posts:[…]}}, {results:[…]}, a bare array, …).
func findRows(v any, depth int) []map[string]any {
	if depth > 3 {
		return nil
	}
	switch t := v.(type) {
	case []any:
		var rows []map[string]any
		for _, e := range t {
			if m, ok := e.(map[string]any); ok {
				rows = append(rows, m)
			}
		}
		return rows
	case map[string]any:
		for _, k := range rowContainerKeys {
			if child, ok := t[k]; ok {
				if rows := findRows(child, depth+1); len(rows) > 0 {
					return rows
				}
			}
		}
	}
	return nil
}

func toTregPost(row map[string]any) tregPost {
	if legacy, ok := row["legacy"].(map[string]any); ok {
		merged := make(map[string]any, len(row)+len(legacy))
		for k, v := range row {
			merged[k] = v
		}
		for k, v := range legacy {
			merged[k] = v
		}
		row = merged
	}
	p := tregPost{
		ID:      str(row, idKeys...),
		Text:    strings.TrimSpace(str(row, textKeys...)),
		URL:     str(row, urlKeys...),
		Likes:   num(row, "likeCount", "like_count", "favorite_count", "likes", "numLikes", "reactions"),
		Replies: num(row, "replyCount", "reply_count", "replies", "numComments", "comments"),
		Reposts: num(row, "retweetCount", "retweet_count", "reposts", "numShares", "shares"),
	}
	p.CreatedAt = parseAnyTime(row, timeKeys)
	for _, k := range []string{"author", "user"} {
		switch a := row[k].(type) {
		case string:
			p.Author = a
		case map[string]any:
			p.Author, p.AuthorURL = str(a, userKeys...), str(a, authorURLKeys...)
		}
		if p.Author != "" {
			break
		}
	}
	if p.Author == "" {
		p.Author = str(row, "username", "screen_name", "author_username", "authorName", "author_name")
	}
	if p.AuthorURL == "" {
		p.AuthorURL = str(row, "authorUrl", "author_url", "authorProfileUrl")
	}
	return p
}

func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if v != "" {
				return v
			}
		case float64:
			return strconv.FormatInt(int64(v), 10)
		}
	}
	return ""
}

func num(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k].(float64); ok {
			return int(v)
		}
	}
	return 0
}

var timeLayouts = []string{time.RFC3339, time.RubyDate, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"}

func parseAnyTime(m map[string]any, keys []string) time.Time {
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64:
			if v > 1e12 { // milliseconds
				return time.UnixMilli(int64(v)).UTC()
			}
			if v > 0 {
				return time.Unix(int64(v), 0).UTC()
			}
		case string:
			for _, l := range timeLayouts {
				if t, err := time.Parse(l, v); err == nil {
					return t.UTC()
				}
			}
		}
	}
	return time.Time{}
}

func tregMentionBase(wsID string, kw database.ListActiveKeywordsRow, platform database.PlatformType, p tregPost, tool string) database.CreateMentionParams {
	created := pgtype.Timestamptz{}
	if !p.CreatedAt.IsZero() {
		created = pgtype.Timestamptz{Time: p.CreatedAt, Valid: true}
	}
	return database.CreateMentionParams{
		WorkspaceID:       wsID,
		KeywordID:         pgUUID(kw.ID),
		Platform:          string(platform),
		Url:               p.URL,
		Content:           p.Text,
		AuthorUsername:    pgtextPtr(p.Author),
		AuthorProfileUrl:  pgtextPtr(p.AuthorURL),
		Status:            database.MentionStatusNew,
		PlatformMetadata:  jsonBytes(map[string]any{"source": "treg", "tool": tool, "query": kw.Term}),
		EngagementMetrics: jsonBytes(map[string]any{"likes": p.Likes, "replies": p.Replies, "reposts": p.Reposts}),
		KeywordMatches:    []string{kw.Term},
		PlatformCreatedAt: created,
	}
}

// tregTwitterParams maps a hit to a mention. PlatformID matches the
// Pinchtab/Scrapling crawlers ("twitter_<status id>") so sources dedupe.
func tregTwitterParams(wsID string, kw database.ListActiveKeywordsRow, p tregPost) (database.CreateMentionParams, bool) {
	id := p.ID
	if id == "" {
		id = extractStatusID(p.URL)
	}
	if id == "" {
		return database.CreateMentionParams{}, false
	}
	p.Author = strings.TrimPrefix(p.Author, "@")
	if p.URL == "" && p.Author != "" {
		p.URL = "https://x.com/" + p.Author + "/status/" + id
	}
	if p.URL == "" {
		p.URL = "https://x.com/i/status/" + id
	}
	if p.AuthorURL == "" && p.Author != "" {
		p.AuthorURL = "https://x.com/" + p.Author
	}
	params := tregMentionBase(wsID, kw, database.PlatformTypeTwitter, p, tregXTool)
	params.PlatformID = "twitter_" + id
	return params, true
}

// tregLinkedInParams uses the activity id from the post URN, like the
// browser crawlers ("linkedin_<id>").
func tregLinkedInParams(wsID string, kw database.ListActiveKeywordsRow, p tregPost) (database.CreateMentionParams, bool) {
	if p.URL == "" {
		return database.CreateMentionParams{}, false
	}
	id := extractLinkedInID(p.URL)
	if id == "" && p.ID != "" {
		id = p.ID
		if strings.Contains(id, ":") {
			id = extractLinkedInID(id)
		}
	}
	if id == "" {
		return database.CreateMentionParams{}, false
	}
	params := tregMentionBase(wsID, kw, database.PlatformTypeLinkedin, p, tregLinkedInTool)
	params.PlatformID = "linkedin_" + id
	return params, true
}
