# treg vendor listing: LeadEcho (draft submission)

Prepared 2026-11-04 from https://treg.to/vendor-listing.md and the treg repo's `docs/VENDORS.md`,
`src/treg/oauth_providers.py` (HUNTER entry shape) and `src/treg/catalog/capabilities.yaml`.
Plan reference: `docs/competitive-analysis-snitchfeed-treg.md` section 5, item 18.

**Status: not submitted.** No PR opened, nobody contacted. The public API (`/api/public/v1/*`) and
workspace API keys are being built on another branch; every endpoint path below is the planned
shape and must be reconciled with the shipped routes before submission.

## 1. Eligibility check

| Requirement (treg) | LeadEcho | Status |
|---|---|---|
| Self-serve API keys, no sales call | Workspace API keys created in Settings -> API & MCP after self-serve sign-up | Pending: keys ship with the MCP/API branch |
| Key in a header or query param, never the path | `Authorization: Bearer <key>` header | OK by design |
| Free probe that rejects a bad key distinguishably | `GET /api/public/v1/probe`: 200 + workspace id for a valid key, 401 JSON for an invalid one | Pending: must be observed live and quoted from the wire |
| Published pricing page, stable URL | https://leadecho.app/#pricing | Exists; update it to match the final plan pricing first |
| Docs with example parameter values | https://leadecho.app/docs/api-reference | Needs a section for `/api/public/v1` with example values |

Preferred extras: an OpenAPI spec at `https://leadecho.app/api/public/v1/openapi.json` (not built),
no rate-card endpoint (our API is subscription-billed, so cost per call is `0` within plan limits),
documented rate limits (to write).

Do not submit until every "Pending" row is done.

## 2. Registry entry (`src/treg/oauth_providers.py`)

```python
LEADECHO = OAuthProvider(
    service="leadecho",
    display_name="LeadEcho",
    auth_kind="key",
    token_label="Workspace API key",
    token_placeholder="your LeadEcho workspace API key",
    # Header keeps the key out of logged URLs.
    token_header="Authorization",
    token_format="Bearer {secret}",
    setup_url="https://leadecho.app/app/settings",
    setup_action_label="Get your LeadEcho API key",
    setup_steps=(
        "Sign in to LeadEcho and open Settings -> API & MCP.",
        "Create a key, give it a name, and copy it (it is shown once).",
    ),
    setup_note="Calls are included in your LeadEcho plan; the probe is free.",
    auth_uri="", token_uri="",
    scopes={},
    client_id_setting="", client_secret_setting="",
    category="Social media",
    summary="Buying-intent social mentions from Reddit, HN, X, LinkedIn and Bluesky, scored, with grounded reply drafts and reply-to-revenue attribution.",
    base_url="https://leadecho.app/api/public/v1",
    docs_url="https://leadecho.app/docs/api-reference",
    probe_path="/probe",  # free; a bad key gets 401
)
```

Add `LEADECHO` to the provider tuple that builds `REGISTRY`, and a neutral lettermark at
`src/treg/web/logos/leadecho.svg`:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="12" fill="#444"/><text x="32" y="42" font-family="sans-serif" font-size="28" font-weight="700" text-anchor="middle" fill="#fff">LE</text></svg>
```

## 3. Core catalog file (`src/treg/catalog/leadecho.yaml`)

No existing capability covers "scored buying-intent mentions across platforms" (the closest are
`reddit.search.posts` / `x.search.posts`, which are raw platform search, and `ai-search.mentions.*`,
which is AI answer engines). So the file proposes a `social-listening` platform. Probe reuses
`account.identity`.

```yaml
provider: leadecho
source:
  docs: https://leadecho.app/docs/api-reference
  curated: 2026-11-04
limits: "TODO: per-key rate limit once set"
pricing_url: https://leadecho.app/#pricing
proposed_platforms:
  social-listening: {label: "Social listening & buying intent", category: "Social media", summary: "Scored mentions of people asking for a product like yours, across Reddit, HN, X, LinkedIn and Bluesky."}
proposed_capabilities:
  social-listening.mentions.search: "Search scored buying-intent mentions by keyword"
  social-listening.mentions.get: "Get one mention with its intent score"
  social-listening.leads.list: "List leads found from social mentions"
  social-listening.leads.get: "Get one lead with its posts, replies and clicks"
  social-listening.listeners.create: "Start monitoring keywords for buying intent"
  social-listening.listeners.update: "Change a listener's keywords or filters"
  social-listening.replies.draft: "Draft a reply to a mention from your own docs"
  social-listening.attribution.report: "Report clicks and revenue from social replies"
  social-listening.summary.weekly: "Summarize the week's leads and attributed revenue"
endpoints:
  - id: leadecho.account.probe
    capability: account.identity
    platform: account
    scope: own_account
    method: GET
    path: /probe
    name: "Check API key"
    summary: "Return the workspace the key belongs to"
    input: {}
    test_request: {}
    cost: {type: free, value: 0, currency: USD, source: docs, source_url: https://leadecho.app/#pricing, checked: 2026-11-04, confidence: documented}
    docs_url: https://leadecho.app/docs/api-reference
  - id: leadecho.mentions.search
    capability: social-listening.mentions.search
    platform: social-listening
    scope: own_account
    method: GET
    path: /mentions
    name: "Search mentions"
    summary: "Search scored mentions in your workspace"
    input:
      queryParams:
        q:        {type: string,  required: false, note: "full-text filter", example: "alternative"}
        platform: {type: string,  required: false, note: "reddit | hackernews | x | linkedin | bluesky | devto", example: "reddit"}
        intent:   {type: string,  required: false, note: "e.g. buying_intent, recommendation_request", example: "buying_intent"}
        since:    {type: string,  required: false, note: "ISO date", example: "2026-10-28"}
        limit:    {type: integer, required: false, note: "1-100, default 20", example: 1}
    test_request: {queryParams: {limit: 1}}
    cost: {type: per_call, value: 0, currency: USD, source: docs, source_url: https://leadecho.app/#pricing, checked: 2026-11-04, confidence: documented}
    docs_url: https://leadecho.app/docs/api-reference
  - id: leadecho.mentions.get
    capability: social-listening.mentions.get
    platform: social-listening
    scope: own_account
    method: GET
    path: /mentions/{id}
    name: "Get mention"
    summary: "One mention with intent, fit score and author"
    input: {pathParams: {id: {type: string, required: true, example: "TODO: id from mentions.search"}}}
    test_request: {pathParams: {id: "TODO"}}
    cost: {type: per_call, value: 0, currency: USD, source: docs, source_url: https://leadecho.app/#pricing, checked: 2026-11-04, confidence: documented}
  - id: leadecho.leads.list
    capability: social-listening.leads.list
    platform: social-listening
    scope: own_account
    method: GET
    path: /leads
    name: "List leads"
    summary: "Leads by stage and fit"
    input:
      queryParams:
        stage: {type: string,  required: false, note: "prospect | qualified | engaged | converted | lost", example: "prospect"}
        limit: {type: integer, required: false, example: 1}
    test_request: {queryParams: {limit: 1}}
    cost: {type: per_call, value: 0, currency: USD, source: docs, source_url: https://leadecho.app/#pricing, checked: 2026-11-04, confidence: documented}
  - id: leadecho.leads.get
    capability: social-listening.leads.get
    platform: social-listening
    scope: own_account
    method: GET
    path: /leads/{id}
    name: "Get lead"
    summary: "One lead with its mentions, replies, clicks and conversions"
    input: {pathParams: {id: {type: string, required: true, example: "TODO: id from leads.list"}}}
    test_request: {pathParams: {id: "TODO"}}
    cost: {type: per_call, value: 0, currency: USD, source: docs, source_url: https://leadecho.app/#pricing, checked: 2026-11-04, confidence: documented}
  - id: leadecho.listeners.create
    capability: social-listening.listeners.create
    platform: social-listening
    scope: own_account
    method: POST
    path: /listeners
    name: "Create listener"
    summary: "Monitor keywords with plain-English looks-for and skips"
    input:
      body:
        name:      {type: string, required: true,  example: "treg test listener"}
        keywords:  {type: array,  required: true,  example: ["alternative to snitchfeed"]}
        looks_for: {type: string, required: false, example: "people asking for a social listening tool"}
        skips:     {type: string, required: false, example: "vendor promotions"}
        platforms: {type: array,  required: false, example: ["reddit", "hackernews"]}
    test_request: {body: {name: "treg test listener", keywords: ["alternative to snitchfeed"], platforms: ["hackernews"]}}
    cost: {type: per_call, value: 0, currency: USD, source: docs, source_url: https://leadecho.app/#pricing, checked: 2026-11-04, confidence: documented}
  - id: leadecho.listeners.update
    capability: social-listening.listeners.update
    platform: social-listening
    scope: own_account
    method: PATCH
    path: /listeners/{id}
    name: "Tune listener"
    summary: "Change keywords, looks-for or skips"
    input: {pathParams: {id: {type: string, required: true, example: "TODO"}}, body: {skips: {type: string, required: false, example: "hiring posts"}}}
    test_request: {pathParams: {id: "TODO"}, body: {skips: "hiring posts"}}
    cost: {type: per_call, value: 0, currency: USD, source: docs, source_url: https://leadecho.app/#pricing, checked: 2026-11-04, confidence: documented}
  - id: leadecho.replies.draft
    capability: social-listening.replies.draft
    platform: social-listening
    scope: own_account
    method: POST
    path: /mentions/{id}/draft-reply
    name: "Draft reply"
    summary: "A reply grounded in your knowledge base, with a tracked link"
    input: {pathParams: {id: {type: string, required: true, example: "TODO"}}}
    test_request: {pathParams: {id: "TODO"}}
    cost: {type: per_call, value: 0, currency: USD, source: docs, source_url: https://leadecho.app/#pricing, checked: 2026-11-04, confidence: documented, note: "LLM cost is on the workspace's own model key (BYOK)"}
  - id: leadecho.attribution.report
    capability: social-listening.attribution.report
    platform: social-listening
    scope: own_account
    method: GET
    path: /attribution
    name: "Attribution report"
    summary: "Replies, clicks, conversions and revenue by keyword, platform or listener"
    input:
      queryParams:
        group_by: {type: string,  required: false, note: "keyword | platform | listener", example: "keyword"}
        days:     {type: integer, required: false, example: 7}
    test_request: {queryParams: {group_by: "keyword", days: 7}}
    cost: {type: per_call, value: 0, currency: USD, source: docs, source_url: https://leadecho.app/#pricing, checked: 2026-11-04, confidence: documented}
  - id: leadecho.summary.weekly
    capability: social-listening.summary.weekly
    platform: social-listening
    scope: own_account
    method: GET
    path: /summary/weekly
    name: "Weekly summary"
    summary: "The week's buyers, complaints, noisy keywords and attributed revenue"
    input: {}
    test_request: {}
    cost: {type: per_call, value: 0, currency: USD, source: docs, source_url: https://leadecho.app/#pricing, checked: 2026-11-04, confidence: documented}
```

Before the PR: run `uv run --frozen python scripts/catalog_added.py`, `scripts/catalog_validate.py`
and the test suite as `vendor-listing.md` says. Do not add `verified:` stamps or example responses;
never put a key in the diff.

Open question for review: every endpoint is `own_account` (it reads the caller's workspace), so the
listing is discovery + BYOK only, not "platform-eligible". That matches how we bill (subscription).

## 4. PR description (draft)

```
Add LeadEcho (social listening / buying-intent mentions)

Contact: hello@leadecho.app

LeadEcho finds people on Reddit, Hacker News, X, LinkedIn and Bluesky asking for a product
like yours, scores the intent, drafts a reply from your own docs, and reports clicks and
revenue from those replies. Agents use it to run social lead generation end to end.

Auth: workspace API key as an Authorization: Bearer header. Self-serve: sign up at
https://leadecho.app/app/register, Settings -> API & MCP.

Probe: GET /api/public/v1/probe
  valid key   -> TODO status + body, observed YYYY-MM-DD
  garbage key -> TODO status + body, observed YYYY-MM-DD (expected 401)

Pricing: https://leadecho.app/#pricing. Subscription; API calls are included in the plan,
so catalog cost is 0 per call. No rate-card endpoint. OpenAPI: TODO (none yet).

Self-verification ledger (fill from live calls with our own key):
  leadecho.account.probe         TODO status | claims $0 | meter $0 | date
  leadecho.mentions.search       TODO
  leadecho.mentions.get          TODO
  leadecho.leads.list            TODO
  leadecho.leads.get             TODO
  leadecho.listeners.create      TODO
  leadecho.listeners.update      TODO
  leadecho.replies.draft         TODO
  leadecho.attribution.report    TODO
  leadecho.summary.weekly        TODO

Full-surface map:
  catalogued: probe, mentions search/get, leads list/get, listeners create/update,
              draft reply, attribution report, weekly summary
  excluded:   queue_reply / push_to_crm / set_stage / mute (write actions with side effects
              on a customer's CRM or public posting; agents should use them through the
              LeadEcho MCP server where the user confirms), API key management (account admin),
              conversion webhook (inbound from the customer's site, not agent-facing),
              UTM redirect /r/{code} (public link target, not an API).

Proposed: platform `social-listening` and 9 capabilities (see leadecho.yaml); no existing
capability covers scored cross-platform intent mentions.
```

## 5. Checklist before submitting

- [ ] Public API and workspace keys shipped; paths above match the real routes
- [ ] Probe bad-key behavior captured live (status + body + date)
- [ ] `/api/public/v1` section with example values in the API reference docs
- [ ] Pricing page reflects the final plans
- [ ] Ledger filled from live calls
- [ ] Clone `superdesigndev/treg`, re-read `docs/VENDORS.md` and `.claude/skills/vendor-listing/SKILL.md`, rebase on `main`, run validators
