package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
	"leadecho/internal/knowledge"
	"leadecho/internal/onboarding"
)

// Onboarding flow v2: streamed analysis, editable brand profile, KB seed.

const (
	competitorLookupTimeout = 10 * time.Second
	kbSeedTimeout           = 3 * time.Minute
	sseWriteWindow          = 3 * time.Minute
)

var errBadURL = errors.New("bad url")

// SetKnowledge enables the KB seed from the crawled product site. Optional.
func (h *OnboardingHandler) SetKnowledge(kb *knowledge.Service) { h.kb = kb }

// SetCompetitorFinder enables the empty-competitors fallback. Optional (nil-safe).
func (h *OnboardingHandler) SetCompetitorFinder(f onboarding.CompetitorFinder) { h.finder = f }

func (h *OnboardingHandler) pageFetcher() onboarding.PageFetcher {
	if h.fetch != nil {
		return h.fetch
	}
	return onboarding.SafeFetcher(10 * time.Second)
}

// sse writes Server-Sent Events. The API server's WriteTimeout (30s) is
// extended per stream so a ~45s build is not cut off.
type sse struct {
	w http.ResponseWriter
	f http.Flusher
}

func startSSE(w http.ResponseWriter) (*sse, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return nil, false
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(sseWriteWindow))
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream
	w.WriteHeader(http.StatusOK)
	f.Flush()
	return &sse{w: w, f: f}, true
}

func (s *sse) send(event string, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		b = []byte(`{}`)
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, b)
	s.f.Flush()
}

type analysisResult struct {
	Analysis     *ai.ProductAnalysis     `json:"analysis"`
	BrandProfile onboarding.BrandProfile `json:"brand_profile"`
	Pages        []onboarding.Page       `json:"pages"`
	Fallback     bool                    `json:"fallback"` // true when no LLM was available
}

// runAnalysis crawls the product site, analyses it (LLM or keyless
// heuristic), fills empty competitors, persists the analysis + brand profile
// and seeds the knowledge base in the background.
func (h *OnboardingHandler) runAnalysis(ctx context.Context, wsID, rawURL string, step func(string)) (*analysisResult, error) {
	pageURL, err := onboarding.NormalizeProductURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errBadURL, err)
	}
	rec, err := h.q.CreateOnboardingAnalysis(ctx, database.CreateOnboardingAnalysisParams{
		WorkspaceID: wsID, SourceUrl: pageURL, Analysis: []byte("{}"), Status: "pending",
	})
	if err != nil {
		return nil, fmt.Errorf("create analysis record: %w", err)
	}
	fail := func(msg string, cause error) error {
		_, _ = h.q.UpdateOnboardingAnalysis(ctx, database.UpdateOnboardingAnalysisParams{
			ID: rec.ID, Analysis: []byte("{}"), Status: "failed",
			ErrorMessage: pgtype.Text{String: msg + ": " + cause.Error(), Valid: true},
		})
		return fmt.Errorf("%s: %w", msg, cause)
	}

	pages, err := h.crawlSite(ctx, pageURL, step)
	if err != nil {
		return nil, fail("failed to fetch URL", err)
	}
	step(fmt.Sprintf("Read %d page(s), analysing your product", len(pages)))

	res := &analysisResult{Pages: pages}
	if h.llmRouter != nil {
		res.Analysis, err = h.llmRouter.AnalyzeProductPage(ctx, wsID, onboarding.AnalysisText(pages))
	}
	if h.llmRouter == nil || err != nil {
		h.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("onboarding: AI analysis unavailable, using page heuristics")
		res.Analysis, res.Fallback = onboarding.HeuristicAnalysis(pages), true
		step("No AI provider available: drafted the profile from your page, please review it")
	}
	h.fillCompetitors(ctx, res.Analysis, pageURL, step)
	step(fmt.Sprintf("Found %d pain points and %d competitors", len(res.Analysis.PainPoints), len(res.Analysis.Competitors)))

	resultJSON, _ := json.Marshal(res.Analysis)
	_, _ = h.q.UpdateOnboardingAnalysis(ctx, database.UpdateOnboardingAnalysisParams{ID: rec.ID, Analysis: resultJSON, Status: "completed"})

	bp, err := onboarding.FromAnalysis(res.Analysis, pageURL).Normalized()
	if err != nil { // no product name at all: keep the URL host so the card is editable
		bp.Product = pageURL
	}
	if row, err := h.q.UpsertBrandProfile(ctx, bp.Params(wsID)); err == nil {
		bp = onboarding.FromRow(row)
	} else {
		h.logger.Error().Err(err).Str("workspace_id", wsID).Msg("onboarding: save brand profile")
	}
	res.BrandProfile = bp
	h.seedKnowledgeBase(wsID, pages)
	return res, nil
}

// crawlSite crawls via the SSRF-safe fetcher, falling back to Scrapling for
// the homepage alone (JS-heavy sites).
func (h *OnboardingHandler) crawlSite(ctx context.Context, pageURL string, step func(string)) ([]onboarding.Page, error) {
	pages, err := onboarding.Crawl(ctx, h.pageFetcher(), pageURL, onboarding.MaxCrawlPages, step)
	if err == nil && len(pages) > 0 && len(pages[0].Text) > 200 {
		return pages, nil
	}
	if h.scrapling != nil {
		if nerr := h.scrapling.Navigate(ctx, pageURL); nerr == nil {
			if text, terr := h.scrapling.GetText(ctx); terr == nil && text != "" {
				return append([]onboarding.Page{{URL: pageURL, Text: text}}, tail(pages)...), nil
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return pages, nil
}

func tail(p []onboarding.Page) []onboarding.Page {
	if len(p) < 2 {
		return nil
	}
	return p[1:]
}

func (h *OnboardingHandler) fillCompetitors(ctx context.Context, pa *ai.ProductAnalysis, pageURL string, step func(string)) {
	if len(pa.Competitors) > 0 || h.finder == nil {
		return
	}
	step("Looking up alternatives to " + pa.ProductName)
	cctx, cancel := context.WithTimeout(ctx, competitorLookupTimeout)
	defer cancel()
	found, err := h.finder.FindCompetitors(cctx, pa.ProductName, pageURL)
	if err != nil {
		h.logger.Warn().Err(err).Msg("onboarding: competitor fallback failed")
		return
	}
	pa.Competitors = found
}

// seedKnowledgeBase stores each crawled page as a KB document (skipping URLs
// already in the KB) and indexes it, in the background.
func (h *OnboardingHandler) seedKnowledgeBase(wsID string, pages []onboarding.Page) {
	if h.q == nil || len(pages) == 0 {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error().Interface("panic", r).Msg("onboarding: KB seed panic")
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), kbSeedTimeout)
		defer cancel()
		seeded := 0
		for _, p := range pages {
			if h.seedPage(ctx, wsID, p) {
				seeded++
			}
		}
		h.logger.Info().Str("workspace_id", wsID).Int("documents", seeded).Msg("onboarding: knowledge base seeded")
	}()
}

func (h *OnboardingHandler) seedPage(ctx context.Context, wsID string, p onboarding.Page) bool {
	if len(p.Text) < 100 {
		return false
	}
	src := pgtype.Text{String: p.URL, Valid: true}
	if n, err := h.q.CountDocumentsBySourceURL(ctx, database.CountDocumentsBySourceURLParams{WorkspaceID: wsID, SourceUrl: src}); err != nil || n > 0 {
		return false
	}
	title := p.Title
	if title == "" {
		title = p.URL
	}
	doc, err := h.q.CreateDocument(ctx, database.CreateDocumentParams{
		WorkspaceID: wsID, Title: title, Content: p.Text, ContentType: "text", SourceUrl: src,
		FileSizeBytes: pgtype.Int4{Int32: int32(len(p.Text)), Valid: true},
	})
	if err != nil {
		h.logger.Warn().Err(err).Str("url", p.URL).Msg("onboarding: KB document create failed")
		return false
	}
	if h.kb != nil {
		if err := h.kb.IndexDocument(ctx, wsID, doc.ID, doc.Title, doc.Content); err != nil {
			h.logger.Warn().Err(err).Str("document_id", doc.ID).Msg("onboarding: KB index failed (document kept, re-save to index)")
		}
	}
	return true
}

// AnalyzeStream runs the analysis and streams progress as SSE.
// POST /onboarding/analyze {url} → events: step, result | error.
func (h *OnboardingHandler) AnalyzeStream(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if _, err := onboarding.NormalizeProductURL(body.URL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s, ok := startSSE(w)
	if !ok {
		return
	}
	res, err := h.runAnalysis(r.Context(), wsID, body.URL, func(msg string) { s.send("step", map[string]string{"message": msg}) })
	if err != nil {
		s.send("error", map[string]string{"error": err.Error()})
		return
	}
	s.send("result", res)
}

// GetBrandProfile returns the workspace's brand profile.
// GET /brand-profile
func (h *OnboardingHandler) GetBrandProfile(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	row, err := h.q.GetBrandProfile(r.Context(), wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "brand profile not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load brand profile")
		return
	}
	writeJSON(w, http.StatusOK, onboarding.FromRow(row))
}

// PutBrandProfile replaces the workspace's brand profile.
// PUT /brand-profile
func (h *OnboardingHandler) PutBrandProfile(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	var body onboarding.BrandProfile
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	bp, err := body.Normalized()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := h.q.UpsertBrandProfile(r.Context(), bp.Params(wsID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save brand profile")
		return
	}
	writeJSON(w, http.StatusOK, onboarding.FromRow(row))
}
