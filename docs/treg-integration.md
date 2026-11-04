# treg integration notes

Read from https://treg.to/integrate.md, https://treg.to/llms.txt, https://treg.to/skill.md and the
public catalog (`GET /catalog/search?q=…`, `GET /catalog/endpoints/<id>`) on 2026-11-04. No paid
calls were made; response bodies below are what the docs and catalog describe, not captured traffic.

## Call shape

- `POST https://treg.to/call/<endpoint-id>` with a JSON body (catalog tools whose `method` is POST).
  GET tools take query params instead. Provider body is relayed verbatim, including its errors.
- Auth: org-scoped token in `X-Treg-Token` (no `X-Treg-Org` needed). Identity tokens may answer
  `400 choose an org` -- always mint an org token for the product (`treg org agent-new <name>`).
- `X-Treg-Meta: customer=<workspace_id>` -- up to 5 `k=v` pairs, values `[A-Za-z0-9._:-]`, <=128
  chars, not email-like. Malformed -> unbilled `422` before relay. UUIDs are valid values.
- `X-Treg-Route-Max-Cost: <usd>` -- ceiling for one call, in **USD** (decimal), not micro. Over it:
  `402 {"error":"route_max_cost"}`, nothing charged. Routed tools default to $1 per call.
- `Idempotency-Key: <key>` -- 24h per caller. A replay returns the stored answer with
  `X-Treg-Idempotent-Replay: true`, `X-Treg-Cost-Micro: 0`, `X-Treg-Original-Cost-Micro`. A failed call
  frees its key. `409` = original still running (retry shortly). `410 idempotency_*` = may have been
  charged, answer lost: do not blindly retry. New work needs a new key (same key + different body = 422).

Response headers we read:

- `X-Treg-Cost-Micro` -- integer micro-USD charged. Absent = ran on the team's own key, not billed.
- `X-Treg-Call-Id` -- stored in our `treg_calls` ledger; resolves at `GET /calls/<id>` and
  `GET /calls/<id>/result` (recover a paid answer).
- `X-Treg-Error: 1` -- the refusal is treg's own (branch on this, not on status).
- `X-Treg-Served-By` -- provider that answered a routed call.

## Errors and what LeadEcho does

| status / error | whose | LeadEcho |
|---|---|---|
| 402 `insufficient_balance` (body has balance + topup_url) | team | `treg.ErrInsufficientBalance`, logged at error with `alert=treg`, client pauses 5 min, fetchers fall back silently. Never shown to users. |
| 429 `platform_daily_cap_reached` | team | `treg.ErrPlatformDailyCap`, same handling. |
| 402 `route_max_cost` | ours | `*treg.APIError`, no retry; raise `TREG_MAX_COST_MICRO` or shrink the request. |
| 429 `tag_spend_cap_reached` / 403 `tag_blocked` | per customer | `*treg.APIError`; safe to surface, but fetchers just skip. |
| 409 idempotency in progress, 5xx (not `route_failed`), timeouts | -- | retry with backoff, **same** `Idempotency-Key`. |
| 502 `route_failed` | every provider missed | no retry (misses can bill; body has `charged_micro`). |
| other 4xx | provider/us | no retry. |

## Budgets and usage (admin token)

- `PUT /orgs/<org_id>/budgets/customer/<workspace_id>` `{"daily_cap_micro": N}` (or `{"status":"blocked"}`).
  Omit the value for a default for every customer. Caps are advisory (can overshoot by ~1 call per
  in-flight request). -> `Client.SetCustomerDailyBudget`.
- `GET /orgs/<org_id>/usage/by-tag?key=customer&days=30` ->
  `{key, days, rows:[{value, charged_micro, charged_usd, calls}], attributed_micro, unattributed_micro, total_micro}`.
  Invoices must come from here; assert `attributed + unattributed == total`. -> `Client.UsageByTag`,
  `UsageByTag.Reconciled()`.

## Tools chosen

| job | tool | price (catalog) | body | response |
|---|---|---|---|---|
| X keyword search | `treg.x.search.posts` (routed: anyapi -> tikhub -> justoneapi, cheapest first) | $0.00075 per hit, misses free | `{"q": "<term>"}` | `{output:{posts:[…provider-native rows…]}, raw, _treg:{served_by, tried, charged_micro}}`; miss = `output.posts == []` |
| LinkedIn post keyword search | `anyapi.linkedin.search.posts` | $0.002 per success (misses free) | `{"query": "<term>", "datePosted": "last-week"}` | provider body verbatim (text, link, publish date per row) |
| Reddit comments | **not via treg**: Reddit public JSON `/r/<sub>/comments.json` (free) | $0 | -- | standard Reddit listing |

LinkedIn alternatives considered: `scrapecreators.x.v1-linkedin-search-posts` ($0.00188 but
`per_call`, so misses bill and it goes through Google Search), `harvestapi.linkedin.post.search`
($0.004 per call). `anyapi` is per-success, so it is cheapest per useful answer.

Rows are provider-native (treg does not normalise), so `internal/monitor/treg_parse.go` maps them
tolerantly: id `id|id_str|rest_id|tweetId|urn`, text `text|full_text|content|commentary`, url
`url|link|twitterUrl|postUrl`, author `author.{userName,username,screen_name,name}|username|…`,
time `createdAt|created_at|date|publishedAt|postedAt` (RFC3339, Twitter's ruby date, or unix secs).
Verify against real traffic once a token exists and tighten the mapping.

Reddit comments: `crawl4ai.reddit.post.comments` ($0.0009/post) needs a post URL per call, so a
comment search across a subreddit would cost one call per post. The public `comments.json` stream
gives the newest ~100 comments per subreddit for free; we filter locally, same as posts.

## Config

`TREG_TOKEN` (enables treg; unset = old Pinchtab/Camoufox/Scrapling paths), `TREG_ORG_ID` (budgets/
usage helpers only), `TREG_BASE_URL` (default `https://treg.to`), `TREG_MAX_COST_MICRO` (per-call
ceiling, default 20000 = $0.02), `REDDIT_COMMENTS_ENABLED` (default true).
