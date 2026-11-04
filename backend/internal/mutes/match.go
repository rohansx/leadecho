// Package mutes holds workspace mute lists (authors, domains, keywords,
// subreddits) and the pure matching used to drop muted mentions before insert.
package mutes

import (
	"errors"
	"net/url"
	"strings"
)

const (
	KindAuthor    = "author"
	KindDomain    = "domain"
	KindKeyword   = "keyword"
	KindSubreddit = "subreddit"

	maxValueLen = 200
)

// ValidKinds mirrors the mutes.kind CHECK constraint.
var ValidKinds = map[string]bool{KindAuthor: true, KindDomain: true, KindKeyword: true, KindSubreddit: true}

// Mute is one mute rule.
type Mute struct {
	Kind  string
	Value string
}

// Candidate is the subset of a mention a mute can match on.
type Candidate struct {
	Platform  string
	Author    string
	URL       string
	Title     string
	Content   string
	Subreddit string
}

// Normalize validates and canonicalises a mute value for storage and matching:
// lowercased, trimmed, and stripped of @ / u/ / r/ prefixes and URL parts.
func Normalize(kind, value string) (string, error) {
	if !ValidKinds[kind] {
		return "", errors.New("kind must be one of author, domain, keyword, subreddit")
	}
	v := strings.ToLower(strings.TrimSpace(value))
	switch kind {
	case KindAuthor:
		v = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(v, "/"), "u/"), "@")
	case KindSubreddit:
		v = strings.TrimPrefix(strings.TrimPrefix(v, "/"), "r/")
	case KindDomain:
		v = hostOf(v)
		v = strings.TrimPrefix(v, ".")
	}
	if v == "" {
		return "", errors.New("value is required")
	}
	if len(v) > maxValueLen {
		return "", errors.New("value too long")
	}
	return v, nil
}

// hostOf returns the lowercased host of a URL or bare domain ("" if unparsable).
func hostOf(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Match reports whether any mute applies to the candidate. Values are
// normalised here too, so rules stored before normalisation still work.
func Match(list []Mute, c Candidate) bool {
	for _, m := range list {
		v, err := Normalize(m.Kind, m.Value)
		if err != nil {
			continue
		}
		if matchOne(m.Kind, v, c) {
			return true
		}
	}
	return false
}

func matchOne(kind, v string, c Candidate) bool {
	switch kind {
	case KindAuthor:
		a, _ := Normalize(KindAuthor, c.Author)
		return a != "" && a == v
	case KindDomain:
		h := hostOf(c.URL)
		return h != "" && (h == v || strings.HasSuffix(h, "."+v))
	case KindKeyword:
		return strings.Contains(strings.ToLower(c.Title+"\n"+c.Content), v)
	case KindSubreddit:
		s, _ := Normalize(KindSubreddit, c.Subreddit)
		return c.Platform == "reddit" && s != "" && s == v
	}
	return false
}
