// Package engagers turns the commenters and reactors of a LinkedIn post into
// leads through treg (plan P2 #16).
package engagers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"leadecho/internal/treg"
)

const (
	KindComments  = "comments"
	KindReactions = "reactions"

	ToolComments  = "treg.linkedin.post.comments"  // routed, $0.0015/call
	ToolReactions = "treg.linkedin.post.reactions" // routed, $0.0015/call
	fetchLimit    = 100
)

var (
	ErrDisabled = errors.New("engagers: treg not configured")
	ErrNoData   = errors.New("engagers: treg returned nothing for this post")
)

// PostRef identifies a LinkedIn post the way treg's routed tools accept it.
type PostRef struct {
	URN, ID string
}

func (p PostRef) Body() map[string]any {
	b := map[string]any{"post_urn": p.URN}
	if p.ID != "" {
		b["post_id"] = p.ID
	}
	return b
}

var (
	urnRe      = regexp.MustCompile(`urn:li:(activity|ugcPost|share):(\d{1,25})`)
	activityRe = regexp.MustCompile(`activity-(\d{1,25})`)
)

// ParsePostRef accepts an https linkedin.com post URL or a bare post URN.
func ParsePostRef(raw string) (PostRef, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "urn:li:") {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || !(u.Host == "linkedin.com" || strings.HasSuffix(u.Host, ".linkedin.com")) {
			return PostRef{}, errors.New("post_url must be an https://www.linkedin.com post URL")
		}
		raw = u.Path
	}
	if m := urnRe.FindStringSubmatch(raw); m != nil {
		ref := PostRef{URN: m[0]}
		if m[1] == "activity" {
			ref.ID = m[2]
		}
		return ref, nil
	}
	if m := activityRe.FindStringSubmatch(raw); m != nil {
		return PostRef{URN: "urn:li:activity:" + m[1], ID: m[1]}, nil
	}
	return PostRef{}, errors.New("could not find a post id in post_url")
}

// ValidKinds dedupes kinds; empty means both.
func ValidKinds(kinds []string) ([]string, error) {
	if len(kinds) == 0 {
		return []string{KindComments, KindReactions}, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, k := range kinds {
		if k != KindComments && k != KindReactions {
			return nil, fmt.Errorf("invalid kind %q (comments, reactions)", k)
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out, nil
}

// Engager is one person who commented on or reacted to the post.
type Engager struct {
	Name       string `json:"name"`
	Headline   string `json:"headline,omitempty"`
	ProfileURL string `json:"profile_url,omitempty"`
	PublicID   string `json:"public_id,omitempty"`
	Kind       string `json:"kind"` // comment | reaction (comment wins when both)
	Comment    string `json:"comment,omitempty"`
	Reaction   string `json:"reaction,omitempty"`
	Title      string `json:"title,omitempty"`
	Company    string `json:"company,omitempty"`
}

func (e Engager) key() string {
	if e.PublicID != "" {
		return "id:" + e.PublicID
	}
	return "url:" + strings.TrimRight(e.ProfileURL, "/")
}

// Caller is the slice of *treg.Client used here.
type Caller interface {
	Enabled() bool
	Call(ctx context.Context, req treg.Request) (*treg.Response, error)
}

// Fetch pulls each kind; one kind failing is tolerated, all failing is not.
func Fetch(ctx context.Context, c Caller, ws string, ref PostRef, kinds []string) ([]Engager, error) {
	if c == nil || !c.Enabled() {
		return nil, ErrDisabled
	}
	var lists [][]Engager
	var lastErr error
	for _, kind := range kinds {
		tool := ToolComments
		if kind == KindReactions {
			tool = ToolReactions
		}
		body := ref.Body()
		body["limit"] = fetchLimit
		resp, err := c.Call(ctx, treg.Request{WorkspaceID: ws, Tool: tool, Body: body})
		if err != nil {
			lastErr = err
			continue
		}
		lists = append(lists, parseEngagers(kind, resp.Body))
	}
	if len(lists) == 0 {
		return nil, errors.Join(ErrNoData, lastErr)
	}
	return merge(lists...), nil
}

func parseEngagers(kind string, body []byte) []Engager {
	var v map[string]any
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	if o, ok := v["output"].(map[string]any); ok {
		v = o
	}
	rows, _ := v[kind].([]any)
	var out []Engager
	for _, r := range rows {
		row, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if kind == KindComments {
			out = append(out, commentEngagers(row)...)
			continue
		}
		if e, ok := person(row, "actor", "author", "user"); ok {
			e.Kind, e.Reaction = "reaction", s(row, "reactionType", "reaction_type", "type")
			out = append(out, e)
		}
	}
	return out
}

func commentEngagers(row map[string]any) []Engager {
	var out []Engager
	if e, ok := person(row, "author", "commenter", "actor"); ok {
		e.Kind, e.Comment = "comment", s(row, "text", "comment", "content")
		out = append(out, e)
	}
	replies, _ := row["replies"].([]any)
	for _, r := range replies {
		if rm, ok := r.(map[string]any); ok {
			out = append(out, commentEngagers(rm)...)
		}
	}
	return out
}

func person(row map[string]any, keys ...string) (Engager, bool) {
	for _, k := range keys {
		a, ok := row[k].(map[string]any)
		if !ok {
			continue
		}
		e := Engager{
			Name:       s(a, "name", "fullName", "full_name"),
			Headline:   s(a, "headline", "occupation", "title"),
			ProfileURL: s(a, "profileUrl", "profile_url", "linkedinUrl", "url"),
			PublicID:   s(a, "publicId", "public_identifier", "publicIdentifier", "username"),
		}
		if e.ProfileURL == "" && e.PublicID != "" {
			e.ProfileURL = "https://www.linkedin.com/in/" + e.PublicID
		}
		e.Title, e.Company = splitHeadline(e.Headline)
		return e, e.ProfileURL != "" && e.Name != ""
	}
	return Engager{}, false
}

// merge keeps one row per person, first-seen order; a comment beats a reaction.
func merge(lists ...[]Engager) []Engager {
	idx := map[string]int{}
	var out []Engager
	for _, l := range lists {
		for _, e := range l {
			i, seen := idx[e.key()]
			if !seen {
				idx[e.key()] = len(out)
				out = append(out, e)
				continue
			}
			cur := out[i]
			if cur.Comment == "" && e.Comment != "" {
				cur.Comment, cur.Kind = e.Comment, "comment"
			}
			if cur.Reaction == "" {
				cur.Reaction = e.Reaction
			}
			out[i] = cur
		}
	}
	return out
}

// splitHeadline reads "Title at Company | more" / "Title @ Company".
func splitHeadline(h string) (title, company string) {
	h = strings.TrimSpace(h)
	if i := strings.IndexAny(h, "|·•"); i >= 0 {
		h = strings.TrimSpace(h[:i])
	}
	for _, sep := range []string{" at ", " @ "} {
		if i := strings.Index(h, sep); i > 0 {
			return strings.TrimSpace(h[:i]), strings.TrimSpace(h[i+len(sep):])
		}
	}
	return h, ""
}

func s(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
