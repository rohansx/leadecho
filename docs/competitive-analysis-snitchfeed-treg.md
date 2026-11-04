# Competitive analysis + plan: SnitchFeed, treg.to → LeadEcho

*Researched 2026-11-04 from public sites plus logged-in walkthroughs of both apps (SnitchFeed trial workspace "CloakPipe", treg team "Cloakpipe").*

## TL;DR
- **SnitchFeed** is our direct competitor. Its real moat is an agentic setup:
  - the agent reads your site, writes boolean queries, live-tests them on each platform, then proposes a plain-English listener
  - an AI chat that re-triages the feed
  - an MCP server and copy-paste agent playbooks

  The underlying feed is noisy, and it stops at "here's a post": no attribution, no posting, no enrichment, CRM only through webhooks.
- **treg.to** is a supplier, not a competitor: pay-per-call access to 4,100+ endpoints with an official "integrate into your platform" mode (per-customer tagging, spend caps, invoicing ledger).
  - X search costs ~$0.00075 per call, LinkedIn post search ~$0.002, LinkedIn comments/reactions $0.0015, Reddit comments $0.0009, verified email from ~$0.005.
  - It can replace our cookie-based X/LinkedIn scraping and add enrichment for cents.
- **Plan:**
  1. Copy their agentic onboarding but fix its holes: no empty first feed, competitors actually saved, replies not gated, real samples before you commit.
  2. Ship MCP and API keys.
  3. Plug treg in for X/LinkedIn fetching and lead enrichment.
  4. Lead marketing with our closed loop (reply → posted → clicked → revenue), which SnitchFeed can't match.

---

## 1. SnitchFeed — what I saw inside the app

### Onboarding (chat with their agent "Snitch", ~3 min)
| Step | What happens | Verdict |
|---|---|---|
| Website | User gives a URL; the agent crawls it (~1 min) and asks "how did you hear about us?" meanwhile | ✅ hides the wait |
| Brand profile | Modal with 5 tabs: Overview (what you do / product / positioning), Customers (4 segments + why they buy), **Competitors (0 found)**, Problems (7 pain points with explanations), Keywords (15, grouped Brand / Product / Category / Problem). Editable later. | ✅ rich ICP model; ❌ missed competitors |
| Goals | Multi-select: Brand mentions · Watch competitors · Find buyers · Join conversations · Content ideas | ✅ |
| Platforms | Reddit/X/LinkedIn pre-checked, HN optional | ❌ Bluesky not offered |
| Alerts | Defaults to a 9:30 local daily email digest (time zone auto-detected), then Slack/Discord/email | ✅ good default; ❌ bug: the listener said "Send to Discord" after I picked email |
| Listener | "Describe in 3–4 sentences" plus draft chips built from the profile. The agent streams its steps (details below). | ⭐ best part |
| After creation | Upsells a brand listener, teammate invite, Connect Claude/ChatGPT (just URL + steps, marked done on click), demo | ok |
| First landing | **Empty feed**: "No mentions match your filters" (default fit = medium/high), then a "~30 s" placeholder. Opportunities also empty. | ❌ weak first-value moment |

The listener step in detail:
- The agent reads the brand profile and live-tests **14 boolean queries** on each platform with post counts (Reddit 43, LinkedIn 24, X 1, HN 0).
- It picks the best ones and explains why with a sample thread link.
- It proposes a card:
  - **Looks for:** plain-English positive description
  - **Skips:** plain-English negative description
  - per-platform scan cadence: LinkedIn 12h, X 4h, HN 6h, Reddit 12h
  - alerts
  - 2 raw queries, e.g. `("tool calls" OR "MCP proxy") AND (recommend OR alternative)`
- Below the card: Create / Change / a chat box to revise it.

### App surface
- **Snitch AI (home page):**
  - Chat with history, a reasoning-level selector, and chips (first buyers, today's best leads, watch competitors, write a reply).
  - Its answer to "best leads this week" was good triage: real leads, "not leads but relevant signal", and a next step. It explicitly demoted the top-fit post as "unclear fit".
  - **About 11 credits per answer.**
- **Feed:**
  - Table columns: fit %, AI one-line summary, author, intent tags, listener, age.
  - Detail drawer: author headline, full text with highlights, sentiment, 👍/👎, "seen by", bookmark/archive/push.
  - **Quality issues seen in 52 mentions:**
    - a thought-leadership post tagged "Seeking Alternative" at the top (76%)
    - duplicate rows
    - subreddit mod-bot posts
    - no lead above 76%
- **Replies:** locked until you create a Response Profile. The profile has:
  - goal preset: drive conversion / build relationship / ask for info / handle feedback / support / custom
  - special instructions
  - sliders: tone, length, assertiveness, brand plug
  - a CTA goal
  - per-platform voice examples (Reddit/X/LinkedIn/Bluesky)
- **Opportunities:** counters Opportunities / New / Leads / Engaged, with 24h/7d/30d views and "hide seen".
- **Listener editor:**
  - Intent signal: looks-for (500 chars) + not-relevant; "Update intent" changes scoring weights.
  - Keywords: 3 per listener, per-keyword platforms, whole word / case, skip-words, boolean.
  - Authors: track LinkedIn/X profiles; only-from / never-from handles.
  - Reddit: include/exclude subreddits (2 credits each per scan); **comment search paused "back soon"**; NSFW toggle.
  - Scan intervals with credits/month: **one listener ≈ 960 of the 1,000 trial credits.**
  - Marketing says Reddit is "real-time", but the default cadence is 12h.
- **GTM Tools** (LinkedIn only, also in MCP): person's posts, person's comments, post commenters → CSV, post reactors → CSV.
- **Analytics:** mention volume by fit, platform, keyword, intent; a report builder with many filters. **No outcome or revenue metric anywhere.**
- **Integrations:**
  - MCP: 7 clients; call log; prompt library (setup, find missing keywords, audit noise, weekly digest; daily triage and lead sweep as *scheduled tasks in the MCP client*)
  - Webhooks with delivery log
  - Public API keys for on-demand X/LinkedIn keyword search, so they resell data too
  - Slack
- **Settings:** brand context, team, notifications (daily + weekly digest times), credit ledger by source (polling / API / AI scoring / CSV), billing.

### Public-site facts (from earlier)
- 13 intent tags:
  - Buying Intent, Recommendation Request, Comparison, Seeking Alternative
  - Competitor Complaint, Competitor Mention
  - Brand Mention, Praise
  - Pain Point, Feature Request, Content Opportunity
  - Hiring, Promotional
- Platforms: Reddit, X, LinkedIn, Bluesky, HN.
- Pricing: $47 / $95 / $399 credit plans; webhooks + CSV on Pro and up.
- Native CRM: none (HubSpot only through webhook/Zapier).
- Large programmatic-SEO footprint (`/compare/*`, `/alternatives/*`, free tools).

## 2. treg.to — what I saw inside the app
- **Onboarding:**
  1. Pick your agent (Claude Code, Codex, …) and paste one line, `set up treg — https://treg.to/llms.txt`, into it. A team API key is shown; the agent signs itself in.
  2. "Try it out" prompt cards: trending TikTok, find a work email, LinkedIn profile lookup, LinkedIn posts summary, keyword volume, UGC video.
  3. Optional OAuth to *post* (X, YouTube, TikTok, LinkedIn, FB, IG), manage ads (Google/Meta), and read GA/Search Console.
  4. **Build on treg:** "List as vendor" (agent reads `vendor-listing.md`, opens a PR) or "Integrate into your platform" (`integrate.md`).
- **Tabs:**
  - Catalog: categories with "from" prices
  - Connections: add your own provider keys (BYOK = unmetered)
  - Your own tools: register your APIs, CLIs and skills; agents never hold the keys
  - Activity: every call with request and response
  - Team
- New team gets a $1.00 balance.
- **Platform integration contract** (`integrate.md`):
  - one org-scoped token, sent as `X-Treg-Token`
  - `X-Treg-Meta: customer=<ws_id>` per call
  - response headers `X-Treg-Cost-Micro` and `X-Treg-Call-Id`
  - `X-Treg-Route-Max-Cost` per-call cap
  - per-customer daily caps via `PUT /orgs/<id>/budgets/customer/<value>`
  - invoicing from `GET /orgs/<id>/usage/by-tag?key=customer`
  - `Idempotency-Key` on retries
  - never forward `402 insufficient_balance` or `429 platform_daily_cap_reached` to end users
- Discovery is unauthenticated: `GET https://treg.to/catalog/search?q=…`.

### Prices that matter to us (catalog, pulled 2026-11-04)
| Job | Tool | USD |
|---|---|---|
| X keyword search | `treg.x.search.posts` (routed) / `crawl4ai.x.search.posts` | 0.00075 / 0.00065 per call |
| LinkedIn post keyword search | `anyapi.linkedin.search.posts` / `scrapecreators…linkedin-search-posts` / `harvestapi.linkedin.post.search` | 0.002 / 0.00188 / 0.004 per call |
| LinkedIn post comments | `treg.linkedin.post.comments` (routed) | 0.0015 |
| LinkedIn post reactions | `treg.linkedin.post.reactions` (routed) | 0.0015 |
| LinkedIn user posts | `crawl4ai.linkedin.user.posts` | 0.0009 |
| Reddit search (≤250 posts) | `crawl4ai.reddit.search.posts` | 0.0009 |
| Reddit post comments | `crawl4ai.reddit.post.comments` | 0.0009 |
| Threads search | `treg.threads.search.posts` | 0.002 |
| Bluesky (no search, user/post only) | `scrapecreators…bluesky-*` | 0.00188 (use the free public AT API for search instead) |
| HN search | `crawl4ai.hackernews.search.stories` | 0.00015 (Algolia HN API is free anyway) |
| Company enrich | `crawl4ai.companies.enrich` / `blitzapi.companies.enrich` / routed `treg.companies.enrich` | 0.0005 / 0.000495 |
| Person enrich | routed `treg.people.enrich` (28 providers) | provider-dependent |
| Work email find | routed `treg.people.email.find`; e.g. aiark 0.0053, tomba-from-LinkedIn 0.0089 | ~0.005–0.03 |
| Email verify | `filtrat.people.email.verify` | 0.00025 |

**Unit economics:**
- X hourly for one keyword is 24 × $0.00075 ≈ **$0.54/mo**.
- LinkedIn every 4h for one keyword is 6 × $0.002 ≈ **$0.36/mo**.
- Enriching a lead (company + email find + verify) is ≈ **$0.01**.

So a $49 plan with 10 keywords on X + LinkedIn hourly-ish costs us under $10/mo in data. We can **offer faster cadence than SnitchFeed's 4–12h defaults** and still keep margin.

## 3. LeadEcho today (what we already have)
- **Onboarding** (`dashboard/src/routes/onboarding.tsx`, `handler/onboarding.go`), 3 steps:
  1. URL → `AnalyzeURL` (Scrapling/HTTP fetch → `AnalyzeProductPage`)
  2. Review/edit (product, description, pain points, keywords, platforms, subreddits)
  3. Deploy animation → `Complete` creates profile + embeddings + keywords → `ScanWorkspaceNow` → inbox
- **Bug/gap:** `ai.ProductAnalysis` already returns `Competitors`, `Features` and `TargetAudience`, but `completeRequest` drops them, so they're never saved.
- **Platforms:** Reddit, HN, Dev.to, Lobsters, Indie Hackers, Threads, Google News, Exa, plus X/LinkedIn/Quora through **cookie-based browser sidecars** (Pinchtab/Camoufox/Scrapling). No Bluesky.
- **Edges SnitchFeed lacks:**
  - semantic pain-point matching
  - KB-RAG replies
  - Chrome extension one-click posting
  - UTM attribution → revenue
  - native HubSpot/Attio sync
  - ICP fit score with GitHub enrichment
  - BYOK LLMs
  - self-host
- **Missing:**
  - MCP and public API keys
  - agentic listener builder with live tests
  - NL looks-for/skips
  - 13-tag taxonomy + sentiment
  - digests
  - CSV export
  - mute lists
  - profile tracking
  - LinkedIn engagers → leads
  - response profiles (voice sliders)
  - in-app AI chat
  - Bluesky

## 4. LeadEcho onboarding — the redesign
Goal: **a real lead with a ready reply within 2 minutes, never an empty screen.** We take SnitchFeed's best moves and remove the friction we hit.

1. **URL (only required input).**
   - Start `AnalyzeURL` immediately and stream progress ("reading pricing page…", "found 6 pain points").
   - While it runs, ask the goal chips (Find buyers / Competitor switchers / Brand mentions / Content ideas). One tap each.
2. **Brand profile card,** editable inline with the same 5 tabs as SnitchFeed: Overview, Customer segments, **Competitors** (persist what we already extract, plus "add"), Pain points, Keywords grouped Brand/Product/Category/Problem.
   - If competitors come back empty, run a cheap fallback: one `treg.web.search` for "<product> alternatives".
   - **Also auto-seed the Knowledge Base from the crawled pages,** so the first reply is grounded with zero setup.
3. **Agent builds one listener per goal.** For each:
   - generate keywords + NL **Looks for / Skips**
   - **live-test** against Reddit/HN/Dev.to (free public APIs) and X/LinkedIn (treg, a few cents total)
   - show counts **and 3 real sample posts**; the user taps 👍/👎 on samples
   - those taps become the first scorer calibration: few-shot examples per profile

   This replaces SnitchFeed's "trust me" card with proof.
4. **Backfill 7 days and land on a populated inbox.**
   - If no high-fit results exist yet, show medium-fit with a "loosening filter" banner rather than an empty state.
   - Never land on "No mentions match your filters".
5. **Reply-ready immediately.**
   - Auto-create a default Response Profile from the brand profile: goal = drive conversion, brand plug = low.
   - The top lead opens with a KB-grounded draft and the UTM link already inserted.
   - No gating.
6. **Then the optional cards,** in order of our moat:
   - **Install the extension (post in one click)**
   - set the conversion webhook/domain (attribution)
   - alerts: daily digest by default, Slack optional; *respect the choice* (their Discord bug)
   - connect Claude/ChatGPT (MCP)
   - invite a teammate

**Activation metrics to instrument:** time to first real lead (<2 min), first reply drafted in session 1, extension installed in session 1, first UTM click within 7 days.

## 5. Merged build plan

### P0 — onboarding + agent parity (≈3 weeks)
1. **Persist the full analysis.** Add `competitors`, `features`, `target_audience`, segments to `completeRequest` / monitoring profile; fix the drop. *(tiny)*
2. **NL Looks-for / Skips per profile,** fed into the scorer prompt next to the semantic match. Collect 👍/👎 on mentions as few-shot examples per profile. *(scorer.go + migration)*
3. **Listener builder agent:** generate queries → live-test per platform → counts + samples → user confirms. Reuse `researcher/icp.go`, the monitor fetchers and `AnalyzeURL`; one new endpoint that streams over SSE.
4. **7-day backfill on create + no-empty-inbox fallback.** Reuse `ScanWorkspaceNow` with a lookback parameter.
5. **Auto KB seed from the website crawl + default response profile.** Response profile = goal preset + tone/length/assertiveness/brand-plug + per-platform examples.
6. **Intent taxonomy → 13 tags + sentiment** (sqlc migration, scorer prompt). Add dedupe (same URL or near-duplicate text) and a bot-author filter; both were visible problems in SnitchFeed's feed.
7. **Workspace API keys** (hashed, scoped, revocable) + **MCP server** wrapping existing handlers. Start with ~12 tools:
   - `search_mentions`, `list_leads`, `get_lead`
   - `create_listener`, `tune_listener`, `mute`
   - `draft_reply`, `queue_reply`
   - `push_to_crm`, `set_stage`
   - `attribution_report` *(theirs can't do this)*
   - `weekly_summary`

### P1 — treg integration + retention (≈3 weeks)
8. **`internal/treg` client:** one wrapper that always sets `X-Treg-Meta: customer=<ws_id>`, `X-Treg-Route-Max-Cost` and `Idempotency-Key`, stores `X-Treg-Call-Id` + cost per workspace, and maps 402/429 to internal errors. Follow integrate.md exactly.
9. **X + LinkedIn fetchers through treg** as the default for hosted plans, behind the existing fetcher interface in `monitor/`. Keep the Pinchtab/Camoufox/Scrapling paths for self-host/BYOK. This removes the "paste your cookie" step from onboarding.
10. **Reddit comment search** (treg `crawl4ai.reddit.post.comments`, or the public JSON). SnitchFeed has this paused, so it's an easy differentiator.
11. **Lead enrichment on qualified leads only:**
    - author → `treg.people.enrich` → company (`treg.companies.enrich`) → `treg.people.email.find` → `filtrat` verify
    - feeds the ICP fit score
    - pushed to HubSpot/Attio
    - ~$0.01 per lead; cap per workspace with treg budgets
12. **Bluesky** monitor through the public AT Protocol search (free).
13. **Digests:** daily + weekly email, with a weekly AI summary that includes **$ attributed** ("12 buyers, 4 competitor complaints, 2 noisy keywords, $X attributed").
14. CSV export; mute authors/domains/topics; per-platform cadence with a cost preview (they show credits/month per listener; we should too).

### P2 — growth + agents
15. **In-app AI chat** (our "Snitch AI"): triage, "best leads this week", "why did this keyword make money". Same tools as MCP.
16. **LinkedIn engagers → leads:** a post's commenters and reactors via treg ($0.0015 per call), then enrich and score. Also profile/company tracking (`linkedin.user.posts`).
17. **Agent skills / prompt library** shipped in-app and in `docs/skills/`, plus scheduled automations (daily triage, lead sweep, attribution review).
18. **Distribution:**
    - `/compare/snitchfeed`, `/compare/octolens`, `/alternatives/*`, `/monitor/<platform>`, 2–3 free tools
    - an `llms.txt`
    - **list LeadEcho on treg as a vendor** (follow `vendor-listing.md`: needs self-serve API keys + a probe endpoint), so agents searching "buying-intent social mentions" find us

### Pricing response
- ~$49 entry.
- Webhooks, CRM and CSV on every plan (SnitchFeed gates them to Pro).
- Faster default cadence than theirs (hourly vs 4–12h), made affordable by treg.
- Attribution + enrichment as the upgrade.
- Meter enrichment openly ("credits") the way both competitors do.

### Risks
- **Scraped X/LinkedIn data through treg's upstreams:** ToS risk moves to them; it doesn't disappear. Keep it opt-in per workspace and keep the BYOK/self-host paths.
- **treg is a young dependency.** Keep it behind our fetcher/enricher interfaces and keep the free public-API paths for Reddit/HN/Bluesky.
- **License:** Apache 2.0 with extra terms. Using the hosted API in our product is allowed; re-hosting treg as a registry is not.

## 6. Notes
- Trial costs incurred during research: SnitchFeed 1,000 → 953 credits (1 listener + 1 AI question); treg $1.00 balance untouched (catalog queries are free and unauthenticated).
- Not tested: SnitchFeed Slack/MCP OAuth, webhooks payload, CSV export; treg paid calls.
