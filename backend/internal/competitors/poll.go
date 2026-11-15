package competitors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

const (
	// CheckEvery is how often one source is checked.
	CheckEvery = 24 * time.Hour

	fetchTimeout      = 40 * time.Second
	setupPerPoll      = 2
	sourcesPerPoll    = 40
	itemsPerPoll      = 200
	itemsPerCall      = 20
	alertsPerPoll     = 20
	maxItemAge        = 30 * 24 * time.Hour // an older entry showing up in a feed is history, not news
	giveUpAfter       = 3 * 24 * time.Hour  // unprocessed this long: stop retrying
	recentMovesWindow = 14 * 24 * time.Hour
	summaryWindow     = 90 * 24 * time.Hour
)

// Poll runs one round: set up workspaces that have no competitors yet, check
// the sources that are due, turn new items into moves, send alerts.
//
// ponytail: serial and bounded per round (setupPerPoll, sourcesPerPoll,
// itemsPerPoll). With a 24h cadence that covers a few thousand sources at a
// tick every few minutes; a worker pool is the upgrade if it falls behind.
func (s *Service) Poll(ctx context.Context) {
	s.setupPending(ctx)
	s.checkDue(ctx)
	s.processItems(ctx)
	s.sendAlerts(ctx)
}

func (s *Service) setupPending(ctx context.Context) {
	wss, err := s.Q.ListWorkspacesNeedingCompetitorSetup(ctx, setupPerPoll)
	if err != nil {
		s.Logger.Error().Err(err).Msg("competitors: list workspaces to set up")
		return
	}
	for _, ws := range wss {
		if ctx.Err() != nil {
			return
		}
		created, unresolved, err := s.Setup(ctx, ws)
		if err != nil {
			s.Logger.Error().Err(err).Str("workspace_id", ws).Msg("competitors: setup")
			continue
		}
		s.Logger.Info().Str("workspace_id", ws).Int("created", created).Strs("unresolved", unresolved).Msg("competitors: set up")
	}
}

func (s *Service) checkDue(ctx context.Context) {
	due, err := s.Q.ListDueCompetitorSources(ctx, database.ListDueCompetitorSourcesParams{
		Before: time.Now().Add(-CheckEvery), MaxRows: sourcesPerPoll,
	})
	if err != nil {
		s.Logger.Error().Err(err).Msg("competitors: list due sources")
		return
	}
	for _, src := range due {
		if ctx.Err() != nil {
			return
		}
		if err := s.CheckSource(ctx, src); err != nil {
			s.Logger.Warn().Err(err).Str("source_id", src.ID).Str("kind", src.Kind).Str("url", src.Url).Msg("competitors: check source")
		}
	}
}

// CheckSource fetches one source and stores what is new. The first
// successful check is a baseline: what it finds is recorded as already
// seen and never becomes a move.
func (s *Service) CheckSource(ctx context.Context, src database.CompetitorSource) error {
	fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	var got Fetched
	var err error
	if Paid(src.Kind) {
		if s.PaidLocked(ctx, src.WorkspaceID) {
			return s.Q.TouchCompetitorSource(ctx, src.ID)
		}
		got.Hash = feedBaseline
		got.Items, err = s.Posts.CompanyPosts(fctx, src.WorkspaceID, src.Kind, src.Url)
		if errors.Is(err, ErrLocked) {
			return s.Q.TouchCompetitorSource(ctx, src.ID)
		}
	} else {
		got, err = FetchURL(fctx, s.Fetch, src.Kind, src.Url, src.LastHash, src.LastText)
	}
	if err != nil {
		if ferr := s.Q.MarkCompetitorSourceFailed(ctx, src.ID); ferr != nil {
			return fmt.Errorf("mark failed: %w", ferr)
		}
		return err
	}

	baseline := src.LastHash == ""
	now := time.Now().UTC()
	for _, it := range got.Items {
		if it.Key == "" {
			continue
		}
		at := it.At
		if at.IsZero() {
			at = now
		}
		seen := pgtype.Timestamptz{}
		if baseline || now.Sub(at) > maxItemAge {
			seen = pgtype.Timestamptz{Time: now, Valid: true}
		}
		if _, err := s.Q.InsertCompetitorItem(ctx, database.InsertCompetitorItemParams{
			SourceID: src.ID, CompetitorID: src.CompetitorID, WorkspaceID: src.WorkspaceID,
			ItemKey: clip(it.Key, 500), Title: clip(it.Title, 300), Body: clip(it.Body, maxItemBody*2+100),
			Url: clip(it.URL, 1000), HappenedAt: at, ProcessedAt: seen,
		}); err != nil {
			return fmt.Errorf("store item: %w", err)
		}
	}
	return s.Q.MarkCompetitorSourceChecked(ctx, database.MarkCompetitorSourceCheckedParams{ID: src.ID, LastHash: got.Hash, LastText: got.Text})
}

func (s *Service) processItems(ctx context.Context) {
	items, err := s.Q.ListUnprocessedCompetitorItems(ctx, itemsPerPoll)
	if err != nil {
		s.Logger.Error().Err(err).Msg("competitors: list new items")
		return
	}
	byComp := map[string][]database.CompetitorItem{}
	var order []string
	for _, it := range items {
		if _, ok := byComp[it.CompetitorID]; !ok {
			order = append(order, it.CompetitorID)
		}
		byComp[it.CompetitorID] = append(byComp[it.CompetitorID], it)
	}
	for _, id := range order {
		if ctx.Err() != nil {
			return
		}
		batch := byComp[id]
		if len(batch) > itemsPerCall {
			batch = batch[:itemsPerCall] // the rest next round
		}
		if err := s.processCompetitor(ctx, id, batch); err != nil {
			s.Logger.Warn().Err(err).Str("competitor_id", id).Msg("competitors: read moves")
			s.giveUpOld(ctx, batch)
		}
	}
}

// giveUpOld stops retrying items the model step has failed on for days.
func (s *Service) giveUpOld(ctx context.Context, batch []database.CompetitorItem) {
	var old []string
	for _, it := range batch {
		if time.Since(it.CreatedAt) > giveUpAfter {
			old = append(old, it.ID)
		}
	}
	if len(old) == 0 {
		return
	}
	if err := s.Q.MarkCompetitorItemsProcessed(ctx, old); err != nil {
		s.Logger.Warn().Err(err).Msg("competitors: drop old items")
		return
	}
	s.Logger.Warn().Int("items", len(old)).Msg("competitors: gave up on items that could not be read")
}

func (s *Service) product(ctx context.Context, ws string) (Product, error) {
	bp, err := s.Q.GetBrandProfile(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, nil
	}
	if err != nil {
		return Product{}, fmt.Errorf("load brand profile: %w", err)
	}
	return Product{Name: bp.Product, Overview: bp.Overview, Features: bp.Features}, nil
}

// processCompetitor reads one competitor's new items in a single model call
// and stores the moves. On any error the items stay unprocessed and are
// tried again next round.
func (s *Service) processCompetitor(ctx context.Context, competitorID string, batch []database.CompetitorItem) error {
	comp, err := s.Q.GetCompetitorByID(ctx, competitorID)
	if err != nil {
		return fmt.Errorf("load competitor: %w", err)
	}
	ws := comp.WorkspaceID
	product, err := s.product(ctx, ws)
	if err != nil {
		return err
	}
	var themes []Theme
	if s.Themes != nil {
		if themes, err = s.Themes(ctx, ws); err != nil {
			return fmt.Errorf("load demand themes: %w", err)
		}
	}
	stored, err := s.Q.ListMovesForCompetitor(ctx, database.ListMovesForCompetitorParams{CompetitorID: comp.ID, Since: time.Now().Add(-recentMovesWindow)})
	if err != nil {
		return fmt.Errorf("load recent moves: %w", err)
	}
	recent := make([]RecentMove, len(stored))
	for i, m := range stored {
		recent[i] = RecentMove{ID: m.ID, Title: m.Title, At: m.HappenedAt}
	}
	items := make([]Item, len(batch))
	ids := make([]string, len(batch))
	for i, it := range batch {
		items[i] = Item{Key: it.ItemKey, Title: it.Title, Body: it.Body, URL: it.Url, At: it.HappenedAt}
		ids[i] = it.ID
	}

	raw, err := s.chat(ctx, ws, func() (string, string) { return BuildMovesPrompt(product, comp.Name, themes, recent, items) })
	if err != nil {
		return err
	}
	moves, err := ParseMoves(raw, len(items), themes, recent)
	if err != nil {
		return err
	}
	created := 0
	for _, m := range moves {
		it := items[m.Item-1]
		if m.SameAs != "" {
			if it.URL != "" {
				if err := s.Q.AppendCompetitorMoveURL(ctx, database.AppendCompetitorMoveURLParams{ID: m.SameAs, CompetitorID: comp.ID, Url: it.URL}); err != nil {
					return fmt.Errorf("merge move: %w", err)
				}
			}
			continue
		}
		urls := []string{}
		if it.URL != "" {
			urls = append(urls, it.URL)
		}
		if _, err := s.Q.CreateCompetitorMove(ctx, database.CreateCompetitorMoveParams{
			CompetitorID: comp.ID, WorkspaceID: ws, Kind: m.Kind, Title: m.Title, Summary: m.Summary, Urls: urls,
			HappenedAt: it.At, Impact: m.Impact, Action: m.Action, ThemeID: m.ThemeID, Importance: m.Importance,
		}); err != nil {
			return fmt.Errorf("store move: %w", err)
		}
		created++
	}
	if err := s.Q.MarkCompetitorItemsProcessed(ctx, ids); err != nil {
		return fmt.Errorf("mark items read: %w", err)
	}
	if created > 0 {
		if err := s.rewriteSummary(ctx, comp); err != nil {
			s.Logger.Warn().Err(err).Str("competitor_id", comp.ID).Msg("competitors: rewrite summary")
		}
	}
	return nil
}

// chat charges one model call and runs it.
func (s *Service) chat(ctx context.Context, ws string, prompt func() (system, user string)) (string, error) {
	if s.Credits != nil {
		if err := s.Credits.Spend(ctx, ws, modelCost, reasonCredits); err != nil {
			return "", fmt.Errorf("credits: %w", err)
		}
	}
	system, user := prompt()
	out, err := s.Model.Chat(ctx, ws, []ai.ChatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}})
	if err != nil {
		return "", fmt.Errorf("model: %w", err)
	}
	return out, nil
}

// rewriteSummary writes "what they are building" from the competitor's
// stored moves of the last 90 days, and from nothing else. With no moves the
// summary is cleared without a model call.
func (s *Service) rewriteSummary(ctx context.Context, comp database.Competitor) error {
	stored, err := s.Q.ListMovesForCompetitor(ctx, database.ListMovesForCompetitorParams{CompetitorID: comp.ID, Since: time.Now().Add(-summaryWindow)})
	if err != nil {
		return fmt.Errorf("load moves: %w", err)
	}
	summary := ""
	if len(stored) > 0 {
		moves := make([]RecentMove, len(stored))
		details := map[string]string{}
		for i, m := range stored {
			moves[i] = RecentMove{ID: m.ID, Title: m.Title, At: m.HappenedAt}
			details[m.ID] = m.Summary
		}
		raw, err := s.chat(ctx, comp.WorkspaceID, func() (string, string) { return BuildSummaryPrompt(comp.Name, moves, details) })
		if err != nil {
			return err
		}
		summary = clip(strings.TrimSpace(raw), 1200)
	}
	return s.Q.SetCompetitorSummary(ctx, database.SetCompetitorSummaryParams{ID: comp.ID, Summary: summary})
}

func (s *Service) sendAlerts(ctx context.Context) {
	if s.Alert == nil {
		return
	}
	moves, err := s.Q.ListUnalertedHighCompetitorMoves(ctx, alertsPerPoll)
	if err != nil {
		s.Logger.Error().Err(err).Msg("competitors: list moves to alert")
		return
	}
	for _, m := range moves {
		// Marked first: an alert that fails is lost, never sent twice.
		if err := s.Q.MarkCompetitorMoveAlerted(ctx, m.ID); err != nil {
			s.Logger.Warn().Err(err).Str("move_id", m.ID).Msg("competitors: mark alerted")
			continue
		}
		u := ""
		if len(m.Urls) > 0 {
			u = m.Urls[0]
		}
		s.Alert(ctx, m.WorkspaceID, m.CompetitorName, Alert{Kind: m.Kind, Title: m.Title, Impact: m.Impact, Action: m.Action, URL: u})
	}
}
