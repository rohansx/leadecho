# Identity engine: from a forum handle to a reachable person

Status: built (phases 0–2, treg savings) · 2026-11-22 · branch `rohansx/agent-reach`

## Status

**Phase 0 (prod, 2026-11-22).** treg had never been called for an HN, Reddit, X,
Dev.to, Indie Hackers or Bluesky lead: all 226 enriched leads on those
platforms show every paid step `skipped` (no key to match on), 0 emails.
LinkedIn leads, which do carry a key: person found 103/103, email 102/103.

**Built.**
- `backend/internal/identity`: tier a resolver + `MergeAgent` evidence check.
  Results cached in Redis for every workspace (D3 = yes), 14 days.
- `backend/internal/ai/person_research.go`: tier b agent, Claude Sonnet 5.5
  with Claude's web search/fetch + sandbox tools. `llm.Router.ResearchPerson`.
- `backend/internal/sandbox`: E2B client; `identity-engine/`: the template
  (`leadecho-identity`) and its tool server (browse, username_sweep, youtube).
- Wiring in `researcher`: tier a before treg; tier b in the background when
  email or LinkedIn is still missing (`IDENTITY_AGENT_DAILY_CAP`, default 50
  per workspace); a LinkedIn found only by search is confirmed with one person
  lookup and a name match before it is used.
- `POST /ops/identity-backfill {"limit":n}` for leads that predate this.
- Dashboard: "Who they are" card in the lead's Person360 tab.
- Data model: no migration. `person_identities` (00021) gets one row per shown
  link (`platform` = kind, `source` = `identity_a|identity_b`, evidence in
  `metadata`); the profile lives in `leads.metadata.identity` and
  `persons.metadata.identity`.

**Fewer treg calls.** A published email skips `email.find` and `email.verify`
(status `published`); person/company/email lookups are cached 30 days instead
of 4 h; treg is only called once a key exists; a re-run happens only when an
earlier run had no key and one was found since.

**Not done (phase 3).** YouTube comment source (needs a YouTube Data API key
and a `platform_type` migration); Reacher (needs port-25 egress, which the
hosts here don't give); SearXNG (unneeded: the agent uses Claude's web search,
not Exa).

## 1. Problem

Leads come from HN and Reddit posts written under handles ("throwaway_dev42",
"pg"). Enrichment (`backend/internal/enrich`) sends the author to treg:
person → company → work email → verify. treg needs a key to match on: a
LinkedIn URL, or a full name plus company domain.

Today `enrich/identify.go` produces that key only by:

- X: reading the bio through treg (`treg.x.user.profile`)
- HN: reading the `about` field and regex-matching an email or LinkedIn URL
- Reddit: **nothing**

So most HN/Reddit authors reach treg with just a handle, and treg misses. treg
isn't failing on its own: **identity resolution is missing before it.**

Goal: for a qualified lead's author, find (with evidence) who they are, what
they do, what they usually post about, and how to reach them: website,
GitHub, X, LinkedIn URL, Bluesky/Mastodon, email, booking link (cal.com,
Calendly, SavvyCal, HubSpot meetings).

## 2. What this engine will not do, and why

| Not doing | Why |
|---|---|
| Logged-in scraping of X, Reddit or LinkedIn (Agent Reach's cookie backends, OpenCLI, rdt-cli, linkedin-scraper-mcp) | Already tried and removed (doc 14: Pinchtab/Camoufox). Cookie accounts get banned at server scale, and it breaks those sites' terms. "Public info" doesn't avoid this: the ban comes from *how* the data is fetched, not from what it is. |
| Harvesting emails from Git commits / GitHub for outreach | GitHub Acceptable Use Policy forbids using information from GitHub for unsolicited email. GitHub is used to learn who someone is and to find their links, never as an email source. |
| Full Maigret/Sherlock sweeps (3,000+ sites) | Slow, noisy (dating and gaming sites), hammers third parties. A curated list of ~40 professional sites only. |
| Account-existence probes (holehe, GHunt-style) | Invasive. Not a B2B prospecting signal. |
| Reading a LinkedIn page | We only find the **URL** (from the person's own site, GitHub or search) and hand it to treg, which is where treg works well. |

## 3. Shape

```
Leads Ready lead (score >= 6)
        │
        ▼
Tier A: deterministic resolver (Go, in backend/internal/enrich)   free, ~seconds
  HN profile (about + submitted links) · Reddit profile + recent posts (existing OAuth)
  GitHub user/search API · personal website via existing Scrapling sidecar
  rel="me" / social links / mailto / booking links · Gravatar · Bluesky DID
        │  confidence >= 0.8 ─────────────────────────────┐
        ▼  else, if lead score is high and quota left      │
Tier B: research agent in an E2B sandbox  (identity-engine/, Python)   cents/run
  LLM loop with read-only tools: search, read_page, github, youtube,
  username sweep (Maigret, curated sites), headless browser for JS sites
        │                                                  │
        ▼                                                  ▼
Identity graph (identities table, every claim has an evidence URL)
        │
        ▼
treg enrich with a real key (LinkedIn URL or name + domain)  → email + verify
```

Tier A ships first and alone. It is plain HTTP in Go, reuses `identify.go`
and the Scrapling sidecar, and probably covers most of the cases worth
covering. Tier B is added only for leads Tier A can't resolve, and only once
Tier A's measured hit rate shows how big that gap is.

## 4. Tier A: deterministic resolver (Go)

Extends `enrich.identify` into a small resolver that collects *candidate
links*, each with the evidence that produced it.

1. **Platform profile**
   - HN: `about` (already fetched) → parse every URL, not just email/LinkedIn.
     `submitted` items: domains they post often (`show HN` of their own site).
   - Reddit: `/user/{name}/about` + last ~50 posts/comments through the
     existing OAuth client (`monitor/reddit_oauth.go`). Profile description and
     links. Post subreddits give "what they post about".
2. **GitHub**: `GET /users/{handle}` (same handle) plus links found in step 1.
   Profile gives `name`, `company`, `blog`, `twitter_username`, `bio`,
   `location`. Needs a GitHub token in config (5k req/h).
3. **Personal website** (from GitHub `blog`, HN about, Reddit profile): fetch
   through the existing Scrapling sidecar (`backend/internal/browser/scrapling.go`).
   Read `/`, `/about`, `/contact`. Extract: `rel="me"` links, X/LinkedIn/GitHub/
   Bluesky/Mastodon links, `mailto:`, booking links
   (`cal.com/`, `calendly.com/`, `savvycal.com/`, `meetings.hubspot.com/`,
   `zcal.co/`, `tidycal.com/`), job title and company from the page text.
4. **Gravatar** profile by username (`gravatar.com/{handle}.json`): verified links.
5. **Bluesky**: a domain handle is verified ownership of that domain.

### Confidence (the core of lead quality)

A wrong person is worse than no person: it sends a cold email to a stranger.
So every link carries a reason, and handle equality alone is never enough.

| Evidence | Weight |
|---|---|
| Link found on the author's own platform profile (HN about, Reddit description) | strong |
| Two profiles link to each other (site ↔ GitHub, GitHub → X and X bio → site) | strong |
| Bluesky domain handle / `rel="me"` verified link | strong |
| Same handle + same display name + overlapping topic/location | medium |
| Same handle only | weak: stored, never shown as a match |

Show a contact route only when its confidence is >= 0.8. The UI shows the
evidence chain ("found on their HN profile → their site links to this GitHub").

## 5. Tier B: research agent in E2B (`identity-engine/`)

A separate Python service, like `scrapling-sidecar/`. E2B's official SDKs are
Python and JS only (Go ports are 4–6-star hobby repos), and the useful tools
(Maigret, Crawl4AI, browser-use, Agent Reach, yt-dlp) are Python.

- **Trigger:** backend publishes `identity.research.requested` on the Redis
  stream backbone (doc 17) for leads with score >= threshold whose Tier A
  confidence < 0.8 and with workspace quota left. Engine consumes, writes
  results back through an internal API endpoint (not straight into the DB).
- **Where the agent loop runs:** in the engine, *not* in the sandbox. The
  sandbox only runs tool commands. LLM keys never enter the sandbox, and
  untrusted page content can't reach credentials.
- **Tools (all read-only):**
  | Tool | Backed by |
  |---|---|
  | `search(q)` | Exa (existing). Later SearXNG self-hosted to cut Exa spend |
  | `read_page(url)` | Crawl4AI (Apache-2.0) → markdown + links |
  | `browse(url, goal)` | Playwright / browser-use (MIT) for JS-only sites; capped steps |
  | `github(handle)` / `github_search(q)` | `gh` CLI |
  | `username_sweep(handle)` | Maigret (MIT), `--tags` limited to the ~40 professional sites |
  | `youtube(channel_or_video)` | yt-dlp |
  | Agent Reach | preinstalled as a convenience router for GitHub/YouTube/RSS/web only. Its X/Reddit/LinkedIn channels stay off (§2) |
- **Output:** strict JSON schema: candidate links (each with `evidence_url` +
  `method`), `summary_what_they_do` (1–2 lines), `topics_they_post_about`,
  `company`, `role`. Any claim without an evidence URL is dropped server-side.
- **Prompt injection:** page text is treated as data. The agent has no write
  tools, no outbound messaging, no credentials. Output is validated against
  the schema and the confidence rules in §4 before storage.
- **Limits:** 5 min sandbox timeout, max ~25 tool calls, per-workspace daily
  cap reusing `middleware.NewQuota` / the `enrich` daily cap pattern.

### E2B template

```dockerfile
# identity-engine/e2b.Dockerfile
FROM e2bdev/code-interpreter:latest
RUN pip install --no-cache-dir maigret crawl4ai browser-use yt-dlp agent-reach \
 && playwright install --with-deps chromium
RUN apt-get update && apt-get install -y gh && rm -rf /var/lib/apt/lists/*
COPY sites.json /opt/maigret/sites.json   # curated professional site list
```

Built with `e2b template build`. The same image can run as a plain Dokploy
container if E2B per-run cost or availability becomes a problem (decision D2).

## 6. Email

In order, stopping at the first verified hit:

1. Published by the person: HN about, website `mailto:`, GitHub profile
   public email field (shown on the profile itself, not from commits).
2. treg email-find with the now-known name + company domain (existing step,
   much higher hit rate with a real key).
3. Later, if (2) stays weak: pattern guess (`first@`, `first.last@`) verified
   by Reacher (`check-if-email-exists`, MIT, self-hosted Rust). Needs port-25
   egress from a clean IP, and is useless on catch-all domains, so it comes
   after treg.

## 7. Data model

New table, one row per discovered identity of a person:

```sql
CREATE TABLE person_identities (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  person_id    uuid NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
  kind         text NOT NULL,   -- website|github|x|linkedin|bluesky|mastodon|email|booking|youtube
  value        text NOT NULL,   -- normalised URL or address
  confidence   real NOT NULL,
  evidence     jsonb NOT NULL,  -- [{url, method, tier}]
  found_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (person_id, kind, value)
);
```

`persons.metadata.research` holds the summary (what they do, topics, role,
company, last researched at). Workspace-scoped like `persons`; a
cross-workspace cache of public identities is decision D3.

## 8. Phases

| Phase | Scope | Size |
|---|---|---|
| 0. Measure | Prod query: enrichment step hit rates by platform from `leads.metadata.enrichment.steps`. Hand-label 50 Leads Ready HN + Reddit authors as the gold set every later phase is scored on. | ½ day |
| 1. Tier A | Resolver in `enrich` (HN links, Reddit profile, GitHub, website via Scrapling, Gravatar, Bluesky). Confidence rules. `person_identities` migration. Feed LinkedIn URL / name + domain into treg. Identity card with evidence in the lead drawer. | 3–4 days |
| 2. Tier B | `identity-engine/` service, E2B template, agent loop, stream wiring, quota, schema validation. "What they do / what they post" summary. | ~1 week |
| 3. Extras | Reacher verification; YouTube comment scanner (separate source); SearXNG to reduce Exa; cost/yield panel per workspace. | as needed |

Each phase is judged on the gold set before the next starts.

## 9. Success metrics

- **Precision** on the gold set >= 90% for shown links (the number that matters most)
- % of Leads Ready authors with >= 1 verified contact route (email, booking link, or LinkedIn URL)
- treg enrich hit rate before vs after (wasted credits should drop)
- Cost per resolved lead (Tier B LLM + sandbox seconds)

## 10. Compliance

Public, professional information for B2B outreach (legitimate interest under
GDPR). Store the source of every field (the `evidence` column), honour
deletion requests by deleting the `persons` row (cascades), rate-limit and
respect robots.txt in `read_page`, keep the exclusions in §2.

## 11. Decisions needed

- **D1. Agent model.** Haiku 5.5 (cheap, likely enough for link-following) vs
  Sonnet 5.5. Proposal: Haiku, escalate to Sonnet on low-confidence results.
- **D2. E2B vs self-hosted container for Tier B.** E2B gives isolation and no
  ops. Self-hosted gives zero per-run cost. Same image either way. Proposal: E2B.
- **D3. Share resolved public identities across workspaces?** Saves cost when
  the same HN author appears for several customers, but mixes customer data.
  Proposal: no, for now.
