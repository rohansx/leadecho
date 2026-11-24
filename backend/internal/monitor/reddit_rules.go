package monitor

import (
	"context"
	"net/url"
	"strings"

	"leadecho/internal/subrules"
)

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
