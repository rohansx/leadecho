package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ToolXProfile returns an X account's public name and bio.
const ToolXProfile = "treg.x.user.profile"

// StepHNProfile is the free Hacker News profile lookup (not a treg tool).
const StepHNProfile = "hackernews.user.profile"

// hnUserURL is a var so tests can point it at a local server.
var hnUserURL = "https://hacker-news.firebaseio.com/v0/user/%s.json"

var hnHTTP = &http.Client{Timeout: 10 * time.Second}

var (
	emailRe    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+`)
	linkedInRe = regexp.MustCompile(`(?i)linkedin\.com/in/[A-Za-z0-9][A-Za-z0-9_-]{1,99}`)
	tagRe      = regexp.MustCompile(`<[^>]*>`)
)

type stepFunc func(tool string, body map[string]any, apply func(map[string]any) bool)

// identify looks up the public profile behind an X or Hacker News handle and
// returns the subject with any email or LinkedIn URL the profile reveals, so
// the paid person lookup has something to match on. The bio itself is kept:
// "CTO at Acme" is often all a reader needs.
func (p *Pipeline) identify(ctx context.Context, s Subject, r *Result, step stepFunc) Subject {
	switch strings.ToLower(s.Platform) {
	case "twitter", "x":
		if !slugRe.MatchString(s.Handle) {
			return s
		}
		step(ToolXProfile, map[string]any{"username": s.Handle}, func(o map[string]any) bool {
			r.Bio = cleanBio(str(o, "description"))
			if name := strings.TrimSpace(str(o, "name")); name != "" && !strings.HasPrefix(name, "@") {
				r.FullName = name
			}
			return r.Bio != "" || r.FullName != ""
		})
	case "hackernews":
		about, err := hnAbout(ctx, s.Handle)
		switch {
		case err != nil:
			r.Steps[StepHNProfile] = StepError
			p.logger.Debug().Err(err).Str("handle", s.Handle).Msg("enrich: hn profile lookup failed")
		case about == "":
			r.Steps[StepHNProfile] = StepMiss
		default:
			r.Steps[StepHNProfile] = StepHit
			r.Bio = about
		}
	default:
		return s
	}
	if s.Email == "" {
		s.Email = emailRe.FindString(r.Bio)
		r.Email = s.Email
	}
	if !isLinkedInProfile(s.ProfileURL) {
		if m := linkedInRe.FindString(r.Bio); m != "" {
			s.ProfileURL = "https://www." + m
		}
	}
	return s
}

// hnAbout fetches a Hacker News user's "about" text ("" when empty or unknown).
func hnAbout(ctx context.Context, handle string) (string, error) {
	if !slugRe.MatchString(handle) {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(hnUserURL, url.PathEscape(handle)), nil)
	if err != nil {
		return "", err
	}
	resp, err := hnHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("hn user: status %d", resp.StatusCode)
	}
	var u *struct {
		About string `json:"about"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return "", err
	}
	if u == nil { // unknown user: the API answers null
		return "", nil
	}
	return cleanBio(u.About), nil
}

// cleanBio strips HTML and caps the length: bios are untrusted public text.
func cleanBio(s string) string {
	s = strings.ReplaceAll(s, "<p>", "\n")
	s = strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(s, "")))
	if r := []rune(s); len(r) > 500 {
		s = string(r[:500]) + "…"
	}
	return s
}
