package monitor

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"

	"leadecho/internal/database"
)

// Threads that rank in web search: a Reddit, Hacker News or Dev.to thread
// that shows up in web search for a buyer's keyword keeps sending readers for
// months, so a reply there is worth more. Once a week per workspace, each
// active keyword that found an open Leads Ready thread is searched once on
// Exa (no page contents, about $0.005 a search) and the threads found in the
// results get platform_metadata.search_rank. Exa's order is its own, not
// Google's: the app says "web search", never "Google".

const (
	searchRankInterval   = 7 * 24 * time.Hour
	searchRankPoll       = time.Hour
	searchRankPerTick    = 3   // staggers the first deploy, when every workspace is due
	searchRankMaxQueries = 20  // hard cap: searches per workspace per week
	searchRankMaxThreads = 500 // newest open lead threads considered
	searchRankResults    = 10  // results per search: a first page
)

// SearchRank is stored on a mention whose thread showed up in web search.
type SearchRank struct {
	Query     string    `json:"query"`
	Position  int       `json:"position"` // 1-based
	CheckedAt time.Time `json:"checked_at"`
}

// watchSearchRank checks due workspaces every hour. Off without EXA_API_KEY.
func (m *Monitor) watchSearchRank(ctx context.Context) {
	if m.exaAPIKey == "" {
		return
	}
	t := time.NewTicker(searchRankPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.checkDueSearchRanks(ctx, time.Now().UTC())
		}
	}
}

func (m *Monitor) checkDueSearchRanks(ctx context.Context, now time.Time) {
	ids, err := m.q.ListWorkspacesDueSearchRank(ctx, now.Add(-searchRankInterval), searchRankPerTick)
	if err != nil {
		m.logger.Warn().Err(err).Msg("search rank: list due workspaces")
		return
	}
	for _, ws := range ids {
		// Marked first: a check that fails waits a week too, so the cap holds.
		if err := m.q.MarkSearchRankChecked(ctx, ws, now); err != nil {
			m.logger.Warn().Err(err).Str("workspace_id", ws).Msg("search rank: mark")
			continue
		}
		if m.credits.ScanAllowed(ctx, ws) {
			m.checkSearchRank(ctx, ws, now)
		}
	}
}

func (m *Monitor) checkSearchRank(ctx context.Context, ws string, now time.Time) {
	threads, err := m.q.ListSearchRankThreads(ctx, ws, searchRankMaxThreads)
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", ws).Msg("search rank: list threads")
		return
	}
	results := map[string][]string{}
	for _, q := range searchRankQueries(threads, searchRankMaxQueries) {
		hits, err := exaSearch(ctx, exaSearchURL, m.exaAPIKey, exaSearchRequest{Query: q, Type: "auto", NumResults: searchRankResults})
		if err != nil {
			m.logger.Warn().Err(err).Str("workspace_id", ws).Msg("search rank: search")
			continue // not searched: ranks stored for q stay as they are
		}
		urls := make([]string, 0, len(hits))
		for _, h := range hits {
			urls = append(urls, exaResultLink(h))
		}
		results[q] = urls
	}
	flagged := 0
	for id, rank := range matchSearchRanks(threads, results, now) {
		var raw []byte
		if rank != nil {
			raw, _ = json.Marshal(rank)
			flagged++
		}
		if err := m.q.SetMentionSearchRank(ctx, ws, id, raw); err != nil {
			m.logger.Warn().Err(err).Str("workspace_id", ws).Str("mention_id", id).Msg("search rank: save")
		}
	}
	m.logger.Info().Str("workspace_id", ws).Int("searches", len(results)).Int("ranking", flagged).Msg("search rank: checked")
}

// searchRankQueries: the distinct keywords behind threads, those with the
// most threads first, at most max.
func searchRankQueries(threads []database.SearchRankThread, max int) []string {
	count := map[string]int{}
	for _, t := range threads {
		if kw := strings.TrimSpace(t.Keyword); kw != "" {
			count[kw]++
		}
	}
	out := make([]string, 0, len(count))
	for kw := range count {
		out = append(out, kw)
	}
	sort.Slice(out, func(i, j int) bool {
		if count[out[i]] != count[out[j]] {
			return count[out[i]] > count[out[j]]
		}
		return out[i] < out[j]
	})
	return out[:min(len(out), max)]
}

// matchSearchRanks returns the changes to store, by mention id: the best
// (lowest) position a thread holds across the searched queries, or nil to
// clear a stored rank whose query was searched again and no longer finds it.
// Threads with nothing to change are left out.
func matchSearchRanks(threads []database.SearchRankThread, results map[string][]string, now time.Time) map[string]*SearchRank {
	queries := make([]string, 0, len(results))
	for q := range results {
		queries = append(queries, q)
	}
	sort.Strings(queries)
	best := map[string]*SearchRank{} // thread key -> best rank
	for _, q := range queries {
		for i, u := range results[q] {
			k := threadKey(u)
			if k == "" {
				continue
			}
			if r, ok := best[k]; !ok || i+1 < r.Position {
				best[k] = &SearchRank{Query: q, Position: i + 1, CheckedAt: now}
			}
		}
	}
	out := map[string]*SearchRank{}
	for _, t := range threads {
		if r, ok := best[threadKey(t.URL)]; ok {
			out[t.MentionID] = r
		} else if _, searched := results[t.RankQuery]; t.RankQuery != "" && searched {
			out[t.MentionID] = nil
		}
	}
	return out
}

// redditHostPrefixes are Reddit's alternate front ends for the same thread.
var redditHostPrefixes = []string{"www.", "old.", "new.", "np.", "m.", "amp."}

// threadKey names the thread a URL points at, so the same thread matches
// however it is linked: any Reddit host, slug or comment permalink is the
// post id; Hacker News is the item id; anything else is host and path
// without www., query, fragment or trailing slash. "" for no thread.
func threadKey(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	for _, p := range redditHostPrefixes {
		if strings.HasPrefix(host, p) && strings.HasSuffix(host, "reddit.com") {
			host = strings.TrimPrefix(host, p)
			break
		}
	}
	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")
	switch host {
	case "reddit.com":
		for i, p := range parts {
			if p == "comments" && i+1 < len(parts) && parts[i+1] != "" {
				return "reddit:" + strings.ToLower(parts[i+1])
			}
		}
	case "redd.it":
		if path != "" {
			return "reddit:" + strings.ToLower(parts[0])
		}
	case "news.ycombinator.com":
		if id := u.Query().Get("id"); id != "" {
			return "hn:" + id
		}
		return ""
	}
	if path == "" {
		return ""
	}
	return strings.TrimPrefix(host, "www.") + "/" + strings.ToLower(path)
}
