package competitors

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/billing"
	"leadecho/internal/database"
	"leadecho/internal/demand"
	"leadecho/internal/monitor"
	"leadecho/internal/onboarding"
	"leadecho/internal/treg"
)

// NewService wires the service to the running system: the SSRF-guarded
// fetcher, Exa company search, treg posts through the monitor, demand
// themes, credits and alert webhooks. mon, credits and exaKey may be empty.
func NewService(pool *pgxpool.Pool, q *database.Queries, model Model, mon *monitor.Monitor, credits *billing.Credits, exaKey string, logger zerolog.Logger) *Service {
	s := &Service{
		Q: q, Fetch: onboarding.SafeFetcher(15 * time.Second), Model: model, Logger: logger,
		Themes: demandThemes(demand.DBStore{Q: q}),
	}
	if pool != nil {
		s.Tx = func(ctx context.Context, fn func(database.Querier) error) error {
			tx, err := pool.Begin(ctx)
			if err != nil {
				return fmt.Errorf("begin: %w", err)
			}
			defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
			if err := fn(q.WithTx(tx)); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
	}
	if exaKey != "" {
		s.Search = func(ctx context.Context, query string) ([]monitor.ExaWebResult, error) {
			return monitor.ExaWebSearch(ctx, "", exaKey, query, "company", 8)
		}
	}
	if mon != nil && mon.Treg().Enabled() {
		s.Posts = monitorPosts{mon}
	}
	if mon != nil { // alerts are webhooks: they need no paid data
		s.Alert = func(ctx context.Context, ws, competitor string, a Alert) {
			slack, discord := AlertText(competitor, a)
			mon.NotifyText(ctx, ws, slack, discord)
		}
	}
	if credits.Enforced() {
		s.Credits = credits
		s.Locked = func(ctx context.Context, ws string) bool { // X and LinkedIn sources
			l := credits.LockedPlatforms(ctx, ws)
			return l["twitter"] || l["linkedin"]
		}
		s.Limit = func(ctx context.Context, ws string) int {
			plan, err := q.GetWorkspacePlan(ctx, ws)
			if err != nil {
				return FreeLimit
			}
			return PlanLimit(plan)
		}
	}
	return s
}

var (
	// Slack reads <...> as links and pings; these three escapes disarm it.
	slackEscape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	// A zero-width space after @ keeps Discord from pinging @everyone / @here.
	discordEscape = strings.NewReplacer("@", "@\u200b")
)

// AlertText renders a high-importance move for Slack and for Discord. The
// words come from a competitor's own pages by way of a model, so they are
// escaped: they must not be able to ping a channel or plant a link.
func AlertText(competitor string, a Alert) (slack, discord string) {
	body := fmt.Sprintf("%s\nFor you: %s\nDo: %s", a.Title, a.Impact, a.Action)
	slack = fmt.Sprintf("*Competitor move: %s (%s)*\n%s", slackEscape.Replace(competitor), a.Kind, slackEscape.Replace(body))
	discord = fmt.Sprintf("**Competitor move: %s (%s)**\n%s", discordEscape.Replace(competitor), a.Kind, discordEscape.Replace(body))
	if u, err := url.Parse(a.URL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && !strings.ContainsAny(a.URL, "<>| \n") {
		slack += "\n<" + a.URL + "|Source>"
		discord += "\n" + a.URL
	}
	return slack, discord
}

type monitorPosts struct{ m *monitor.Monitor }

// CompanyPosts maps "this workspace may not spend paid data right now" to
// ErrLocked, so the source waits a day and is not counted as failing.
func (p monitorPosts) CompanyPosts(ctx context.Context, ws, kind, profileURL string) ([]Item, error) {
	posts, err := p.m.CompanyPosts(ctx, ws, kind, profileURL)
	if err != nil {
		if errors.Is(err, billing.ErrNoPaidData) || errors.Is(err, billing.ErrPaidDataPaused) || errors.Is(err, billing.ErrNoCredits) ||
			errors.Is(err, treg.ErrCustomerDailyCap) || errors.Is(err, monitor.ErrTregOff) || treg.IsOperatorError(err) {
			return nil, ErrLocked
		}
		return nil, err
	}
	items := make([]Item, len(posts))
	for i, post := range posts {
		items[i] = Item{Key: post.ID, Body: post.Text, URL: post.URL, At: post.At}
	}
	return items, nil
}

// demandThemes reads the workspace's open demand themes.
func demandThemes(store demand.DBStore) func(ctx context.Context, ws string) ([]Theme, error) {
	return func(ctx context.Context, ws string) ([]Theme, error) {
		state, err := store.LoadState(ctx, ws)
		if err != nil {
			return nil, err
		}
		var out []Theme
		for _, t := range state.Themes {
			if t.Status == demand.StatusShipped {
				continue
			}
			out = append(out, Theme{ID: t.ID, Title: t.Title, Coverage: t.Coverage, Competitors: t.Competitors, People: t.People()})
		}
		return out, nil
	}
}
