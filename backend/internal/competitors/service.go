package competitors

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"leadecho/internal/ai"
	"leadecho/internal/database"
	"leadecho/internal/monitor"
	"leadecho/internal/onboarding"
)

const (
	// MaxSources is the most sources one competitor can have.
	MaxSources = 10
	// DefaultLimit is how many competitors a workspace may watch when no
	// plan limit is wired in (self-host).
	DefaultLimit = 10
	// FreeLimit is the cap on the free plan.
	FreeLimit = 3

	// MaxAttempts is how many times the model step tries an item before it
	// is dropped.
	MaxAttempts = 3

	reasonCredits = "competitor_watch"
	modelCost     = 1 // credits per model call, as for one scored post
)

// SearchFunc looks a company up on the web (Exa company search).
type SearchFunc func(ctx context.Context, query string) ([]monitor.ExaWebResult, error)

// PostFetcher returns a company's recent posts on x or linkedin. It returns
// ErrLocked when the workspace has no paid data.
type PostFetcher interface {
	CompanyPosts(ctx context.Context, wsID, kind, profileURL string) ([]Item, error)
}

// Model is the chat model.
type Model interface {
	Chat(ctx context.Context, workspaceID string, msgs []ai.ChatMessage) (string, error)
}

// Credits charges a workspace for a model call, and gives the credit back
// when the call produced nothing usable.
type Credits interface {
	Spend(ctx context.Context, ws string, cost int, reason string) error
	Refund(ctx context.Context, ws string, n int, reason string)
}

// Service finds a workspace's competitors, watches their sources and turns
// what they publish into moves. Nil optional fields switch that part off.
type Service struct {
	Q      database.Querier
	Fetch  onboarding.PageFetcher
	Model  Model
	Logger zerolog.Logger

	Search  SearchFunc                                                // nil: names cannot be resolved to websites
	Posts   PostFetcher                                               // nil: x and linkedin are not polled
	Credits Credits                                                   // nil: model calls are free
	Themes  func(ctx context.Context, ws string) ([]Theme, error)     // nil: no demand themes
	Limit   func(ctx context.Context, ws string) int                  // nil: DefaultLimit
	Locked  func(ctx context.Context, ws string) bool                 // nil: paid data is available
	Alert   func(ctx context.Context, ws, competitor string, m Alert) // nil: no alerts
	// Tx runs fn in one database transaction. nil: fn runs on Q, unprotected
	// (tests only; NewService always sets it).
	Tx func(ctx context.Context, fn func(q database.Querier) error) error
}

func (s *Service) tx(ctx context.Context, fn func(q database.Querier) error) error {
	if s.Tx == nil {
		return fn(s.Q)
	}
	return s.Tx(ctx, fn)
}

// OwnHost is the host of the customer's own site, or "" when unknown.
func (s *Service) OwnHost(ctx context.Context, ws string) string {
	bp, err := s.Q.GetBrandProfile(ctx, ws)
	if err != nil {
		return ""
	}
	return onboarding.HostOf(bp.SourceUrl)
}

// IsOwnSite reports whether website is the customer's own site.
func IsOwnSite(website, ownHost string) bool {
	host := onboarding.HostOf(website)
	return ownHost != "" && (host == ownHost || strings.HasSuffix(host, "."+ownHost))
}

// Alert is a high-importance move sent to the workspace's alert channels.
type Alert struct {
	Kind, Title, Impact, Action, URL string
}

// PlanLimit is the competitor cap for a hosted plan.
func PlanLimit(plan string) int {
	if plan == "free" {
		return FreeLimit
	}
	return DefaultLimit
}

func (s *Service) limit(ctx context.Context, ws string) int {
	if s.Limit == nil {
		return DefaultLimit
	}
	return s.Limit(ctx, ws)
}

// PaidLocked reports whether x and linkedin sources are locked for ws.
func (s *Service) PaidLocked(ctx context.Context, ws string) bool {
	return s.Posts == nil || (s.Locked != nil && s.Locked(ctx, ws))
}

func squash(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

// PickWebsite chooses a competitor's own site from search hits, or "" when
// no hit is clearly theirs. Nobody confirms the choice, so a hit counts only
// if its domain is the name, or it is a homepage whose title leads with the
// name (a page titled "Acme: an honest review" on someone's blog is not
// Acme's site). Review sites and the customer's own site never count.
func PickWebsite(name, ownHost string, hits []monitor.ExaWebResult) string {
	want := squash(name)
	if want == "" {
		return ""
	}
	for _, h := range hits {
		u, err := url.Parse(strings.TrimSpace(h.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}
		host := onboarding.HostOf(h.URL)
		if onboarding.IsAggregatorHost(host) || IsOwnSite(h.URL, ownHost) {
			continue
		}
		labels := strings.Split(host, ".")
		domain := labels[0]
		if len(labels) >= 2 {
			domain = labels[len(labels)-2]
		}
		title := h.Title
		if i := strings.IndexAny(title, "-|:–—·"); i >= 0 {
			title = title[:i]
		}
		homepage := strings.Trim(u.Path, "/") == ""
		if squash(domain) == want || (homepage && squash(title) == want) {
			return u.Scheme + "://" + u.Host
		}
	}
	return ""
}

// guessTLDs are tried, in order, when search finds nothing.
var guessTLDs = []string{".com", ".io", ".ai", ".co"}

// ErrSearchDown: the website search failed, so "not found" is not final.
var ErrSearchDown = errors.New("website search failed")

// Resolve finds a competitor's website from its name, or "" when it cannot
// tell with confidence. It returns ErrSearchDown when it found nothing and
// the search itself had failed: the caller should try again later.
// Search first; when that finds nothing (or no search
// is configured) it tries the obvious domains and accepts one only if the
// page there names the company in its title.
func (s *Service) Resolve(ctx context.Context, name, ownHost string) (string, error) {
	var searchErr error
	if s.Search != nil {
		hits, err := s.Search(ctx, name)
		if err != nil {
			s.Logger.Warn().Err(err).Str("competitor", name).Msg("competitors: website search failed")
			searchErr = fmt.Errorf("%w: %v", ErrSearchDown, err)
		}
		if site := PickWebsite(name, ownHost, hits); site != "" {
			return site, nil
		}
	}
	want := squash(name)
	if len(want) < 3 || s.Fetch == nil {
		return "", searchErr
	}
	for _, tld := range guessTLDs {
		host := want + tld
		if host == ownHost {
			continue
		}
		fctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		body, err := s.Fetch(fctx, "https://"+host)
		cancel()
		if err != nil {
			if strings.Contains(err.Error(), "status ") {
				// A site answers there but will not let us read it (a bot
				// wall, usually). It is probably theirs; a lookalike on the
				// next ending is not, so stop instead of guessing on.
				return "", searchErr
			}
			continue // no such host: try the next ending
		}
		if titleNames(onboarding.PageTitle(body), want) {
			return "https://" + host, nil
		}
	}
	return "", searchErr
}

// titleNames reports whether one part of a page title is exactly the name
// ("HackerRank - Online Coding Tests", "Hiring platform | Codility"). A title
// that only contains it ("iMocha.com for sale", "leetcode.ai") does not
// count: parked and lookalike domains do that.
func titleNames(title, want string) bool {
	for _, part := range strings.FieldsFunc(title, func(r rune) bool { return strings.ContainsRune("|:–—·-", r) }) {
		if squash(part) == want {
			return true
		}
	}
	return false
}

// Setup creates the workspace's competitors from the names onboarding stored
// in its brand profile: each name is resolved to a website and its sources
// are read off the homepage. The customer is not asked for anything. Names
// with no confident website are recorded as unresolved and not watched.
// Setup is recorded even when nothing was found, so it runs once. The one
// exception is a search outage: then it returns an error without recording
// anything, and the next round tries again.
func (s *Service) Setup(ctx context.Context, ws string) (created int, unresolved []string, err error) {
	bp, err := s.Q.GetBrandProfile(ctx, ws)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fmt.Errorf("load brand profile: %w", err)
	}
	have, err := s.Q.ListCompetitors(ctx, ws)
	if err != nil {
		return 0, nil, fmt.Errorf("list competitors: %w", err)
	}
	known := map[string]bool{}
	for _, c := range have {
		known[strings.ToLower(c.Name)] = true
	}
	room := s.limit(ctx, ws) - len(have)
	ownHost := onboarding.HostOf(bp.SourceUrl)
	unresolved = []string{}
	searchDown := false
	product := squash(bp.Product)
	for _, name := range bp.Competitors {
		name = strings.TrimSpace(name)
		if name == "" || known[strings.ToLower(name)] || (product != "" && squash(name) == product) {
			continue // blank, already watched, or the customer's own product
		}
		if room <= 0 {
			break
		}
		site, err := s.Resolve(ctx, name, ownHost)
		searchDown = searchDown || errors.Is(err, ErrSearchDown)
		if site == "" {
			unresolved = append(unresolved, name)
			continue
		}
		if _, err := s.Add(ctx, ws, name, site); err != nil {
			s.Logger.Warn().Err(err).Str("workspace_id", ws).Str("competitor", name).Msg("competitors: add failed")
			unresolved = append(unresolved, name)
			continue
		}
		known[strings.ToLower(name)] = true
		created++
		room--
	}
	if searchDown {
		return created, unresolved, fmt.Errorf("setup for %s not finished: %w", ws, ErrSearchDown)
	}
	if err := s.Q.MarkCompetitorSetup(ctx, database.MarkCompetitorSetupParams{WorkspaceID: ws, Unresolved: unresolved}); err != nil {
		return created, unresolved, fmt.Errorf("mark setup: %w", err)
	}
	return created, unresolved, nil
}

var (
	// ErrLimit: the workspace already watches as many competitors as its plan allows.
	ErrLimit = errors.New("competitor limit reached")
	// ErrExists: a competitor with that name is already watched.
	ErrExists = errors.New("competitor already watched")
)

// Add starts watching one competitor: it stores it and the sources found on
// its homepage. website must already be a validated http(s) URL.
func (s *Service) Add(ctx context.Context, ws, name, website string) (database.Competitor, error) {
	n, err := s.Q.CountCompetitors(ctx, ws)
	if err != nil {
		return database.Competitor{}, fmt.Errorf("count competitors: %w", err)
	}
	if _, err := s.Q.GetCompetitorByName(ctx, database.GetCompetitorByNameParams{WorkspaceID: ws, Name: clip(name, 80)}); err == nil {
		return database.Competitor{}, ErrExists
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return database.Competitor{}, fmt.Errorf("look up competitor: %w", err)
	}
	if int(n) >= s.limit(ctx, ws) {
		return database.Competitor{}, ErrLimit
	}
	c, err := s.Q.CreateCompetitor(ctx, database.CreateCompetitorParams{WorkspaceID: ws, Name: clip(name, 80), Website: website})
	if err != nil {
		return database.Competitor{}, fmt.Errorf("create competitor: %w", err)
	}
	if err := s.Rediscover(ctx, c); err != nil {
		return c, err
	}
	return c, nil
}

// Rediscover reads the competitor's homepage again and adds any source not
// stored yet. Existing sources, and what was learned from them, are kept.
func (s *Service) Rediscover(ctx context.Context, c database.Competitor) error {
	if c.Website == "" {
		return nil
	}
	fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	for i, src := range Discover(fctx, s.Fetch, c.Website) {
		if i >= MaxSources {
			break
		}
		if _, err := s.Q.CreateCompetitorSource(ctx, database.CreateCompetitorSourceParams{
			CompetitorID: c.ID, WorkspaceID: c.WorkspaceID, Kind: src.Kind, Url: src.URL,
		}); err != nil {
			return fmt.Errorf("store source: %w", err)
		}
	}
	return nil
}
