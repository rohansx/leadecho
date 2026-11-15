package competitors

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"leadecho/internal/database"
)

const (
	viewMovesWindow = 90 * 24 * time.Hour
	peopleWindow    = 60 * 24 * time.Hour
	maxPeople       = 50
	maxSuggestions  = 5
	// minSuggestPeople: a product is suggested once themes naming it were
	// raised by at least this many people.
	minSuggestPeople = 3
)

// SourceView is one source as the page shows it.
type SourceView struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	URL       string     `json:"url"`
	Broken    bool       `json:"broken"`
	Locked    bool       `json:"locked"` // needs paid data the workspace does not have
	CheckedAt *time.Time `json:"last_checked_at"`
}

// MoveView is one move as the page shows it.
type MoveView struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Summary    string    `json:"summary"`
	Impact     string    `json:"impact"`
	Action     string    `json:"action"`
	URLs       []string  `json:"urls"`
	ThemeID    string    `json:"theme_id"`
	ThemeTitle string    `json:"theme_title"`
	Importance string    `json:"importance"`
	At         time.Time `json:"at"`
}

// CompetitorView is one competitor with its sources and recent moves.
type CompetitorView struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Website string       `json:"website"`
	Status  string       `json:"status"`
	Summary string       `json:"summary"`
	Sources []SourceView `json:"sources"`
	Moves   []MoveView   `json:"moves"`
	People  int          `json:"people"` // unhappy with, or leaving, this competitor
}

// View is everything the Competitors page shows.
type View struct {
	Competitors []CompetitorView `json:"data"`
	Unresolved  []string         `json:"unresolved"`  // names whose website could not be found
	Suggestions []string         `json:"suggestions"` // products people keep naming, not watched yet
	Limit       int              `json:"limit"`
	SetUp       bool             `json:"set_up"` // automatic setup has run
	PaidLocked  bool             `json:"paid_locked"`
}

// NamePattern returns a case-insensitive Postgres regular expression that
// matches name as a whole word, or "" for a name too short to match safely.
//
// ponytail: whole-word match on the name. Names that are also ordinary
// words ("Notion", "Linear") will over-match; the fix is to have the scorer
// record which competitor a complaint is about.
func NamePattern(name string) string {
	name = strings.TrimSpace(name)
	if len([]rune(name)) < 3 {
		return ""
	}
	return `\m` + regexp.QuoteMeta(name) + `\M`
}

// Load builds the page view for a workspace.
func (s *Service) Load(ctx context.Context, ws string) (View, error) {
	v := View{Competitors: []CompetitorView{}, Unresolved: []string{}, Suggestions: []string{}, Limit: s.limit(ctx, ws), PaidLocked: s.PaidLocked(ctx, ws)}
	comps, err := s.Q.ListCompetitors(ctx, ws)
	if err != nil {
		return v, fmt.Errorf("list competitors: %w", err)
	}
	srcs, err := s.Q.ListCompetitorSources(ctx, ws)
	if err != nil {
		return v, fmt.Errorf("list sources: %w", err)
	}
	moves, err := s.Q.ListCompetitorMoves(ctx, database.ListCompetitorMovesParams{WorkspaceID: ws, Since: time.Now().Add(-viewMovesWindow)})
	if err != nil {
		return v, fmt.Errorf("list moves: %w", err)
	}
	var themes []Theme
	if s.Themes != nil {
		if themes, err = s.Themes(ctx, ws); err != nil {
			s.Logger.Warn().Err(err).Str("workspace_id", ws).Msg("competitors: load demand themes")
		}
	}
	themeTitle := map[string]string{}
	for _, t := range themes {
		themeTitle[t.ID] = t.Title
	}

	index := map[string]int{}
	watched := map[string]bool{}
	since := time.Now().Add(-peopleWindow)
	for _, c := range comps {
		cv := CompetitorView{ID: c.ID, Name: c.Name, Website: c.Website, Status: c.Status, Summary: c.Summary, Sources: []SourceView{}, Moves: []MoveView{}}
		if pat := NamePattern(c.Name); pat != "" {
			n, err := s.Q.CountCompetitorPeople(ctx, database.CountCompetitorPeopleParams{WorkspaceID: ws, Since: since, NamePattern: pat})
			if err != nil {
				return v, fmt.Errorf("count people: %w", err)
			}
			cv.People = int(n)
		}
		index[c.ID] = len(v.Competitors)
		watched[squash(c.Name)] = true
		v.Competitors = append(v.Competitors, cv)
	}
	for _, src := range srcs {
		i, ok := index[src.CompetitorID]
		if !ok {
			continue
		}
		sv := SourceView{ID: src.ID, Kind: src.Kind, URL: src.Url, Broken: src.Broken, Locked: Paid(src.Kind) && v.PaidLocked}
		if src.LastCheckedAt.Valid {
			t := src.LastCheckedAt.Time
			sv.CheckedAt = &t
		}
		v.Competitors[i].Sources = append(v.Competitors[i].Sources, sv)
	}
	for _, m := range moves {
		i, ok := index[m.CompetitorID]
		if !ok {
			continue
		}
		v.Competitors[i].Moves = append(v.Competitors[i].Moves, MoveView{
			ID: m.ID, Kind: m.Kind, Title: m.Title, Summary: m.Summary, Impact: m.Impact, Action: m.Action, URLs: m.Urls,
			ThemeID: m.ThemeID, ThemeTitle: themeTitle[m.ThemeID], Importance: m.Importance, At: m.HappenedAt,
		})
	}

	setup, err := s.Q.GetCompetitorSetup(ctx, ws)
	switch {
	case err == nil:
		v.SetUp = true
		for _, name := range setup.Unresolved {
			if !watched[squash(name)] {
				v.Unresolved = append(v.Unresolved, name)
			}
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return v, fmt.Errorf("load setup: %w", err)
	}
	for _, name := range v.Unresolved {
		watched[squash(name)] = true // already listed; not also a suggestion
	}
	v.Suggestions = Suggest(themes, watched)
	return v, nil
}

// Suggest returns products people keep naming in demand themes that are not
// watched yet, most-named first. skip holds squashed names to leave out.
func Suggest(themes []Theme, skip map[string]bool) []string {
	people := map[string]int{}
	display := map[string]string{}
	for _, t := range themes {
		for _, name := range t.Competitors {
			key := squash(name)
			if key == "" || skip[key] {
				continue
			}
			people[key] += t.People
			if display[key] == "" {
				display[key] = strings.TrimSpace(name)
			}
		}
	}
	var keys []string
	for k, n := range people {
		if n >= minSuggestPeople {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if people[keys[i]] != people[keys[j]] {
			return people[keys[i]] > people[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > maxSuggestions {
		keys = keys[:maxSuggestions]
	}
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = display[k]
	}
	return out
}

// Person is someone complaining about, or looking to leave, a competitor.
type Person struct {
	MentionID string    `json:"mention_id"`
	Platform  string    `json:"platform"`
	URL       string    `json:"url"`
	Author    string    `json:"author"`
	Quote     string    `json:"quote"`
	Tags      []string  `json:"tags"`
	At        time.Time `json:"at"`
}

// People lists who in the workspace's feed is unhappy with a competitor.
func (s *Service) People(ctx context.Context, ws, competitorID string) ([]Person, error) {
	c, err := s.Q.GetCompetitor(ctx, database.GetCompetitorParams{ID: competitorID, WorkspaceID: ws})
	if err != nil {
		return nil, err
	}
	out := []Person{}
	pat := NamePattern(c.Name)
	if pat == "" {
		return out, nil
	}
	rows, err := s.Q.ListCompetitorPeople(ctx, database.ListCompetitorPeopleParams{
		WorkspaceID: ws, Since: time.Now().Add(-peopleWindow), NamePattern: pat, MaxRows: maxPeople,
	})
	if err != nil {
		return nil, fmt.Errorf("list people: %w", err)
	}
	for _, r := range rows {
		out = append(out, Person{
			MentionID: r.ID, Platform: r.Platform, URL: r.Url, Author: r.AuthorUsername.String,
			Quote: clip(strings.Join(strings.Fields(r.Content), " "), 280), Tags: r.IntentTags, At: r.PostedAt,
		})
	}
	return out, nil
}
