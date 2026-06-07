# LeadEcho E2E Hardening — Iteration Journal

Branch: `e2e-hardening` (off `master` @ 884134e). No pushes without approval.

## Stack (isolated, never touches the dokploy prod stack)
- Postgres: `leadecho-e2e-postgres` @ 127.0.0.1:15433 (pgvector pg16)
- Redis: `leadecho-e2e-redis` @ 127.0.0.1:16380
- API: pm2 `leadecho-api` @ :8090 (native go binary, rebuilt from /opt/leadecho/backend)
- Dashboard: pm2 `leadecho-dash` @ :13100 (vite dev, served at /app/)
- Playwright suite: /opt/leadecho/.e2e/tests
- Go: /usr/local/go (1.25.7). Build: `go build -o /opt/leadecho/.e2e/leadecho-api ./cmd/api`
- Restart API after backend code change: `pm2 restart leadecho-api`

## Environment constraints
- AI keys (OPENAI/GLM/VOYAGE) are EMPTY (even in prod). AI features (scoring,
  embeddings, RAG reply gen) can't run live without keys → seed data + flag.
  Drop keys into `.e2e/backend.e2e.env` and `pm2 restart leadecho-api` to enable.
- Social scraping (Reddit/Twitter/LinkedIn) needs session cookies → seeded, not live.
- `pkill -f leadecho-api` self-kills the calling shell (string match) — never use it.

## Findings / Fixes
### [FIXED] #1 pgvector type never registered → all mention ingestion broken
- Symptom: every HN mention insert failed: `can't scan into dest[24]
  (col: content_embedding): unsupported data type: <nil>`. Inbox stays empty.
- Root cause: `NewPostgresPool` never registered the pgvector codec, so pgx had
  no codec for the `vector` OID; scanning the RETURNING clause failed. Compounded
  by `Mention.ContentEmbedding` being a non-pointer `pgvector.Vector` (column is
  nullable) so NULL couldn't be represented.
- Fix: `internal/database/postgres.go` — `config.AfterConnect` registers
  `pgvector-go/pgx` types on every conn. `internal/database/models.go` —
  `ContentEmbedding` → `*pgvector.Vector`. `go mod tidy`.
- Verified: 59 mentions persist (54 live HN + seed); no scan errors.

### [FIXED] #2 Auth redirects not basepath-aware → land on 404
- Symptom: after login/register the app did `window.location.href = "/inbox"`,
  and logout did `"/login"`. SPA basepath is `/app`, so users landed on `/inbox`
  / `/login` which 404 (dev) or hit the Astro landing (prod) — broken auth UX.
- Fix: `routes/_auth/login.tsx` + `routes/_auth/register.tsx` → `/app/inbox`;
  `lib/auth.tsx` logout → `/app/login`. (`_auth.tsx` `<Navigate to="/inbox">` is
  router-aware and already correct.)
- Verified by: tests/auth.spec.ts (asserts post-auth URL is /app/inbox).

## Bug-hunt (48 findings) — fix batches
### Batch A [DONE, committed] correctness/validation/error-mapping
- Inbox tier list/count mismatch (HIGH): rewrote ListMentionsFiltered as the exact
  NULL-safe complement of the other two tiers → high-score non-lead mentions no
  longer vanish from every tab. (mentions.sql + mentions.sql.go)
- mentions: UpdateStatus invalid→400, not-found→404; added awareness_level to response.
- leads: UpdateStage not-found→404 (was 500), stage validation→400 (Create+UpdateStage).
- keywords: duplicate→409 (was 500), platform/match_type validation→400, term trim,
  Delete malformed-id→400 & non-existent→404. NOTE: keyword platforms is text[] with 7
  crawler sources (reddit/hackernews/devto/lobsters/indiehackers/twitter/linkedin) — NOT
  the 4-value platform_type enum; match types broad/exact/phrase/contains.
- documents: GetDocument filters is_active=true (deleted→404); Update preserves IsActive
  (no resurrection); source_url must be http(s) (stored-XSS guard).
- profiles: embed-FIRST on update (no data loss on embed failure); truthful pain_points
  in responses; Delete non-existent→404; name trim.
- Regression spec: tests/api-regression.spec.ts (11 tests, all green).

### Batch B [DONE, committed] security
- UTM open-redirect: reject non-http(s) destinations on create + re-validate before
  redirect; build query via net/url (no raw concat).
- Webhook SSRF: provider host allowlist (https only) + dial-time block of
  private/loopback/link-local/CGNAT IPs (DNS-rebind safe) + no redirects +
  non-2xx treated as failure.
- config: refuse to boot outside development if JWT_SECRET missing/default or
  ENCRYPTION_KEY unset/reused.

### Batch C [DONE, committed] engine
- onboarding Complete idempotent + nil normalization + keyword error handling.
- monitor.insertMention defaults empty Status→new (fixes 7 sidecar crawlers).
- FindSimilarPainPoints joins monitoring_profiles WHERE is_active=true.
- analytics Overview propagates query errors (500) instead of silent zeros.
- Deduped 12 accumulated test-workspace profiles (pre-idempotency noise).

### Batch D [DONE, committed] frontend
- api.ts surfaces clean { error } message (no "<status>: {json}" leak).
- Global mutation error toast (MutationCache.onError) — failures no longer silent.
- Sidebar: removed duplicate /analytics 'Tracking' entry, unique keys, logo→/inbox.
- Regression: tests/frontend-ux.spec.ts.

### Still open (lower priority, noted not fixed)
- LOW: list `total` returns page size not full count (mentions/leads); pipeline
  column counts can exceed rendered cards >100/stage; crypto.MaskKey reveals
  9–11-char keys; webhook URLs stored plaintext in settings; BYOK keys stored but
  never consumed by AI handlers; RotateToken non-transactional; sessions per-user
  vs per-workspace divergence; WorkspaceID() fail-open dev fallback (latent).
- MEDIUM: inbox filters don't compose (tier/status/platform/search mutually
  exclusive); voyage EmbedTexts sparse-result guard.

## Feature surface to cover
Pages: index(overview), inbox, pipeline, keywords, knowledge-base, profiles,
analytics, alerts, workflows, browser-sessions, settings, onboarding, + auth.
