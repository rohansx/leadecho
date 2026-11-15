# Competitor Watch: design

*2026-11-15. Status: approved by the owner on 2026-11-15.*

## Intent

A LeadEcho customer names their competitors once. LeadEcho then watches what
each competitor publishes, keeps only the real moves, and tells the customer
three things per competitor: what they are building, how it affects the
customer, and what to do about it.

The reason to build it inside LeadEcho rather than leave it to a standalone
tool: LeadEcho already holds the other half of the picture. It knows what the
customer's market is asking for (demand themes, each marked covered / partial /
gap) and who is complaining about which competitor. A competitor move joined to
that evidence is advice a customer can act on. A move on its own is a
newsletter.

## Goals

- Competitors are a first-class thing in a workspace: a name, a website, and
  the places they publish. LeadEcho finds all three itself.
- Watch, per competitor: X posts, LinkedIn posts, landing page, pricing page,
  changelog, blog, GitHub releases.
- Turn raw items into **moves**: a launch, a pricing change, a positioning
  change, or other company news. Everything else is dropped.
- For each move, say how it bears on this customer, using their product
  profile and demand themes, and suggest one action.
- Keep a short rolling summary per competitor: what they are building.
- Show it on a Competitors page and in the weekly digest; alert on big moves.
- Attach the people in the feed who are complaining about, or looking to
  leave, that competitor.

## Non-goals

- Newsletters and in-app updates (decided 2026-11-15: out).
- Logging in to a competitor's product, or anything behind a login.
- Showing every post a competitor makes. Noise is dropped, not listed.
- A general "company brain" over the customer's own internal knowledge.
- Battlecards, win/loss tracking, sales-enablement content.

## Anti-goals

- Competitor posts landing in the inbox as leads, or being scored as leads.
- Advice with no evidence behind it. A suggested action must point at the move
  and, where it exists, at the demand theme or the people it rests on.
- A weekly wall of text. If nothing moved, the digest section says so in one
  line or is left out.
- Unbounded spend: a workspace cannot make polling or model cost grow without
  a cap.

## What exists today (and what this touches)

| Piece | Where | Use here |
|---|---|---|
| Competitor names per workspace (`TEXT[]`, names only) | `brand_profiles.competitors`, `internal/onboarding/competitors.go` | Seed for the new competitor rows |
| Polling X / LinkedIn profiles through treg, due-list pattern | `internal/monitor/tracked_profiles.go` | Same pattern and treg tools; **not** the same table, because tracked posts become mentions and get lead-scored |
| SSRF-safe page fetch with bounded redirects, HTML to text | `internal/onboarding/crawl.go` (`SafeFetcher`), `internal/netsafe` | Fetch competitor pages and feeds |
| Demand themes with coverage and named competitors | `internal/demand` (state in `workspaces.settings.demand`) | Join a move to a theme |
| Intent tags `competitor_complaint`, `seeking_alternative` | `internal/ai/classify.go` | Find the people to attach to a competitor |
| Daily / weekly digest, email + Slack | `internal/digest` | New "competitor moves" section |
| Alerts (Slack / Discord / email webhooks) | `internal/monitor/notify.go`, alerts page | Big-move alert |
| Credits per paid action | `internal/billing` | Meter polling and model calls |
| Model router | `internal/llm`, `internal/ai` | Classify and summarise |
| Dashboard routes | `dashboard/src/routes/_dashboard/` | New `competitors.tsx` |

## Approaches considered

**A. Reuse tracked profiles and mentions.** Add each competitor's X and
LinkedIn handle as a tracked profile, tag the resulting mentions, and filter
the inbox. Smallest build. Rejected: page changes and GitHub releases are not
posts and have no home; competitor posts would run through lead scoring (the
scorer already gives competitor launches 0 to 3) and sit in the inbox as
noise; a "move" with an impact and an action has nowhere to live.

**B. A competitor-watch package with its own tables (chosen).** New tables for
competitors, their sources, and moves. Watchers per source kind reuse the treg
polling, the safe fetcher, and the model router. Moves are joined to demand
themes and to complaint mentions at read time. More to build than A, but each
part is small, every claim traces to a stored item with a URL and a date, and
nothing leaks into the lead pipeline.

**C. A weekly research agent.** Once a week, an agent searches the web for
"what's new at <competitor>" and writes the briefing; no per-source polling.
Least code (the research tools exist). Rejected as the base: it misses things,
cannot say "the pricing page changed on Tuesday", costs more per run, and its
claims are hard to verify. Its one good idea is kept: the rolling summary is
written by the model, but only from stored moves.

Chosen: **B**. Picked by Claude; the customer-facing shape was the user's.

## Design

### Data

Three tables, one migration.

- `competitors`: workspace, name, website, status (`active` / `paused`),
  rolling summary text and when it was written. Unique on (workspace, name).
- `competitor_sources`: competitor, kind, url or handle, last checked, last
  content hash, last snapshot text (pages only), consecutive failures.
  Kinds: `x`, `linkedin`, `page_landing`, `page_pricing`, `changelog`, `blog`,
  `github`. Unique on (competitor, kind, url).
- `competitor_moves`: competitor, source, kind (`launch`, `pricing`,
  `positioning`, `news`), title, one-paragraph what-happened, source URL,
  happened-at, impact text, suggested action, linked demand theme id
  (nullable), importance (`high` / `normal`), a dedupe key. Unique on
  (competitor, dedupe key).

`brand_profiles.competitors` stays as it is (keyword generation reads it).

### Finding the competitors (no customer input)

The customer is never asked to type a competitor (decided 2026-11-15).
LeadEcho sets the list up itself and starts watching.

What is stored today is **names only**: `brand_profiles.competitors` is a
list of names from the onboarding product analysis, with the Exa
"<product> alternatives" search as the fallback when the analysis finds none.
That search does return each competitor's website, but
`ExaCompetitorFinder.FindCompetitors` discards it and keeps the name. So the
names exist; the websites do not.

Setup, run once when onboarding saves the brand profile, and once as a
backfill for existing workspaces:

1. Take the names in `brand_profiles.competitors`.
2. Resolve each name to a website. Change the finder to return name and URL
   together so the fallback path keeps what it already has; for names that
   came from the product analysis, one Exa company search per name
   ("<name>", category company), taking the top hit that is not an
   aggregator and not the customer's own site.
3. A name that cannot be resolved to a website with confidence is not
   watched. It is listed on the page as "couldn't find their site".
4. Create the competitor rows, up to the plan cap, and run source discovery
   on each.

Kept up to date: demand themes already record the products people name when
they describe a need. A product named by several people that is not on the
list is added as a suggestion on the page, one click to start watching. It is
not auto-watched, because people name all kinds of tools.

The customer can remove a competitor, fix a wrong website, or add one by
hand. None of that is required for the feature to work.

### Finding a competitor's sources

Sources are saved and watched without confirmation; the customer can edit
them later. Given a website, fetch the homepage with `SafeFetcher` and read
its links:

- X and LinkedIn: links to `x.com` / `twitter.com` / `linkedin.com/company`.
- GitHub: a link to `github.com/<org>`.
- Changelog, blog, pricing: links whose path contains `changelog`,
  `whats-new`, `releases`, `updates`, `blog`, `pricing`.
- Feeds: `<link rel="alternate" type="application/rss+xml|atom+xml">` on the
  changelog and blog pages.

Because nobody confirms the result, discovery prefers missing a source to
watching a wrong one: a social or GitHub link is taken only from the
competitor's own homepage, and only when there is exactly one candidate.

### Watchers

One due-list loop in the monitor tick, next to `pollTrackedProfiles`, bounded
per tick. Each source is checked at most once a day.

| Kind | How | New item means |
|---|---|---|
| `x`, `linkedin` | treg user-posts tools, as tracked profiles do | a post id not seen before |
| `changelog`, `blog` with a feed | fetch RSS / Atom, parse with `encoding/xml` | an entry id not seen before |
| `changelog`, `blog` without a feed | fetch page, extract text, compare hash | text changed; the new text is the item |
| `page_landing`, `page_pricing` | fetch page, extract text, compare hash | text changed; old and new text are the item |
| `github` | `https://github.com/<org>/<repo>/releases.atom`, up to 3 repos per competitor | a release id not seen before |

Rules:

- The first check of any source only records a baseline. It produces no moves,
  so adding a competitor does not flood the page with history.
- Page text is normalised before hashing (whitespace, and lines that are only
  a date or a counter) so cosmetic churn does not count as a change.
- A source that fails five times running is marked broken and shown as such on
  the page; it stops being polled until the customer fixes or removes it.
- Fetches go through `SafeFetcher`. Competitor URLs are customer-supplied and
  are untrusted input.

### From items to moves

New items for one competitor from one poll go to the model in a single call,
with the workspace's product profile and its open demand themes. The model
returns, per item, either "not a move" or a move: kind, title, what happened,
which demand theme it bears on (by id, or none), the impact on this customer
in one or two sentences, one suggested action, and importance.

- Fetched text is untrusted. The prompt says so, as the demand prompts do, and
  the output is parsed as strict JSON with unknown theme ids discarded.
- `importance: high` is reserved for pricing changes and for a launch that
  lands on a demand theme the customer has marked `gap` or `partial`.
- Moves that describe the same event from two sources (a tweet and a changelog
  entry) are merged: the model is shown the competitor's moves from the last
  14 days and may answer "same as move N", which adds the URL to that move.

### Rolling summary

When a competitor has new moves, its summary is rewritten from its moves of
the last 90 days: three to five sentences on what they are building and where
they are heading. Written only from stored moves. No moves in 90 days: the
summary says nothing has been observed.

### People attached to a competitor

On the competitor's page, list recent mentions tagged `competitor_complaint`
or `seeking_alternative` whose text names that competitor. This is a read-time
query, not new storage. They open in the inbox as they do today.

### Where the customer sees it

**Competitors page** (`/competitors`):

```
Competitors                                         [+ Add competitor]

Acme            3 moves this month   6 people unhappy   7 sources ok
  Building: bulk export and a public API; pushing upmarket (SSO, audit log).
  ┌ HIGH  Pricing  Nov 12  Starter plan raised from $29 to $49
  │       For you: your $39 plan is now cheaper than their entry tier.
  │       Do: say so on the pricing page; 6 people complaining this week →
  ├ Launch  Nov 10  Bulk CSV export
  │       For you: 14 people asked for this; you're marked "gap". See theme →
  └ Positioning  Nov 3  Homepage now leads with "for agencies"

Beta Co         no moves in 30 days   1 source broken (blog)
```

Clicking a competitor opens its timeline, its sources (edit, pause, fix), and
its unhappy people.

**Weekly digest:** a "Competitor moves" section listing high-importance moves
first, at most five, each with its one-line impact. Omitted when empty.

**Alert:** a high-importance move goes out through the existing alert
channels, once.

**Demand page:** a theme that a move is linked to shows "Acme shipped this on
Nov 10" under it.

### Limits and cost

- Competitors per workspace are capped by plan (proposed: 3 on free, 10 paid).
- **Free plan and social sources (decided 2026-11-15).** Hosted free
  workspaces cannot make treg calls today (`PaidDataMeter.Spend` returns
  `ErrNoPaidData`; X and LinkedIn are in `PaidPlatforms`). Free gets pages,
  feeds and GitHub; X and LinkedIn show as locked with an upgrade prompt. A
  paid-data refusal never counts toward the five-failures rule.
- **Test workspace:** the Utkrusht workspace is the one to build and check
  this against.
- Sources per competitor capped at 10; GitHub repos at 3.
- Each source is polled once a day; treg calls and model calls are charged in
  credits like other paid actions. A workspace that is out of credits stops
  being polled and the page says so.
- Rough scale: 5 competitors with 2 social sources each is about 300 treg
  calls a month, well under a dollar at current treg prices. Page and feed
  fetches are free. The model is called only when something is new.

## Build order

Each phase is usable on its own.

1. **Competitors and sources.** Tables, automatic setup from the brand
   profile (name to website, then source discovery), backfill for existing
   workspaces, the page with an empty timeline, remove / fix / add by hand.
2. **Watchers.** The poll loop and the five source kinds, storing raw new
   items, baseline rule, broken-source handling.
3. **Moves.** The model step, merging, the timeline, the link to demand themes.
4. **Briefing.** Rolling summary, digest section, high-importance alert.
5. **People.** Unhappy-people list on the page, the note on the demand theme.

## Left to the builder

- Table and column names, query shapes, package layout (a new
  `internal/competitors` package is expected).
- The exact text-normalisation rules for page hashing; tune on real pages.
- Prompt wording.
- Credit cost per poll and per model call, in line with `billing/plans.go`.
- Whether raw new items are kept in a table or passed straight to the model
  step within one poll.

## Open points to verify before building

- **LinkedIn company pages.** The treg tool used today
  (`crawl4ai.linkedin.user.posts`) takes a person's profile. Competitors
  publish from company pages. Confirm treg has a company-posts tool and its
  price; if not, LinkedIn is limited to named people (a founder) at first.
  Settle this before phase 1: it decides whether discovery looks for
  `linkedin.com/company` or `linkedin.com/in` links.
- **JavaScript-only pages.** `SafeFetcher` reads server-rendered HTML. A
  pricing page rendered entirely in the browser will come back empty. Detect
  the empty case and mark the source broken rather than hashing nothing; the
  scrapling sidecar is the fallback if this turns out to be common.
- **Naming a competitor in a mention.** The people list matches on the
  competitor's name in the text. Short or common names ("Notion", "Linear")
  will over-match; check against real data before shipping phase 5.
