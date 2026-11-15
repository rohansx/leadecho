# Competitor Watch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** LeadEcho finds a workspace's competitors itself, watches what they publish, and turns it into moves with an impact and a suggested action.

**Architecture:** A new `internal/competitors` package owns four tables (competitors, sources, items, moves). The monitor tick calls its `Poll`, which sets up workspaces that have none, fetches due sources into `competitor_items`, and turns unprocessed items into `competitor_moves` with one model call per competitor. The dashboard, the digest and alerts read moves.

**Tech Stack:** Go (chi, pgx, sqlc 1.31.1, goose), Postgres, React + TanStack Router dashboard. No new dependencies: feeds use `encoding/xml`, HTML uses `golang.org/x/net/html` (already in go.mod).

**Spec:** `docs/superpowers/specs/2026-11-15-competitor-watch-design.md`

## Global Constraints

- The customer is never asked to type a competitor. Setup is automatic.
- Competitor content never becomes a `mentions` row and never reaches lead scoring.
- All competitor URLs are fetched through `onboarding.SafeFetcher` (SSRF guard).
- Fetched text is untrusted: prompts say so; model output is parsed as strict JSON; unknown theme ids are discarded.
- First check of a source records a baseline and produces no items.
- Each source is checked at most once every 24h. Five failures in a row marks it broken; a paid-data refusal is not a failure.
- Caps: 3 competitors on free, 10 on paid; 10 sources per competitor; 3 GitHub repos per competitor.
- Free plan: pages, feeds and GitHub only; `x` and `linkedin` sources are stored but shown locked and not polled.
- `importance: high` only for a pricing move, or a launch linked to a demand theme whose coverage is `gap` or `partial`.
- Import direction: `competitors` may import `monitor`, `onboarding`, `demand`; `monitor` must not import `competitors` (it holds an interface).
- Shell in this environment: no heredocs, `rm` prompts, `cp` is `cp -i`. sqlc and goose are in `~/go/bin`.

## Review Focus

1. A page rendered in the browser returns almost no text: the source must go broken, not "unchanged forever".
2. A competitor name resolves to the wrong company (or to the customer's own site): it must not be watched.
3. A feed that reorders or re-dates old entries must not re-emit them as new.
4. The model returns a move for item N that does not exist, or a theme id that does not exist: dropped, not stored.
5. A workspace with no brand profile or an empty competitor list: setup does nothing and is not retried every tick.

---

### Task 1: Schema and queries

**Files:**
- Create: `backend/migrations/00039_competitor_watch.sql`
- Create: `backend/internal/database/queries/competitors.sql`
- Generated: `backend/internal/database/competitors.sql.go`, `models.go`, `querier.go`

**Produces** (sqlc names):
- `competitors(id, workspace_id, name, website, status 'active'|'paused', summary, summary_at, created_at)`, unique `(workspace_id, lower(name))`.
- `competitor_sources(id, competitor_id, workspace_id, kind, url, last_checked_at, last_hash, last_text, failures, broken, created_at)`, kind in `x, linkedin, page_landing, page_pricing, changelog, blog, github`, unique `(competitor_id, kind, url)`.
- `competitor_items(id, source_id, competitor_id, workspace_id, item_key, title, body, url, happened_at, processed_at, created_at)`, unique `(source_id, item_key)`.
- `competitor_moves(id, competitor_id, workspace_id, kind 'launch'|'pricing'|'positioning'|'news', title, summary, urls TEXT[], happened_at, impact, action, theme_id, importance 'high'|'normal', alerted_at, created_at)`.
- `workspaces.settings.competitors_setup_at` marks that setup ran (no column).
- Queries: `ListCompetitors`, `CountCompetitors`, `CreateCompetitor` (ON CONFLICT DO NOTHING), `UpdateCompetitor`, `DeleteCompetitor`, `SetCompetitorSummary`, `ListSourcesByWorkspace`, `CreateCompetitorSource`, `DeleteCompetitorSource`, `ListDueCompetitorSources(before, max_rows)` (active competitors, not broken), `MarkSourceChecked(id, hash, text)`, `MarkSourceFailed(id)` (failures+1, broken at 5), `InsertCompetitorItem` (ON CONFLICT DO NOTHING, `:execrows`), `ListUnprocessedItems(max_rows)`, `MarkItemsProcessed(ids)`, `CreateCompetitorMove`, `AppendMoveURL`, `ListMovesByWorkspace(since)`, `ListMovesByCompetitor(since)`, `ListUnalertedHighMoves`, `MarkMoveAlerted`, `ListWorkspacesNeedingCompetitorSetup(max_rows)`, `MarkCompetitorSetup`, `ListCompetitorPeople(workspace_id, name_pattern, since, max_rows)`.

- [ ] Write migration and queries; run `~/go/bin/sqlc generate` in `backend/`; `go build ./...` passes.
- [ ] Apply migration to the local test database; `go test ./internal/database/...` passes.
- [ ] Commit.

### Task 2: Source discovery

**Files:** Create `backend/internal/competitors/discover.go`, `discover_test.go`.

**Produces:**
- `type Source struct{ Kind, URL string }`
- `func ExtractSources(pageURL, html string) []Source` (pure)
- `func FeedLink(pageURL, html string) string` (pure; first RSS/Atom alternate link, absolute)
- `func Discover(ctx context.Context, fetch onboarding.PageFetcher, website string) []Source` (homepage, then feed lookup on changelog/blog pages; always includes `page_landing`)

Tests (table, in `discover_test.go`):
- homepage with one `x.com/acme`, one `linkedin.com/company/acme`, `github.com/acme`, `/changelog`, `/blog`, `/pricing` yields one source of each kind plus `page_landing`.
- two different X handles on the page yields no `x` source.
- `x.com/intent/tweet`, `x.com/share`, `twitter.com/home`, `linkedin.com/in/someone`, `linkedin.com/shareArticle`, `github.com/acme/repo/issues/3` are ignored or reduced (`github.com/acme/repo` becomes `github.com/acme`).
- relative and off-site links: `/pricing` resolves against the page; `https://other.com/blog` is ignored.
- `FeedLink` returns the absolute href for `type="application/atom+xml"` and `rss+xml`, empty otherwise.

- [ ] Tests fail, implement, tests pass, commit.

### Task 3: Fetching items from a source

**Files:** Create `backend/internal/competitors/watch.go`, `feed.go`, `watch_test.go`, `feed_test.go`.

**Produces:**
- `type Item struct{ Key, Title, Body, URL string; At time.Time }`
- `func ParseFeed(b []byte) ([]Item, error)` (RSS 2.0 and Atom; key = guid/id, else link)
- `func PageText(html string) string` (visible text; drops script/style/nav/footer)
- `func NormalizeText(s string) string` (collapse whitespace; drop lines that are only digits, dates, or "© year")
- `func Hash(s string) string` (sha256 hex)
- `const MinPageText = 200` (runes; below this a page fetch is a failure)
- `func GitHubFeeds(ctx, fetch, orgURL string) []string` (up to 3 `releases.atom` URLs; from the org page's pinned/first repo links)
- `type PostFetcher interface{ ProfilePosts(ctx context.Context, wsID, platform, handle string) ([]Item, error) }`
- `var ErrLocked = errors.New("paid data not available")`
- `func (s *Service) fetchSource(ctx, src database.CompetitorSource) (items []Item, hash, text string, err error)`:
  - `x`/`linkedin`: `Posts.ProfilePosts`; paid refusal returns `ErrLocked`.
  - `changelog`/`blog`: feed URL if the stored URL is a feed, else page diff.
  - page diff: first check (empty `last_hash`) returns no items; changed text returns one item, key = new hash, body = `"BEFORE:\n…\n\nAFTER:\n…"` each side capped at 6000 runes.
  - `github`: releases.atom entries.

Tests:
- `ParseFeed` on an RSS sample and an Atom sample: keys, titles, dates, links.
- `NormalizeText`: two texts differing only by a date line and whitespace hash equal.
- `PageText` on a page with only `<script>` content is shorter than `MinPageText` (Review Focus 1).
- page diff: first check no items; same text no items; changed text one item with both sides.
- feed re-dated entry with the same guid has the same key (Review Focus 3; dedupe is the unique index).

- [ ] Tests fail, implement, tests pass, commit.

### Task 4: Automatic setup (names to websites to sources)

**Files:** Create `backend/internal/competitors/setup.go`, `setup_test.go`. Modify `backend/internal/onboarding/competitors.go` (export aggregator check).

**Produces:**
- `type SearchFunc func(ctx context.Context, query string) ([]monitor.ExaWebResult, error)`
- `func PickWebsite(name, ownHost string, hits []monitor.ExaWebResult) string` (pure): first hit that is not an aggregator, not `ownHost`, and whose host label or title's first segment matches the name ignoring case, spaces and punctuation; else "".
- `func (s *Service) Setup(ctx context.Context, wsID string) (created int, unresolved []string, err error)`: names from the brand profile, capped by plan, resolve, create competitor + discovered sources, store unresolved names in `workspaces.settings.competitors_unresolved`, mark setup.
- `func (s *Service) AddCompetitor(ctx, wsID, name, website string) (database.Competitor, error)` (manual add; runs discovery).

Tests:
- `PickWebsite("Acme HR", "mine.com", …)`: skips `g2.com`, skips `mine.com`, skips `acme-reviews.io` titled "Top 10 HR tools", picks `acmehr.com` (Review Focus 2).
- no matching hit returns "".
- `Setup` with a fake store and an empty competitor list creates nothing and still marks setup (Review Focus 5).
- `Setup` stops at the plan cap.

- [ ] Tests fail, implement, tests pass, commit.

### Task 5: Poll loop

**Files:** Create `backend/internal/competitors/service.go`, `poll.go`, `poll_test.go`. Modify `backend/internal/monitor/monitor.go` (interface + call in `tick`), `backend/internal/monitor/tracked_profiles.go` (export `ProfilePosts`), wiring in `backend/cmd/api` / `internal/api/router.go`.

**Produces:**
- `type Service struct{ Q Store; Fetch onboarding.PageFetcher; Posts PostFetcher; Search SearchFunc; Model Model; Credits Credits; Themes ThemeSource; Notify Notifier; Logger zerolog.Logger; Now func() time.Time }`
- `func (s *Service) Poll(ctx context.Context)`: (1) set up up to 2 workspaces needing it; (2) up to 40 due sources: fetch, insert items, mark checked / failed (`ErrLocked` marks checked without failure); (3) process unprocessed items (Task 6); (4) alert (Task 8).
- monitor: `type CompetitorWatcher interface{ Poll(ctx context.Context) }`, `func (m *Monitor) SetCompetitors(w CompetitorWatcher)`, called after `pollTrackedProfiles`.
- monitor: `func (m *Monitor) ProfilePosts(ctx, wsID, platform, handle string) ([]ProfilePost, error)` returning id, text, url, time; an adapter in `competitors` maps it to `Item`.

Tests (fake store, fake fetcher):
- baseline: first poll of a page source stores a hash and inserts no items.
- five failed fetches mark the source broken; `ErrLocked` does not increment failures.
- a short page (under `MinPageText`) counts as a failure.

- [ ] Tests fail, implement, tests pass; `go build ./...`; commit.

### Task 6: Items to moves

**Files:** Create `backend/internal/competitors/moves.go`, `moves_test.go`.

**Produces:**
- `type Model interface{ Chat(ctx context.Context, workspaceID string, msgs []ai.ChatMessage) (string, error) }`
- `type ThemeSource interface{ Themes(ctx context.Context, wsID string) ([]Theme, Product, error) }` with `Theme{ID, Title, Coverage string}`; implemented over `demand.DBStore`.
- `func BuildMovesPrompt(p Product, competitor string, themes []Theme, recent []RecentMove, items []Item) (system, user string)`
- `func ParseMoves(raw string, nItems int, themes []Theme, recent []RecentMove) ([]MoveOut, error)` where `MoveOut{Item int; SameAs string; Kind, Title, Summary, Impact, Action, ThemeID, Importance string}`.
- `func (s *Service) processCompetitor(ctx, comp database.Competitor, items []database.CompetitorItem) error`: one model call, at most 20 items; store moves, append URL on `SameAs`, mark all items processed, rewrite summary (Task 7). A model or parse error leaves items unprocessed for the next tick; items older than 3 days unprocessed are marked processed and logged.

Parse rules: item index outside `1..nItems` dropped; kind outside the four dropped; unknown theme id cleared; `SameAs` must be a known recent move id or it is cleared; importance forced to `normal` unless kind is `pricing`, or kind is `launch` with a theme whose coverage is `gap`/`partial`; forced to `high` for `pricing`. Text fields clipped (title 120, others 400).

Tests: each parse rule above (Review Focus 4); JSON wrapped in a code fence parses; garbage returns an error.

- [ ] Tests fail, implement, tests pass, commit.

### Task 7: Rolling summary

**Files:** Modify `backend/internal/competitors/moves.go`; test in `moves_test.go`.

**Produces:** `func (s *Service) rewriteSummary(ctx, comp database.Competitor) error`: moves from the last 90 days, one model call, 3 to 5 sentences, stored with `SetCompetitorSummary`. No moves: summary cleared. Prompt states "write only from the moves listed".

- [ ] Test (fake model sees only the listed moves; empty list makes no model call), implement, commit.

### Task 8: API

**Files:** Create `backend/internal/api/handler/competitors.go`, `competitors_test.go`. Modify `backend/internal/api/router.go`.

**Produces** (all under `/api/v1`, workspace-scoped):
- `GET /competitors` → `{data:[{id,name,website,status,summary,sources:[{id,kind,url,broken,locked,last_checked_at}],moves:[…last 90 days…],people:int}], unresolved:[names], suggestions:[names], limit:int}`. `suggestions` = competitor names appearing in 2+ demand themes or asks and not on the list.
- `POST /competitors` `{name, website}`; `PATCH /competitors/{id}` `{website?, status?}`; `DELETE /competitors/{id}`.
- `POST /competitors/{id}/sources` `{kind,url}`; `DELETE /competitors/{id}/sources/{sourceID}`.
- `GET /competitors/{id}/people` → recent mentions tagged `competitor_complaint` or `seeking_alternative` whose content matches the name as a whole word (last 60 days, 50 max).

Validation: website must pass `onboarding.NormalizeProductURL`; kind in the allowed set; caps return 409 with a plain message; a competitor from another workspace is 404.

- [ ] Handler tests (validation, cap, cross-workspace 404), implement, commit.

### Task 9: Dashboard page

**Files:** Create `dashboard/src/routes/_dashboard/competitors.tsx`. Modify the sidebar nav and the API client the other pages use; `dashboard/src/routes/_dashboard/demand.tsx` (note under a linked theme).

Layout as sketched in the spec: one card per competitor (name, summary, counts, moves newest first with High badge, "For you" and "Do" lines, link to the theme and to the people list), sources drawer with broken/locked states, unresolved names, one-click suggestions, add by hand. Empty state: "We're finding your competitors" before setup, "Nothing has moved yet" after.

- [ ] Build, `make web-check` passes, look at it in the browser, commit.

### Task 10: Digest and alerts

**Files:** Modify `backend/internal/digest/build.go`, `data.go`, `render.go`, tests; `backend/internal/competitors/poll.go` (alert step).

**Produces:**
- `digest.Data.CompetitorMoves []CompetitorMove{Competitor, Kind, Title, Impact, URL string; High bool}`: period's moves, high first, at most 5; section omitted when empty; included in `Facts`.
- Alert: each unalerted high move is sent once through the workspace's existing alert channels and marked alerted.

- [ ] Tests (ordering, cap, omitted when empty; alert sent once), implement, commit.

### Task 11: Check against the Utkrusht workspace

- [ ] Read Utkrusht's brand profile competitors (read-only), run setup and one poll locally with that list, and report which competitors resolved, which sources were found, and what broke.
