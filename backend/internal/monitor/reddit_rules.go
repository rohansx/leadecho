package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"leadecho/internal/subrules"
	"leadecho/internal/treg"
)

// tregPageTool reads old Reddit's rules page with a real browser when Reddit
// refuses us directly ($0.00025 a page; a blocked page is free). No treg
// Reddit tool returns rules: anyapi.reddit.subreddit_details has only a
// one-line description.
const tregPageTool = "crawl4ai.web.scrape"

// SubredditRulesFetcher is how subreddit rules are read, the way posts are:
// Reddit's API when REDDIT_CLIENT_ID/SECRET are set (falling back to treg if
// it fails), else treg when configured (production), else public JSON.
func SubredditRulesFetcher(s *PublicSources, t *treg.Client) subrules.FetchFunc {
	return func(ctx context.Context, wsID, sub string) (subrules.Raw, error) {
		if s.RedditOAuth != nil || !t.Enabled() {
			raw, err := s.SubredditRules(ctx, sub)
			if err == nil || !t.Enabled() {
				return raw, err
			}
		}
		return tregSubredditRules(ctx, t, wsID, sub)
	}
}

// tregSubredditRules scrapes old.reddit.com/r/<sub>/about/rules, whose page
// also carries the sidebar, so one metered call gets both.
func tregSubredditRules(ctx context.Context, t *treg.Client, wsID, sub string) (subrules.Raw, error) {
	resp, err := t.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tregPageTool, Body: map[string]string{
		"url": "https://old.reddit.com/r/" + url.PathEscape(sub) + "/about/rules", "format": "md"}})
	if err != nil {
		return subrules.Raw{}, err
	}
	// Same answer shape as onboarding.ParseRenderedPage (bare or under "output");
	// onboarding imports monitor, so it is read here.
	var r struct {
		Markdown string `json:"markdown"`
		Output   struct {
			Markdown string `json:"markdown"`
		} `json:"output"`
	}
	_ = json.Unmarshal(resp.Body, &r) // bad JSON reads as an empty page
	md := r.Markdown
	if md == "" {
		md = r.Output.Markdown
	}
	raw := subrules.RawFromPage(md)
	if len(raw.Rules) == 0 {
		return raw, errors.New("treg: rules page empty or blocked")
	}
	return raw, nil
}

// redditGet fetches pathAndQuery ("/r/x/new?limit=25") via OAuth when
// configured, else from the public "<path>.json" endpoint: the one Reddit
// path every caller shares.
func (s *PublicSources) redditGet(ctx context.Context, pathAndQuery string, out any) error {
	if s.RedditOAuth != nil {
		return s.RedditOAuth.getJSON(ctx, s.Client, pathAndQuery, out)
	}
	path, query, _ := strings.Cut(pathAndQuery, "?")
	u := s.RedditBase + path + ".json"
	if query != "" {
		u += "?" + query
	}
	return s.getJSON(ctx, u, out)
}

// SubredditRules reads a subreddit's rules and sidebar (/about/rules and
// /about) through the crawler's Reddit path. The sidebar is optional: a
// failure there keeps the rules.
func (s *PublicSources) SubredditRules(ctx context.Context, sub string) (subrules.Raw, error) {
	base := "/r/" + url.PathEscape(sub) + "/about"
	var rules struct {
		Rules []subrules.Rule `json:"rules"`
	}
	if err := s.redditGet(ctx, base+"/rules", &rules); err != nil {
		return subrules.Raw{}, err
	}
	raw := subrules.Raw{Rules: rules.Rules}
	var about struct {
		Data struct {
			Description       string `json:"description"`
			PublicDescription string `json:"public_description"`
			SubmitText        string `json:"submit_text"`
		} `json:"data"`
	}
	if s.redditGet(ctx, base, &about) == nil {
		d := about.Data
		raw.Sidebar = strings.Join([]string{d.PublicDescription, d.Description, d.SubmitText}, "\n")
	}
	return raw, nil
}
