# LeadEcho - API Design Specification

## Table of Contents

1. [API Design Principles](#1-api-design-principles)
2. [Authentication & Authorization](#2-authentication--authorization)
3. [Common Patterns](#3-common-patterns)
4. [Endpoint Specification](#4-endpoint-specification)
   - [Mentions](#41-mentions)
   - [Leads](#42-leads)
   - [Keywords](#43-keywords)
   - [Knowledge Base (RAG)](#44-knowledge-base-rag)
   - [Analytics](#45-analytics)
   - [Workflows](#46-workflows)
   - [Extension](#47-extension)
   - [Webhooks](#48-webhooks)
   - [Team](#49-team)
5. [SSE Event Specification](#5-sse-event-specification)
6. [Webhook Output](#6-webhook-output)
7. [Rate Limiting Strategy](#7-rate-limiting-strategy)

---

## 1. API Design Principles

### Base URL

All API endpoints are served under a versioned prefix:

```
https://api.leadecho.app/api/v1/
```

### Core Principles

| Principle | Implementation |
|-----------|---------------|
| **RESTful** | Resources as nouns, HTTP methods as verbs. Predictable URL structure. |
| **JSON everywhere** | All request and response bodies use `application/json` unless explicitly noted (e.g., file upload). |
| **Versioned** | All endpoints prefixed with `/api/v1/`. Breaking changes require a new version. |
| **Consistent errors** | Every error response follows the same envelope format regardless of endpoint. |
| **Workspace-scoped** | All data endpoints are implicitly scoped to the authenticated user's active workspace (Clerk organization). |
| **Idempotent where possible** | PUT and DELETE operations are idempotent. POST endpoints that create resources return the created resource with its ID. |

### HTTP Methods

| Method | Usage |
|--------|-------|
| `GET` | Retrieve a resource or list of resources. Never mutates state. |
| `POST` | Create a new resource, or trigger an action (e.g., search, reply). |
| `PATCH` | Partial update of an existing resource. Only provided fields are changed. |
| `DELETE` | Remove a resource. Returns `204 No Content` on success. |

### HTTP Status Codes

| Code | Meaning | When Used |
|------|---------|-----------|
| `200` | OK | Successful GET, PATCH |
| `201` | Created | Successful POST that creates a resource |
| `204` | No Content | Successful DELETE |
| `400` | Bad Request | Malformed JSON, missing required fields, validation failure |
| `401` | Unauthorized | Missing or invalid authentication token |
| `403` | Forbidden | Valid auth but insufficient permissions (wrong role, wrong workspace) |
| `404` | Not Found | Resource does not exist or is not accessible in the current workspace |
| `409` | Conflict | Duplicate resource (e.g., keyword already exists) |
| `422` | Unprocessable Entity | Semantically invalid request (e.g., invalid enum value for status) |
| `429` | Too Many Requests | Rate limit exceeded |
| `500` | Internal Server Error | Unexpected server failure |

### Request Headers

| Header | Required | Description |
|--------|----------|-------------|
| `Authorization` | Yes | `Bearer <clerk_jwt_token>` or `X-API-Key <key>` |
| `Content-Type` | For POST/PATCH | `application/json` (or `multipart/form-data` for file uploads) |
| `Accept` | No | Defaults to `application/json` |
| `X-Request-ID` | No | Client-generated request ID for tracing. Server generates one if omitted. |

### Response Headers

Every response includes:

| Header | Description |
|--------|-------------|
| `Content-Type` | `application/json` |
| `X-Request-ID` | Unique request identifier (echoed from client or server-generated) |
| `X-RateLimit-Limit` | Maximum requests allowed in the current window |
| `X-RateLimit-Remaining` | Requests remaining in the current window |
| `X-RateLimit-Reset` | Unix timestamp when the rate limit window resets |

---

## 2. Authentication & Authorization

### Clerk JWT Tokens (Primary)

All dashboard and extension API calls authenticate via Clerk-issued JWT tokens. The token is passed in the `Authorization` header.

```
Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Token validation flow (Go middleware):**

1. Extract `Bearer <token>` from `Authorization` header
2. Verify JWT signature against Clerk's JWKS endpoint (cached)
3. Validate `exp`, `nbf`, `iss` claims
4. Extract `sub` (user ID) and `org_id` (workspace/organization ID) from claims
5. If `org_id` is present, scope all subsequent queries to that workspace
6. If `org_id` is absent, return `403` for any workspace-scoped endpoint

**Clerk JWT claims used:**

```json
{
  "sub": "user_2abc123def",
  "org_id": "org_xyz789",
  "org_role": "org:admin",
  "org_permissions": ["org:mentions:read", "org:mentions:reply", "org:leads:manage"],
  "exp": 1740000000,
  "iss": "https://clerk.leadecho.app"
}
```

### Role-Based Access Control

Clerk organizations map directly to LeadEcho workspaces. Three roles are defined:

| Role | Permissions | Use Case |
|------|------------|----------|
| `admin` | Full access: manage team, billing, keywords, workflows, reply, delete | Workspace owner, account admin |
| `editor` | Read all, create/edit mentions and leads, draft and submit replies, manage keywords | Sales reps, marketers |
| `viewer` | Read-only access to mentions, leads, analytics | Monitoring-only team members |

**Permission matrix:**

| Action | `admin` | `editor` | `viewer` |
|--------|---------|----------|----------|
| View mentions/leads/analytics | Yes | Yes | Yes |
| Update mention status | Yes | Yes | No |
| Submit replies | Yes | Yes | No |
| Manage keywords | Yes | Yes | No |
| Manage documents (RAG) | Yes | Yes | No |
| Manage workflows | Yes | Yes | No |
| Manage team members/roles | Yes | No | No |
| Manage billing/settings | Yes | No | No |
| Delete resources | Yes | No | No |

### API Keys (Extension & Webhooks)

For contexts where JWT authentication is impractical:

**Extension API Key:**

The Chrome extension authenticates using a per-user API key stored in `chrome.storage.local`. The key is generated from the dashboard settings page.

```
Authorization: X-API-Key leh_ext_a1b2c3d4e5f6...
```

API keys are scoped to a single user and workspace. They carry the same permissions as the user's role.

**Webhook Signing:**

Inbound webhooks (Clerk, Stripe) are validated via provider-specific signatures rather than Bearer tokens:

- **Clerk webhooks:** Verified using Svix signature headers (`svix-id`, `svix-timestamp`, `svix-signature`)
- **Stripe webhooks:** Verified using `Stripe-Signature` header with the endpoint secret

---

## 3. Common Patterns

### 3.1 Pagination

Two pagination strategies are used depending on the data characteristics:

#### Cursor-Based Pagination (Real-Time Data)

Used for: **Mentions**, **Lead Events**, **Workflow Executions** -- data where new records are continuously inserted and offset-based pagination would produce inconsistent results.

**Request:**

```
GET /api/v1/mentions?limit=25&cursor=eyJpZCI6ImFiYzEyMyIsImNyZWF0ZWRfYXQiOiIyMDI2LTAyLTIzVDE0OjMwOjAwWiJ9
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | integer | `25` | Number of items to return (max `100`) |
| `cursor` | string | `null` | Opaque cursor from previous response. Omit for first page. |

**Response envelope:**

```json
{
  "data": [],
  "pagination": {
    "next_cursor": "eyJpZCI6ImRlZjQ1NiIsImNyZWF0ZWRfYXQiOiIyMDI2LTAyLTIzVDE0OjI5OjAwWiJ9",
    "has_more": true
  }
}
```

The cursor is a base64-encoded JSON object containing the sort key(s) of the last item. The server decodes it to construct a `WHERE ... AND (created_at, id) < ($cursor_created_at, $cursor_id)` clause for stable keyset pagination.

#### Offset-Based Pagination (Static/Aggregated Data)

Used for: **Analytics**, **Keywords**, **Documents**, **Team Members** -- data that changes infrequently and where users benefit from knowing total count and page numbers.

**Request:**

```
GET /api/v1/keywords?page=2&per_page=20
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page` | integer | `1` | Page number (1-indexed) |
| `per_page` | integer | `20` | Items per page (max `100`) |

**Response envelope:**

```json
{
  "data": [],
  "pagination": {
    "page": 2,
    "per_page": 20,
    "total": 87,
    "total_pages": 5
  }
}
```

### 3.2 Filtering

Filters are passed as query parameters. Multiple values for the same filter are comma-separated.

**Common filter parameters:**

| Parameter | Type | Example | Used By |
|-----------|------|---------|---------|
| `status` | enum (comma-separated) | `status=new,reviewed` | Mentions, Leads |
| `platform` | enum (comma-separated) | `platform=reddit,hackernews` | Mentions, Analytics |
| `date_from` | ISO 8601 date | `date_from=2026-02-01` | Mentions, Analytics, Leads |
| `date_to` | ISO 8601 date | `date_to=2026-02-23` | Mentions, Analytics, Leads |
| `keyword_id` | UUID | `keyword_id=550e8400-e29b-41d4-a716-446655440000` | Mentions |
| `min_score` | float (0-10) | `min_score=7.0` | Mentions |
| `q` | string | `q=react+deployment` | Mentions (full-text search) |
| `stage` | enum | `stage=engaged` | Leads |
| `assigned_to` | UUID | `assigned_to=user_2abc123` | Mentions, Leads |

**Platform enum values:** `reddit`, `hackernews`, `twitter`, `linkedin`

**Mention status enum values:** `new`, `reviewed`, `replied`, `archived`, `spam`

**Lead stage enum values:** `detected`, `engaged`, `replied`, `clicked`, `converted`, `lost`

### 3.3 Sorting

Sorting is controlled via `sort` and `order` query parameters.

```
GET /api/v1/mentions?sort=relevance_score&order=desc
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `sort` | string | Resource-dependent | Field to sort by |
| `order` | enum | `desc` | `asc` or `desc` |

**Sortable fields by resource:**

| Resource | Sortable Fields | Default Sort |
|----------|----------------|--------------|
| Mentions | `created_at`, `relevance_score`, `platform` | `created_at desc` |
| Leads | `created_at`, `updated_at`, `estimated_value`, `stage` | `updated_at desc` |
| Keywords | `created_at`, `name`, `mention_count` | `created_at desc` |
| Documents | `created_at`, `name`, `size` | `created_at desc` |

### 3.4 Error Response Format

All errors follow a consistent envelope:

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "The request body contains invalid fields.",
    "details": {
      "fields": {
        "status": "Must be one of: new, reviewed, replied, archived, spam",
        "relevance_score": "Must be a number between 0 and 10"
      }
    }
  }
}
```

**Error code catalog:**

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `UNAUTHORIZED` | 401 | Missing or invalid authentication token |
| `FORBIDDEN` | 403 | Insufficient permissions for this action |
| `NOT_FOUND` | 404 | Resource does not exist in this workspace |
| `VALIDATION_FAILED` | 400 | Request body or query parameter validation failed |
| `INVALID_CURSOR` | 400 | Pagination cursor is malformed or expired |
| `DUPLICATE_RESOURCE` | 409 | Resource with the same unique key already exists |
| `UNPROCESSABLE_ENTITY` | 422 | Request is well-formed but semantically invalid |
| `RATE_LIMITED` | 429 | Rate limit exceeded for this endpoint or tier |
| `INTERNAL_ERROR` | 500 | Unexpected server error |
| `SERVICE_UNAVAILABLE` | 503 | Dependent service (Claude API, Redis) is unreachable |

**Rate limit error includes retry information:**

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "Rate limit exceeded. Try again in 45 seconds.",
    "details": {
      "limit": 100,
      "remaining": 0,
      "reset_at": "2026-02-23T14:31:00Z",
      "retry_after_seconds": 45
    }
  }
}
```

### 3.5 Rate Limiting Headers

Every API response includes rate limiting headers:

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 67
X-RateLimit-Reset: 1740321060
```

| Header | Description |
|--------|-------------|
| `X-RateLimit-Limit` | Maximum number of requests allowed in the current 60-second window |
| `X-RateLimit-Remaining` | Number of requests remaining before throttling |
| `X-RateLimit-Reset` | Unix timestamp (seconds) when the current window resets |

When `X-RateLimit-Remaining` reaches `0`, subsequent requests receive a `429` response until the window resets. The `Retry-After` header is also included with the number of seconds to wait.

### 3.6 Timestamps

All timestamps are in UTC, formatted as ISO 8601 with timezone designator:

```
2026-02-23T14:30:00Z
```

### 3.7 IDs

All resource IDs are UUIDs (v7, time-ordered) represented as strings:

```
"id": "01953a7c-8f00-7def-b234-567890abcdef"
```

---

## 4. Endpoint Specification

### 4.1 Mentions

Mentions are the core resource -- social media posts detected by the Signal Engine that match the workspace's tracked keywords.

---

#### `GET /api/v1/mentions`

List mentions with filtering, sorting, and cursor-based pagination.

**Auth:** Required (viewer+)

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | integer | No | Items per page (default `25`, max `100`) |
| `cursor` | string | No | Pagination cursor from previous response |
| `status` | string | No | Comma-separated: `new`, `reviewed`, `replied`, `archived`, `spam` |
| `platform` | string | No | Comma-separated: `reddit`, `hackernews`, `twitter`, `linkedin` |
| `keyword_id` | UUID | No | Filter by keyword |
| `min_score` | float | No | Minimum relevance score (0-10) |
| `date_from` | ISO 8601 | No | Start date (inclusive) |
| `date_to` | ISO 8601 | No | End date (inclusive) |
| `q` | string | No | Full-text search across mention content |
| `assigned_to` | string | No | Filter by assigned user ID |
| `sort` | string | No | `created_at`, `relevance_score` (default `created_at`) |
| `order` | string | No | `asc`, `desc` (default `desc`) |

**Example Request:**

```
GET /api/v1/mentions?status=new,reviewed&platform=reddit&min_score=7.0&sort=relevance_score&order=desc&limit=10
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...
```

**Example Response (`200 OK`):**

```json
{
  "data": [
    {
      "id": "01953a7c-8f00-7def-b234-567890abcdef",
      "platform": "reddit",
      "platform_id": "t1_kx4m2n9",
      "status": "new",
      "url": "https://reddit.com/r/SaaS/comments/1abc/looking_for_crm_alternative/kx4m2n9",
      "author": {
        "username": "startup_founder_23",
        "profile_url": "https://reddit.com/u/startup_founder_23",
        "karma": 4520,
        "account_age_days": 890
      },
      "content": "We've been using HubSpot but the pricing is getting out of control for our 10-person team. Looking for something that integrates well with our existing social selling workflow. Any recommendations?",
      "thread_title": "Looking for a CRM alternative that doesn't cost $500/mo",
      "subreddit": "r/SaaS",
      "keyword_matches": [
        {
          "keyword_id": "550e8400-e29b-41d4-a716-446655440000",
          "keyword": "CRM alternative",
          "match_type": "semantic"
        }
      ],
      "relevance_score": 9.2,
      "intent": "buying_signal",
      "sentiment": "frustrated",
      "thread_stats": {
        "upvotes": 47,
        "comments": 23,
        "age_hours": 4.5
      },
      "assigned_to": null,
      "reply_count": 0,
      "created_at": "2026-02-23T14:30:00Z",
      "detected_at": "2026-02-23T14:30:45Z"
    }
  ],
  "pagination": {
    "next_cursor": "eyJpZCI6IjAxOTUzYTdjLThmMDAtN2RlZi1iMjM0LTU2Nzg5MGFiY2RlZiIsImNyZWF0ZWRfYXQiOiIyMDI2LTAyLTIzVDE0OjMwOjAwWiJ9",
    "has_more": true
  }
}
```

---

#### `GET /api/v1/mentions/:id`

Retrieve a single mention with its full context including thread summary and AI analysis.

**Auth:** Required (viewer+)

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Mention ID |

**Example Request:**

```
GET /api/v1/mentions/01953a7c-8f00-7def-b234-567890abcdef
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...
```

**Example Response (`200 OK`):**

```json
{
  "data": {
    "id": "01953a7c-8f00-7def-b234-567890abcdef",
    "platform": "reddit",
    "platform_id": "t1_kx4m2n9",
    "status": "reviewed",
    "url": "https://reddit.com/r/SaaS/comments/1abc/looking_for_crm_alternative/kx4m2n9",
    "author": {
      "username": "startup_founder_23",
      "profile_url": "https://reddit.com/u/startup_founder_23",
      "karma": 4520,
      "account_age_days": 890
    },
    "content": "We've been using HubSpot but the pricing is getting out of control for our 10-person team. Looking for something that integrates well with our existing social selling workflow. Any recommendations?",
    "thread_title": "Looking for a CRM alternative that doesn't cost $500/mo",
    "subreddit": "r/SaaS",
    "keyword_matches": [
      {
        "keyword_id": "550e8400-e29b-41d4-a716-446655440000",
        "keyword": "CRM alternative",
        "match_type": "semantic"
      }
    ],
    "relevance_score": 9.2,
    "intent": "buying_signal",
    "sentiment": "frustrated",
    "thread_summary": "OP is a founder of a 10-person startup looking to replace HubSpot due to escalating costs. Thread has 23 comments. 3 users recommended Pipedrive, 2 mentioned Attio, 1 mentioned Close. No one has mentioned social selling integration as a feature. OP specifically asked about social selling workflow integration.",
    "ai_analysis": {
      "opportunity_type": "direct_need",
      "competitor_mentions": ["HubSpot", "Pipedrive", "Attio", "Close"],
      "pain_points": ["pricing", "social selling integration"],
      "recommended_approach": "value_first",
      "link_safe": true,
      "link_safety_reason": "Author has high karma, subreddit allows tool recommendations in context"
    },
    "thread_stats": {
      "upvotes": 47,
      "comments": 23,
      "age_hours": 4.5
    },
    "draft_replies": [
      {
        "id": "reply_draft_001",
        "variant": "value",
        "content": "We ran into the same problem scaling past 10 people on HubSpot. The key issue is most CRMs treat social selling as an afterthought...",
        "tone": "helpful_expert",
        "includes_link": false,
        "generated_at": "2026-02-23T14:35:00Z"
      },
      {
        "id": "reply_draft_002",
        "variant": "technical",
        "content": "If social selling integration is the priority, you want something that can pull signals from Reddit, X, and LinkedIn into one pipeline...",
        "tone": "technical",
        "includes_link": false,
        "generated_at": "2026-02-23T14:35:00Z"
      },
      {
        "id": "reply_draft_003",
        "variant": "soft_sell",
        "content": "We switched from HubSpot to a tool that actually tracks social engagement ROI. The difference was night and day for our team...",
        "tone": "casual",
        "includes_link": true,
        "generated_at": "2026-02-23T14:35:00Z"
      }
    ],
    "assigned_to": "user_2abc123def",
    "lead_id": null,
    "reply_count": 0,
    "created_at": "2026-02-23T14:30:00Z",
    "detected_at": "2026-02-23T14:30:45Z",
    "reviewed_at": "2026-02-23T14:32:00Z"
  }
}
```

---

#### `PATCH /api/v1/mentions/:id`

Update a mention's status or assignment.

**Auth:** Required (editor+)

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Mention ID |

**Request Body:**

```json
{
  "status": "reviewed",
  "assigned_to": "user_2abc123def"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `status` | string | No | One of: `new`, `reviewed`, `replied`, `archived`, `spam` |
| `assigned_to` | string | No | User ID to assign, or `null` to unassign |

**Example Response (`200 OK`):**

```json
{
  "data": {
    "id": "01953a7c-8f00-7def-b234-567890abcdef",
    "status": "reviewed",
    "assigned_to": "user_2abc123def",
    "updated_at": "2026-02-23T14:32:00Z"
  }
}
```

---

#### `GET /api/v1/mentions/stream`

Server-Sent Events endpoint for real-time mention updates. See [Section 5: SSE Event Specification](#5-sse-event-specification) for full detail.

**Auth:** Required (viewer+) -- token passed as query parameter for SSE compatibility.

```
GET /api/v1/mentions/stream?token=eyJhbGciOiJSUzI1NiIs...
```

**Response:** `text/event-stream` (see SSE section for event types and format).

---

#### `POST /api/v1/mentions/:id/reply`

Submit a reply to be posted on the source platform. The reply enters the approval queue (unless the user has auto-approve enabled) and is then dispatched to the appropriate posting mechanism: Reddit API, X API, or Chrome extension queue (LinkedIn).

**Auth:** Required (editor+)

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Mention ID |

**Request Body:**

```json
{
  "content": "We ran into the same problem scaling past 10 people on HubSpot. The key issue is most CRMs treat social selling as an afterthought. We ended up building an internal tool that pulls signals from Reddit and LinkedIn into one pipeline, and it cut our response time from hours to minutes. Happy to share what we learned if you're interested.",
  "variant_id": "reply_draft_001",
  "include_link": false,
  "utm_params": {
    "source": "reddit",
    "medium": "social_reply",
    "campaign": "crm_alternative_feb26"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content` | string | Yes | Final reply text (user may have edited the draft) |
| `variant_id` | string | No | ID of the draft variant used (for A/B tracking) |
| `include_link` | boolean | No | Whether the reply includes a tracked link (default `false`) |
| `utm_params` | object | No | Custom UTM parameters for link tracking |
| `schedule_at` | ISO 8601 | No | Schedule reply for later posting. Omit for immediate queue. |

**Example Response (`201 Created`):**

```json
{
  "data": {
    "id": "01953b4d-2100-7abc-cdef-1234567890ab",
    "mention_id": "01953a7c-8f00-7def-b234-567890abcdef",
    "content": "We ran into the same problem scaling past 10 people on HubSpot...",
    "status": "pending_approval",
    "platform": "reddit",
    "posting_method": "api",
    "tracked_url": null,
    "scheduled_at": null,
    "created_at": "2026-02-23T14:40:00Z"
  }
}
```

**Reply status lifecycle:** `pending_approval` -> `approved` -> `queued` -> `posting` -> `posted` | `failed`

---

#### `GET /api/v1/mentions/:id/thread`

Retrieve the full thread context for a mention. Returns the parent post/comment chain and sibling comments. This data is fetched and cached from the source platform.

**Auth:** Required (viewer+)

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Mention ID |

**Example Request:**

```
GET /api/v1/mentions/01953a7c-8f00-7def-b234-567890abcdef/thread
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...
```

**Example Response (`200 OK`):**

```json
{
  "data": {
    "mention_id": "01953a7c-8f00-7def-b234-567890abcdef",
    "platform": "reddit",
    "thread_url": "https://reddit.com/r/SaaS/comments/1abc/looking_for_crm_alternative/",
    "root_post": {
      "author": "startup_founder_23",
      "content": "Looking for a CRM alternative that doesn't cost $500/mo. We're a 10-person team using HubSpot and the pricing is killing us...",
      "created_at": "2026-02-23T10:00:00Z",
      "upvotes": 47
    },
    "comments": [
      {
        "platform_id": "t1_kx4m001",
        "author": "saas_reviewer",
        "content": "Have you looked at Pipedrive? Much cheaper for small teams.",
        "created_at": "2026-02-23T10:15:00Z",
        "upvotes": 12,
        "depth": 0,
        "is_mention": false
      },
      {
        "platform_id": "t1_kx4m2n9",
        "author": "startup_founder_23",
        "content": "We've been using HubSpot but the pricing is getting out of control for our 10-person team. Looking for something that integrates well with our existing social selling workflow. Any recommendations?",
        "created_at": "2026-02-23T14:30:00Z",
        "upvotes": 8,
        "depth": 1,
        "is_mention": true
      }
    ],
    "summary": "Thread discussing CRM alternatives to HubSpot for small teams. 23 comments, primarily recommending Pipedrive and Attio. OP specifically interested in social selling integration.",
    "fetched_at": "2026-02-23T14:30:45Z"
  }
}
```

---

### 4.2 Leads

Leads represent potential customers identified from mentions. A mention can be promoted to a lead, which then moves through a conversion pipeline.

---

#### `GET /api/v1/leads`

List leads with pipeline stage filtering and pagination.

**Auth:** Required (viewer+)

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | integer | No | Items per page (default `25`, max `100`) |
| `cursor` | string | No | Pagination cursor |
| `stage` | string | No | Comma-separated: `detected`, `engaged`, `replied`, `clicked`, `converted`, `lost` |
| `platform` | string | No | Comma-separated platform filter |
| `assigned_to` | string | No | Filter by assigned user |
| `min_value` | integer | No | Minimum estimated value in cents |
| `date_from` | ISO 8601 | No | Created after this date |
| `date_to` | ISO 8601 | No | Created before this date |
| `sort` | string | No | `created_at`, `updated_at`, `estimated_value`, `stage` |
| `order` | string | No | `asc`, `desc` |

**Example Request:**

```
GET /api/v1/leads?stage=engaged,replied&sort=updated_at&order=desc&limit=20
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...
```

**Example Response (`200 OK`):**

```json
{
  "data": [
    {
      "id": "01953c10-4400-7fed-9876-abcdef012345",
      "mention_id": "01953a7c-8f00-7def-b234-567890abcdef",
      "stage": "replied",
      "platform": "reddit",
      "contact": {
        "username": "startup_founder_23",
        "platform": "reddit",
        "profile_url": "https://reddit.com/u/startup_founder_23"
      },
      "context_summary": "Founder looking for CRM alternative with social selling integration. Frustrated with HubSpot pricing.",
      "estimated_value": 6900,
      "assigned_to": "user_2abc123def",
      "notes": "Responded well to our value comment. Might be a Growth tier fit.",
      "tags": ["saas", "crm-switch", "high-intent"],
      "utm_code": "leh_feb26_reddit_crm",
      "reply_count": 1,
      "click_count": 0,
      "created_at": "2026-02-23T14:35:00Z",
      "updated_at": "2026-02-23T15:10:00Z"
    }
  ],
  "pagination": {
    "next_cursor": "eyJpZCI6IjAxOTUzYzEwLTQ0MDAtN2ZlZC05ODc2LWFiY2RlZjAxMjM0NSJ9",
    "has_more": true
  }
}
```

---

#### `POST /api/v1/leads`

Create a lead from an existing mention.

**Auth:** Required (editor+)

**Request Body:**

```json
{
  "mention_id": "01953a7c-8f00-7def-b234-567890abcdef",
  "estimated_value": 6900,
  "notes": "High-intent buyer. Frustrated with HubSpot pricing, specifically mentioned social selling.",
  "tags": ["saas", "crm-switch", "high-intent"],
  "assigned_to": "user_2abc123def"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `mention_id` | UUID | Yes | Source mention to create lead from |
| `estimated_value` | integer | No | Estimated deal value in cents |
| `notes` | string | No | Free-form notes |
| `tags` | string[] | No | Categorization tags |
| `assigned_to` | string | No | User ID to assign |

**Example Response (`201 Created`):**

```json
{
  "data": {
    "id": "01953c10-4400-7fed-9876-abcdef012345",
    "mention_id": "01953a7c-8f00-7def-b234-567890abcdef",
    "stage": "detected",
    "platform": "reddit",
    "contact": {
      "username": "startup_founder_23",
      "platform": "reddit",
      "profile_url": "https://reddit.com/u/startup_founder_23"
    },
    "estimated_value": 6900,
    "notes": "High-intent buyer. Frustrated with HubSpot pricing, specifically mentioned social selling.",
    "tags": ["saas", "crm-switch", "high-intent"],
    "assigned_to": "user_2abc123def",
    "created_at": "2026-02-23T14:35:00Z",
    "updated_at": "2026-02-23T14:35:00Z"
  }
}
```

---

#### `PATCH /api/v1/leads/:id`

Update a lead's stage, value, notes, or assignment.

**Auth:** Required (editor+)

**Request Body:**

```json
{
  "stage": "clicked",
  "estimated_value": 8900,
  "notes": "Clicked through UTM link. Visited pricing page. Follow up in 2 days.",
  "tags": ["saas", "crm-switch", "high-intent", "pricing-viewed"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `stage` | string | No | New pipeline stage |
| `estimated_value` | integer | No | Updated value in cents |
| `notes` | string | No | Updated notes (replaces existing) |
| `tags` | string[] | No | Updated tags (replaces existing) |
| `assigned_to` | string | No | User ID or `null` to unassign |

**Example Response (`200 OK`):**

```json
{
  "data": {
    "id": "01953c10-4400-7fed-9876-abcdef012345",
    "stage": "clicked",
    "estimated_value": 8900,
    "notes": "Clicked through UTM link. Visited pricing page. Follow up in 2 days.",
    "tags": ["saas", "crm-switch", "high-intent", "pricing-viewed"],
    "updated_at": "2026-02-23T16:00:00Z"
  }
}
```

---

#### `GET /api/v1/leads/:id/events`

Retrieve the full event history for a lead, showing every stage change and significant action.

**Auth:** Required (viewer+)

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Lead ID |

**Example Response (`200 OK`):**

```json
{
  "data": [
    {
      "id": "evt_001",
      "type": "stage_change",
      "from_stage": null,
      "to_stage": "detected",
      "actor": "system",
      "metadata": {
        "trigger": "mention_promoted"
      },
      "created_at": "2026-02-23T14:35:00Z"
    },
    {
      "id": "evt_002",
      "type": "stage_change",
      "from_stage": "detected",
      "to_stage": "engaged",
      "actor": "user_2abc123def",
      "metadata": {
        "action": "reply_submitted"
      },
      "created_at": "2026-02-23T14:42:00Z"
    },
    {
      "id": "evt_003",
      "type": "reply_posted",
      "from_stage": null,
      "to_stage": null,
      "actor": "system",
      "metadata": {
        "reply_id": "01953b4d-2100-7abc-cdef-1234567890ab",
        "platform": "reddit",
        "posting_method": "api"
      },
      "created_at": "2026-02-23T14:42:30Z"
    },
    {
      "id": "evt_004",
      "type": "stage_change",
      "from_stage": "engaged",
      "to_stage": "replied",
      "actor": "system",
      "metadata": {
        "trigger": "reply_confirmed_posted"
      },
      "created_at": "2026-02-23T14:42:30Z"
    },
    {
      "id": "evt_005",
      "type": "utm_click",
      "from_stage": null,
      "to_stage": null,
      "actor": "anonymous",
      "metadata": {
        "utm_code": "leh_feb26_reddit_crm",
        "referrer": "https://reddit.com/r/SaaS/comments/1abc/",
        "landing_page": "/pricing"
      },
      "created_at": "2026-02-23T16:00:00Z"
    }
  ]
}
```

---

### 4.3 Keywords

Keywords are the tracked search terms the Signal Engine uses to detect relevant mentions across platforms.

---

#### `GET /api/v1/keywords`

List all keywords for the current workspace.

**Auth:** Required (viewer+)

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number (default `1`) |
| `per_page` | integer | No | Items per page (default `20`, max `100`) |
| `platform` | string | No | Filter keywords active on specific platform(s) |
| `sort` | string | No | `created_at`, `name`, `mention_count` |
| `order` | string | No | `asc`, `desc` |

**Example Response (`200 OK`):**

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "CRM alternative",
      "match_type": "semantic",
      "platforms": ["reddit", "hackernews", "twitter"],
      "is_active": true,
      "negative_keywords": ["enterprise", "SAP"],
      "mention_count": 142,
      "last_match_at": "2026-02-23T14:30:00Z",
      "created_at": "2026-02-01T10:00:00Z"
    },
    {
      "id": "550e8400-e29b-41d4-a716-446655440001",
      "name": "social selling tool",
      "match_type": "semantic",
      "platforms": ["reddit", "hackernews", "twitter", "linkedin"],
      "is_active": true,
      "negative_keywords": [],
      "mention_count": 89,
      "last_match_at": "2026-02-23T12:15:00Z",
      "created_at": "2026-02-01T10:05:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total": 8,
    "total_pages": 1
  }
}
```

---

#### `POST /api/v1/keywords`

Create a new tracked keyword.

**Auth:** Required (editor+)

**Request Body:**

```json
{
  "name": "HubSpot alternative",
  "match_type": "semantic",
  "platforms": ["reddit", "hackernews", "twitter"],
  "negative_keywords": ["enterprise", "SAP", "Oracle"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | The keyword or phrase to track (max 200 chars) |
| `match_type` | string | No | `exact` or `semantic` (default `semantic`) |
| `platforms` | string[] | No | Platforms to monitor (default: all enabled for workspace) |
| `negative_keywords` | string[] | No | Terms that exclude a mention from matching |

**Example Response (`201 Created`):**

```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "name": "HubSpot alternative",
    "match_type": "semantic",
    "platforms": ["reddit", "hackernews", "twitter"],
    "is_active": true,
    "negative_keywords": ["enterprise", "SAP", "Oracle"],
    "mention_count": 0,
    "last_match_at": null,
    "created_at": "2026-02-23T15:00:00Z"
  }
}
```

**Error Response (`409 Conflict`):**

```json
{
  "error": {
    "code": "DUPLICATE_RESOURCE",
    "message": "A keyword with the name 'HubSpot alternative' already exists in this workspace.",
    "details": {
      "existing_id": "550e8400-e29b-41d4-a716-446655440099"
    }
  }
}
```

---

#### `PATCH /api/v1/keywords/:id`

Update an existing keyword.

**Auth:** Required (editor+)

**Request Body:**

```json
{
  "platforms": ["reddit", "hackernews", "twitter", "linkedin"],
  "negative_keywords": ["enterprise", "SAP", "Oracle", "legacy"],
  "is_active": true
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | No | Updated keyword text |
| `match_type` | string | No | `exact` or `semantic` |
| `platforms` | string[] | No | Updated platform list |
| `negative_keywords` | string[] | No | Updated negative keywords (replaces existing) |
| `is_active` | boolean | No | Enable/disable without deleting |

**Example Response (`200 OK`):**

```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "name": "HubSpot alternative",
    "match_type": "semantic",
    "platforms": ["reddit", "hackernews", "twitter", "linkedin"],
    "is_active": true,
    "negative_keywords": ["enterprise", "SAP", "Oracle", "legacy"],
    "mention_count": 0,
    "last_match_at": null,
    "updated_at": "2026-02-23T15:05:00Z"
  }
}
```

---

#### `DELETE /api/v1/keywords/:id`

Delete a keyword. Associated mentions are not deleted but will no longer receive new matches.

**Auth:** Required (admin)

**Example Response:** `204 No Content`

---

### 4.4 Knowledge Base (RAG)

The knowledge base stores documents that the RAG Brain uses to generate contextually accurate, persona-matched replies. Documents are chunked, embedded, and stored in pgvector for semantic retrieval during reply generation.

---

#### `GET /api/v1/documents`

List all documents in the workspace knowledge base.

**Auth:** Required (viewer+)

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number (default `1`) |
| `per_page` | integer | No | Items per page (default `20`, max `100`) |
| `type` | string | No | Filter by type: `product_doc`, `faq`, `past_reply`, `brand_guide`, `other` |
| `sort` | string | No | `created_at`, `name`, `size` |
| `order` | string | No | `asc`, `desc` |

**Example Response (`200 OK`):**

```json
{
  "data": [
    {
      "id": "doc_001",
      "name": "Product FAQ - February 2026",
      "type": "faq",
      "format": "markdown",
      "size_bytes": 24576,
      "chunk_count": 15,
      "status": "processed",
      "uploaded_by": "user_2abc123def",
      "created_at": "2026-02-20T10:00:00Z",
      "processed_at": "2026-02-20T10:00:45Z"
    },
    {
      "id": "doc_002",
      "name": "Brand Voice Guidelines",
      "type": "brand_guide",
      "format": "pdf",
      "size_bytes": 102400,
      "chunk_count": 42,
      "status": "processed",
      "uploaded_by": "user_2abc123def",
      "created_at": "2026-02-18T09:00:00Z",
      "processed_at": "2026-02-18T09:02:30Z"
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total": 7,
    "total_pages": 1
  }
}
```

---

#### `POST /api/v1/documents`

Upload a document to the knowledge base. Uses `multipart/form-data` for file upload.

**Auth:** Required (editor+)

**Content-Type:** `multipart/form-data`

**Form Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `file` | file | Yes | The document file (max 10MB) |
| `name` | string | No | Display name (defaults to filename) |
| `type` | string | No | `product_doc`, `faq`, `past_reply`, `brand_guide`, `other` |

**Accepted formats:** `.md`, `.txt`, `.pdf`, `.html`, `.docx`

**Example Request:**

```
POST /api/v1/documents
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...
Content-Type: multipart/form-data; boundary=----FormBoundary

------FormBoundary
Content-Disposition: form-data; name="file"; filename="product-faq.md"
Content-Type: text/markdown

# Product FAQ
...
------FormBoundary
Content-Disposition: form-data; name="type"

faq
------FormBoundary--
```

**Example Response (`201 Created`):**

```json
{
  "data": {
    "id": "doc_003",
    "name": "product-faq.md",
    "type": "faq",
    "format": "markdown",
    "size_bytes": 8192,
    "chunk_count": 0,
    "status": "processing",
    "uploaded_by": "user_2abc123def",
    "created_at": "2026-02-23T15:00:00Z",
    "processed_at": null
  }
}
```

The document status transitions: `uploading` -> `processing` (chunking + embedding) -> `processed` | `failed`.

---

#### `DELETE /api/v1/documents/:id`

Delete a document and all its associated embeddings from the vector store.

**Auth:** Required (admin)

**Example Response:** `204 No Content`

---

#### `POST /api/v1/documents/search`

Perform a semantic search across the knowledge base. Used internally by the RAG Brain during reply generation, but also exposed for the dashboard's knowledge base search UI.

**Auth:** Required (viewer+)

**Request Body:**

```json
{
  "query": "How does our product handle social selling integration with CRMs?",
  "limit": 5,
  "type": "product_doc"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | Yes | Natural language search query |
| `limit` | integer | No | Max results to return (default `5`, max `20`) |
| `type` | string | No | Filter by document type |
| `min_similarity` | float | No | Minimum cosine similarity threshold (default `0.7`) |

**Example Response (`200 OK`):**

```json
{
  "data": [
    {
      "chunk_id": "chunk_042",
      "document_id": "doc_001",
      "document_name": "Product FAQ - February 2026",
      "content": "LeadEcho integrates with your existing CRM via webhooks. When a lead moves through the pipeline (detected -> engaged -> converted), events are automatically sent to your configured webhook endpoint. We support HubSpot, Pipedrive, and Salesforce through pre-built templates, and any CRM with a custom webhook.",
      "similarity": 0.92,
      "metadata": {
        "section": "Integrations",
        "chunk_index": 7
      }
    },
    {
      "chunk_id": "chunk_015",
      "document_id": "doc_001",
      "document_name": "Product FAQ - February 2026",
      "content": "Our social selling workflow monitors Reddit, HN, X, and LinkedIn for buying signals. When someone asks for a recommendation matching your keywords, we draft a contextual reply using your product docs and brand voice.",
      "similarity": 0.87,
      "metadata": {
        "section": "Core Features",
        "chunk_index": 3
      }
    }
  ]
}
```

---

### 4.5 Analytics

Analytics endpoints return aggregated metrics and time-series data for the workspace. All analytics queries use offset-based pagination and support date range filtering. Data is served from PostgreSQL materialized views for performance.

---

#### `GET /api/v1/analytics/overview`

Top-level KPI metrics for the dashboard overview cards.

**Auth:** Required (viewer+)

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `date_from` | ISO 8601 | No | Start date (default: 30 days ago) |
| `date_to` | ISO 8601 | No | End date (default: today) |
| `compare` | boolean | No | Include previous period comparison (default `false`) |

**Example Request:**

```
GET /api/v1/analytics/overview?date_from=2026-02-01&date_to=2026-02-23&compare=true
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...
```

**Example Response (`200 OK`):**

```json
{
  "data": {
    "period": {
      "from": "2026-02-01",
      "to": "2026-02-23"
    },
    "metrics": {
      "total_mentions": {
        "value": 1247,
        "previous": 983,
        "change_percent": 26.9
      },
      "high_intent_mentions": {
        "value": 312,
        "previous": 241,
        "change_percent": 29.5
      },
      "replies_sent": {
        "value": 89,
        "previous": 64,
        "change_percent": 39.1
      },
      "reply_rate": {
        "value": 28.5,
        "previous": 26.6,
        "change_percent": 7.1
      },
      "utm_clicks": {
        "value": 34,
        "previous": 22,
        "change_percent": 54.5
      },
      "conversions": {
        "value": 8,
        "previous": 5,
        "change_percent": 60.0
      },
      "estimated_revenue": {
        "value": 55200,
        "previous": 34500,
        "change_percent": 60.0
      },
      "avg_response_time_minutes": {
        "value": 23.4,
        "previous": 41.2,
        "change_percent": -43.2
      }
    }
  }
}
```

---

#### `GET /api/v1/analytics/mentions`

Mention volume trends over time, broken down by platform and intent type.

**Auth:** Required (viewer+)

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `date_from` | ISO 8601 | No | Start date |
| `date_to` | ISO 8601 | No | End date |
| `granularity` | string | No | `hour`, `day`, `week`, `month` (default `day`) |
| `platform` | string | No | Filter by platform(s) |
| `keyword_id` | UUID | No | Filter by keyword |

**Example Response (`200 OK`):**

```json
{
  "data": {
    "granularity": "day",
    "series": [
      {
        "date": "2026-02-21",
        "total": 58,
        "by_platform": {
          "reddit": 24,
          "hackernews": 12,
          "twitter": 18,
          "linkedin": 4
        },
        "by_intent": {
          "buying_signal": 14,
          "recommendation_ask": 19,
          "complaint": 8,
          "general_discussion": 17
        }
      },
      {
        "date": "2026-02-22",
        "total": 63,
        "by_platform": {
          "reddit": 28,
          "hackernews": 10,
          "twitter": 20,
          "linkedin": 5
        },
        "by_intent": {
          "buying_signal": 16,
          "recommendation_ask": 21,
          "complaint": 6,
          "general_discussion": 20
        }
      }
    ]
  }
}
```

---

#### `GET /api/v1/analytics/conversions`

Funnel data showing conversion rates at each stage of the lead pipeline.

**Auth:** Required (viewer+)

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `date_from` | ISO 8601 | No | Start date |
| `date_to` | ISO 8601 | No | End date |
| `platform` | string | No | Filter by platform(s) |

**Example Response (`200 OK`):**

```json
{
  "data": {
    "period": {
      "from": "2026-02-01",
      "to": "2026-02-23"
    },
    "funnel": [
      { "stage": "mentions_detected", "count": 1247, "rate": 100.0 },
      { "stage": "high_intent", "count": 312, "rate": 25.0 },
      { "stage": "replied", "count": 89, "rate": 7.1 },
      { "stage": "link_clicked", "count": 34, "rate": 2.7 },
      { "stage": "signed_up", "count": 12, "rate": 1.0 },
      { "stage": "converted_paid", "count": 8, "rate": 0.6 }
    ],
    "by_platform": {
      "reddit": {
        "mentions": 524,
        "replied": 42,
        "clicked": 18,
        "converted": 5
      },
      "hackernews": {
        "mentions": 287,
        "replied": 22,
        "clicked": 8,
        "converted": 2
      },
      "twitter": {
        "mentions": 356,
        "replied": 20,
        "clicked": 6,
        "converted": 1
      },
      "linkedin": {
        "mentions": 80,
        "replied": 5,
        "clicked": 2,
        "converted": 0
      }
    }
  }
}
```

---

#### `GET /api/v1/analytics/keywords`

Performance metrics for each tracked keyword.

**Auth:** Required (viewer+)

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `date_from` | ISO 8601 | No | Start date |
| `date_to` | ISO 8601 | No | End date |
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |
| `sort` | string | No | `mention_count`, `reply_count`, `conversion_rate`, `name` |
| `order` | string | No | `asc`, `desc` |

**Example Response (`200 OK`):**

```json
{
  "data": [
    {
      "keyword_id": "550e8400-e29b-41d4-a716-446655440000",
      "keyword": "CRM alternative",
      "mention_count": 142,
      "high_intent_count": 38,
      "reply_count": 18,
      "click_count": 9,
      "conversion_count": 3,
      "conversion_rate": 2.1,
      "avg_relevance_score": 7.8,
      "top_platform": "reddit"
    },
    {
      "keyword_id": "550e8400-e29b-41d4-a716-446655440001",
      "keyword": "social selling tool",
      "mention_count": 89,
      "high_intent_count": 24,
      "reply_count": 12,
      "click_count": 5,
      "conversion_count": 2,
      "conversion_rate": 2.2,
      "avg_relevance_score": 8.1,
      "top_platform": "hackernews"
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total": 8,
    "total_pages": 1
  }
}
```

---

#### `GET /api/v1/analytics/team`

Per-team-member activity and performance statistics.

**Auth:** Required (admin)

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `date_from` | ISO 8601 | No | Start date |
| `date_to` | ISO 8601 | No | End date |

**Example Response (`200 OK`):**

```json
{
  "data": [
    {
      "user_id": "user_2abc123def",
      "name": "Alex Chen",
      "email": "alex@company.com",
      "role": "admin",
      "stats": {
        "mentions_reviewed": 156,
        "replies_sent": 42,
        "avg_response_time_minutes": 18.5,
        "leads_created": 28,
        "conversions": 5,
        "estimated_revenue_attributed": 34500
      }
    },
    {
      "user_id": "user_3def456ghi",
      "name": "Jordan Lee",
      "email": "jordan@company.com",
      "role": "editor",
      "stats": {
        "mentions_reviewed": 98,
        "replies_sent": 31,
        "avg_response_time_minutes": 32.1,
        "leads_created": 15,
        "conversions": 3,
        "estimated_revenue_attributed": 20700
      }
    }
  ]
}
```

---

### 4.6 Workflows

Workflows define automated pipelines that trigger when a mention matches specified conditions. A workflow chains together: trigger conditions, AI draft generation, notification/approval gates, and posting actions.

---

#### `GET /api/v1/workflows`

List all workflows for the workspace.

**Auth:** Required (viewer+)

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |
| `is_active` | boolean | No | Filter by active status |

**Example Response (`200 OK`):**

```json
{
  "data": [
    {
      "id": "wf_001",
      "name": "High-Intent Reddit Auto-Draft",
      "description": "Auto-draft replies for high-scoring Reddit mentions and notify Slack",
      "is_active": true,
      "trigger": {
        "conditions": {
          "platform": ["reddit"],
          "min_relevance_score": 8.0,
          "intent": ["buying_signal", "recommendation_ask"],
          "keyword_ids": ["550e8400-e29b-41d4-a716-446655440000"]
        }
      },
      "actions": [
        {
          "type": "ai_draft",
          "config": {
            "variants": 3,
            "tone": "helpful_expert",
            "include_link_variant": true
          }
        },
        {
          "type": "notify_slack",
          "config": {
            "channel": "#social-leads",
            "include_thread_summary": true,
            "include_draft_preview": true
          }
        },
        {
          "type": "approval_gate",
          "config": {
            "approvers": ["user_2abc123def", "user_3def456ghi"],
            "timeout_hours": 24,
            "auto_action_on_timeout": "skip"
          }
        },
        {
          "type": "queue_reply",
          "config": {
            "human_mimicry_delay": true,
            "min_delay_seconds": 120,
            "max_delay_seconds": 480
          }
        }
      ],
      "stats": {
        "total_executions": 47,
        "last_execution_at": "2026-02-23T14:30:00Z",
        "approval_rate": 78.7,
        "avg_time_to_approval_minutes": 42.3
      },
      "created_at": "2026-02-10T10:00:00Z",
      "updated_at": "2026-02-22T16:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total": 3,
    "total_pages": 1
  }
}
```

---

#### `POST /api/v1/workflows`

Create a new workflow.

**Auth:** Required (editor+)

**Request Body:**

```json
{
  "name": "LinkedIn High-Value Signals",
  "description": "Notify team when high-value LinkedIn signals are detected",
  "is_active": true,
  "trigger": {
    "conditions": {
      "platform": ["linkedin"],
      "min_relevance_score": 7.5,
      "intent": ["buying_signal"]
    }
  },
  "actions": [
    {
      "type": "ai_draft",
      "config": {
        "variants": 2,
        "tone": "professional"
      }
    },
    {
      "type": "notify_slack",
      "config": {
        "channel": "#linkedin-leads",
        "include_thread_summary": true
      }
    }
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Workflow name (max 200 chars) |
| `description` | string | No | Human-readable description |
| `is_active` | boolean | No | Start active (default `true`) |
| `trigger.conditions` | object | Yes | Trigger conditions (see schema below) |
| `actions` | array | Yes | Ordered list of actions (min 1) |

**Trigger condition schema:**

| Field | Type | Description |
|-------|------|-------------|
| `platform` | string[] | Platforms to match |
| `min_relevance_score` | float | Minimum score threshold (0-10) |
| `intent` | string[] | Intent types: `buying_signal`, `recommendation_ask`, `complaint`, `general_discussion` |
| `keyword_ids` | UUID[] | Specific keywords to match (empty = all) |
| `subreddits` | string[] | Specific subreddits (Reddit only) |

**Action type schema:**

| Type | Description |
|------|-------------|
| `ai_draft` | Generate AI reply drafts using RAG Brain |
| `notify_slack` | Send notification to a Slack channel via webhook |
| `notify_discord` | Send notification to a Discord channel via webhook |
| `notify_email` | Send email notification |
| `approval_gate` | Wait for human approval before proceeding |
| `queue_reply` | Queue the approved reply for posting |
| `create_lead` | Automatically create a lead from the mention |
| `webhook` | Send data to a custom webhook URL |

**Example Response (`201 Created`):** Returns the full workflow object with generated `id`.

---

#### `PATCH /api/v1/workflows/:id`

Update a workflow's configuration.

**Auth:** Required (editor+)

**Request Body:** Any subset of the fields from the POST schema. Partial updates are supported.

**Example Response (`200 OK`):** Returns the updated workflow object.

---

#### `DELETE /api/v1/workflows/:id`

Delete a workflow. Running executions are cancelled.

**Auth:** Required (admin)

**Example Response:** `204 No Content`

---

#### `GET /api/v1/workflows/:id/executions`

List recent executions of a workflow, showing each step's status and outcome.

**Auth:** Required (viewer+)

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | integer | No | Items per page (default `25`, max `100`) |
| `cursor` | string | No | Pagination cursor |
| `status` | string | No | Filter: `running`, `completed`, `failed`, `cancelled`, `waiting_approval` |

**Example Response (`200 OK`):**

```json
{
  "data": [
    {
      "id": "exec_001",
      "workflow_id": "wf_001",
      "mention_id": "01953a7c-8f00-7def-b234-567890abcdef",
      "status": "completed",
      "steps": [
        {
          "action": "ai_draft",
          "status": "completed",
          "started_at": "2026-02-23T14:31:00Z",
          "completed_at": "2026-02-23T14:31:03Z",
          "output": {
            "draft_count": 3
          }
        },
        {
          "action": "notify_slack",
          "status": "completed",
          "started_at": "2026-02-23T14:31:03Z",
          "completed_at": "2026-02-23T14:31:04Z",
          "output": {
            "slack_message_ts": "1740321064.001234"
          }
        },
        {
          "action": "approval_gate",
          "status": "completed",
          "started_at": "2026-02-23T14:31:04Z",
          "completed_at": "2026-02-23T15:12:00Z",
          "output": {
            "approved_by": "user_2abc123def",
            "selected_variant": "reply_draft_001",
            "wait_time_minutes": 41
          }
        },
        {
          "action": "queue_reply",
          "status": "completed",
          "started_at": "2026-02-23T15:12:00Z",
          "completed_at": "2026-02-23T15:15:23Z",
          "output": {
            "reply_id": "01953b4d-2100-7abc-cdef-1234567890ab",
            "posting_delay_seconds": 203
          }
        }
      ],
      "triggered_at": "2026-02-23T14:30:45Z",
      "completed_at": "2026-02-23T15:15:23Z",
      "duration_seconds": 2678
    }
  ],
  "pagination": {
    "next_cursor": "eyJpZCI6ImV4ZWNfMDAxIn0",
    "has_more": true
  }
}
```

---

### 4.7 Extension

These endpoints are specifically for the Chrome extension's communication with the cloud backend. The extension submits LinkedIn signals (which cannot be monitored server-side), fetches queued replies for posting, and confirms successful posts.

---

#### `POST /api/v1/extension/signals`

The Chrome extension submits detected LinkedIn signals (buying intent posts, competitor mentions, relevant comments from the user's feed).

**Auth:** Required (extension API key)

**Request Body:**

```json
{
  "signals": [
    {
      "platform": "linkedin",
      "platform_id": "urn:li:activity:7012345678901234567",
      "url": "https://www.linkedin.com/feed/update/urn:li:activity:7012345678901234567",
      "author": {
        "name": "Sarah Johnson",
        "headline": "VP of Sales at TechCorp",
        "profile_url": "https://www.linkedin.com/in/sarahjohnson",
        "connection_degree": 2
      },
      "content": "We're evaluating our social selling stack for Q2. Currently using Hootsuite for monitoring but it doesn't help us actually engage with prospects. Any recommendations for tools that go beyond just listening?",
      "content_type": "post",
      "engagement": {
        "likes": 45,
        "comments": 12,
        "reposts": 3
      },
      "detected_at": "2026-02-23T14:30:00Z"
    }
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `signals` | array | Yes | Array of detected signals (max 50 per request) |
| `signals[].platform` | string | Yes | Always `linkedin` for extension signals |
| `signals[].platform_id` | string | Yes | LinkedIn URN of the post/comment |
| `signals[].url` | string | Yes | Direct URL to the post |
| `signals[].author` | object | Yes | Author information visible in the feed |
| `signals[].content` | string | Yes | Text content of the post/comment |
| `signals[].content_type` | string | Yes | `post`, `comment`, `article` |
| `signals[].engagement` | object | No | Like/comment/repost counts |
| `signals[].detected_at` | ISO 8601 | Yes | When the extension detected this signal |

**Example Response (`201 Created`):**

```json
{
  "data": {
    "accepted": 1,
    "duplicates": 0,
    "mentions_created": [
      {
        "signal_platform_id": "urn:li:activity:7012345678901234567",
        "mention_id": "01953d20-1100-7abc-dead-beef12345678"
      }
    ]
  }
}
```

---

#### `GET /api/v1/extension/queue`

Fetch pending replies that have been approved and are waiting for the extension to post them on LinkedIn (since LinkedIn posting cannot be done via server-side API).

**Auth:** Required (extension API key)

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | integer | No | Max items to return (default `10`, max `25`) |

**Example Response (`200 OK`):**

```json
{
  "data": [
    {
      "id": "queue_001",
      "reply_id": "01953b4d-2100-7abc-cdef-1234567890ab",
      "mention_id": "01953d20-1100-7abc-dead-beef12345678",
      "platform": "linkedin",
      "target_url": "https://www.linkedin.com/feed/update/urn:li:activity:7012345678901234567",
      "content": "Great question, Sarah. We had the same frustration with listen-only tools. The gap is really between monitoring and engagement...",
      "posting_instructions": {
        "type": "comment",
        "min_delay_seconds": 120,
        "max_delay_seconds": 480,
        "simulate_typing": true
      },
      "approved_at": "2026-02-23T15:00:00Z",
      "expires_at": "2026-02-24T15:00:00Z"
    }
  ]
}
```

---

#### `POST /api/v1/extension/posted`

The extension confirms that a reply has been successfully posted on the platform.

**Auth:** Required (extension API key)

**Request Body:**

```json
{
  "queue_id": "queue_001",
  "reply_id": "01953b4d-2100-7abc-cdef-1234567890ab",
  "platform_post_id": "urn:li:comment:(urn:li:activity:7012345678901234567,7098765432109876543)",
  "posted_at": "2026-02-23T15:05:23Z",
  "actual_content": "Great question, Sarah. We had the same frustration with listen-only tools..."
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `queue_id` | string | Yes | Queue item ID from the GET response |
| `reply_id` | string | Yes | Reply ID |
| `platform_post_id` | string | Yes | Platform-assigned ID of the posted comment |
| `posted_at` | ISO 8601 | Yes | Actual posting timestamp |
| `actual_content` | string | No | Content as actually posted (may differ if user edited in extension) |

**Example Response (`200 OK`):**

```json
{
  "data": {
    "reply_id": "01953b4d-2100-7abc-cdef-1234567890ab",
    "status": "posted",
    "posted_at": "2026-02-23T15:05:23Z"
  }
}
```

---

### 4.8 Webhooks

Inbound webhook endpoints for third-party service integrations. These endpoints use provider-specific signature verification rather than standard Bearer auth.

---

#### `POST /api/v1/webhooks/clerk`

Receives Clerk webhook events for user and organization synchronization. Keeps the LeadEcho database in sync with Clerk's identity data.

**Auth:** Svix signature verification (`svix-id`, `svix-timestamp`, `svix-signature` headers)

**Handled event types:**

| Event | Action |
|-------|--------|
| `user.created` | Create user record in LeadEcho database |
| `user.updated` | Sync updated profile data (name, email, avatar) |
| `user.deleted` | Soft-delete user, revoke API keys |
| `organization.created` | Create workspace |
| `organization.updated` | Sync workspace name/settings |
| `organization.deleted` | Soft-delete workspace and all associated data |
| `organizationMembership.created` | Add user to workspace with role |
| `organizationMembership.updated` | Update user's role in workspace |
| `organizationMembership.deleted` | Remove user from workspace |

**Example Clerk webhook payload (user.created):**

```json
{
  "type": "user.created",
  "data": {
    "id": "user_2abc123def",
    "email_addresses": [
      {
        "email_address": "alex@company.com",
        "id": "idn_abc",
        "verification": { "status": "verified" }
      }
    ],
    "first_name": "Alex",
    "last_name": "Chen",
    "created_at": 1740321000
  }
}
```

**Response:** `200 OK` with empty body (Clerk expects 2xx to confirm receipt).

---

#### `POST /api/v1/webhooks/stripe`

Receives Stripe webhook events for subscription and payment management.

**Auth:** Stripe signature verification (`Stripe-Signature` header)

**Handled event types:**

| Event | Action |
|-------|--------|
| `checkout.session.completed` | Activate subscription, set workspace tier |
| `customer.subscription.updated` | Update tier (upgrade/downgrade), adjust limits |
| `customer.subscription.deleted` | Downgrade workspace to free tier |
| `invoice.payment_succeeded` | Record payment, extend subscription |
| `invoice.payment_failed` | Flag workspace, send notification, start grace period |

**Response:** `200 OK` with empty body.

---

#### `GET /api/v1/webhooks/utm/:code`

UTM click redirect and tracking endpoint. When a tracked link is clicked, this endpoint records the click event and redirects the user to the destination URL.

**Auth:** None (public endpoint)

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `code` | string | Unique UTM tracking code (e.g., `leh_feb26_reddit_crm`) |

**Query Parameters (standard UTM, appended automatically):**

| Parameter | Type | Description |
|-----------|------|-------------|
| `utm_source` | string | Traffic source (e.g., `reddit`) |
| `utm_medium` | string | Marketing medium (e.g., `social_reply`) |
| `utm_campaign` | string | Campaign name |

**Behavior:**

1. Record click event with: timestamp, referrer, user-agent, IP (hashed for privacy)
2. Look up destination URL for the UTM code
3. Respond with `302 Found` redirect to destination URL with UTM parameters preserved

**Example:**

```
GET /api/v1/webhooks/utm/leh_feb26_reddit_crm?utm_source=reddit&utm_medium=social_reply&utm_campaign=crm_alternative_feb26
```

**Response:** `302 Found`

```
Location: https://leadecho.app/pricing?utm_source=reddit&utm_medium=social_reply&utm_campaign=crm_alternative_feb26
```

---

### 4.9 Team

Team management endpoints for workspace member administration.

---

#### `GET /api/v1/team/members`

List all members of the current workspace.

**Auth:** Required (viewer+)

**Example Response (`200 OK`):**

```json
{
  "data": [
    {
      "id": "user_2abc123def",
      "name": "Alex Chen",
      "email": "alex@company.com",
      "avatar_url": "https://img.clerk.com/abc123",
      "role": "admin",
      "status": "active",
      "stats": {
        "mentions_reviewed_30d": 156,
        "replies_sent_30d": 42
      },
      "joined_at": "2026-01-15T10:00:00Z",
      "last_active_at": "2026-02-23T14:30:00Z"
    },
    {
      "id": "user_3def456ghi",
      "name": "Jordan Lee",
      "email": "jordan@company.com",
      "avatar_url": "https://img.clerk.com/def456",
      "role": "editor",
      "status": "active",
      "stats": {
        "mentions_reviewed_30d": 98,
        "replies_sent_30d": 31
      },
      "joined_at": "2026-02-01T09:00:00Z",
      "last_active_at": "2026-02-23T12:15:00Z"
    }
  ]
}
```

---

#### `PATCH /api/v1/team/members/:id/role`

Update a team member's role within the workspace.

**Auth:** Required (admin)

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | User ID of the team member |

**Request Body:**

```json
{
  "role": "editor"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `role` | string | Yes | New role: `admin`, `editor`, `viewer` |

**Example Response (`200 OK`):**

```json
{
  "data": {
    "id": "user_3def456ghi",
    "name": "Jordan Lee",
    "role": "editor",
    "updated_at": "2026-02-23T15:00:00Z"
  }
}
```

**Error Response (`403 Forbidden`):**

```json
{
  "error": {
    "code": "FORBIDDEN",
    "message": "Cannot change the role of the last admin. Assign another admin first.",
    "details": {}
  }
}
```

---

## 5. SSE Event Specification

The `/api/v1/mentions/stream` endpoint provides real-time updates to the dashboard via Server-Sent Events. The SSE connection is long-lived and delivers events as they occur in the system.

### Connection

```
GET /api/v1/mentions/stream?token=<jwt_token>
Accept: text/event-stream
```

Authentication is passed as a query parameter because the `EventSource` browser API does not support custom headers. The JWT is validated identically to the `Authorization` header flow.

**Response headers:**

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

### Event Types

Each SSE event has a type (the `event:` field) and a JSON-encoded `data:` payload.

---

#### `mention.new`

A new mention has been detected and scored by the Signal Engine.

```
event: mention.new
id: evt_01953a7c
data: {"id":"01953a7c-8f00-7def-b234-567890abcdef","platform":"reddit","content":"We've been using HubSpot but the pricing is getting out of control...","relevance_score":9.2,"intent":"buying_signal","keyword_matches":["CRM alternative"],"thread_title":"Looking for a CRM alternative","author":"startup_founder_23","url":"https://reddit.com/r/SaaS/comments/1abc/...","created_at":"2026-02-23T14:30:00Z"}
```

---

#### `mention.updated`

A mention's status, assignment, or other metadata has changed.

```
event: mention.updated
id: evt_01953a8d
data: {"id":"01953a7c-8f00-7def-b234-567890abcdef","status":"reviewed","assigned_to":"user_2abc123def","updated_at":"2026-02-23T14:32:00Z"}
```

---

#### `reply.drafted`

AI reply drafts have been generated for a mention (typically triggered by a workflow or user action).

```
event: reply.drafted
id: evt_01953b2e
data: {"mention_id":"01953a7c-8f00-7def-b234-567890abcdef","draft_count":3,"variants":["value","technical","soft_sell"],"generated_at":"2026-02-23T14:35:00Z"}
```

---

#### `reply.posted`

A reply has been successfully posted to the source platform.

```
event: reply.posted
id: evt_01953c4f
data: {"reply_id":"01953b4d-2100-7abc-cdef-1234567890ab","mention_id":"01953a7c-8f00-7def-b234-567890abcdef","platform":"reddit","posted_at":"2026-02-23T15:15:23Z"}
```

---

#### `lead.updated`

A lead's stage or metadata has changed.

```
event: lead.updated
id: evt_01953d50
data: {"id":"01953c10-4400-7fed-9876-abcdef012345","stage":"clicked","previous_stage":"replied","updated_at":"2026-02-23T16:00:00Z"}
```

---

#### `workflow.execution`

A workflow execution has reached a notable state (started, waiting for approval, completed, failed).

```
event: workflow.execution
id: evt_01953e61
data: {"execution_id":"exec_001","workflow_id":"wf_001","workflow_name":"High-Intent Reddit Auto-Draft","status":"waiting_approval","mention_id":"01953a7c-8f00-7def-b234-567890abcdef","current_step":"approval_gate","updated_at":"2026-02-23T14:31:04Z"}
```

---

#### `heartbeat`

Sent every 30 seconds to keep the connection alive and detect stale connections.

```
event: heartbeat
data: {"timestamp":"2026-02-23T14:31:00Z"}
```

### Reconnection

The server includes an `id:` field with each event. If the SSE connection drops, the client's `EventSource` automatically reconnects and sends the `Last-Event-ID` header. The server replays any missed events from the Redis pub/sub event buffer (retained for 5 minutes).

**Retry directive (sent on initial connection):**

```
retry: 5000
```

This instructs the browser to wait 5 seconds before attempting reconnection.

### Client Integration (TanStack Query)

On the frontend, SSE events update the TanStack Query cache directly:

```typescript
// SSE -> TanStack Query integration
const eventSource = new EventSource(`/api/v1/mentions/stream?token=${token}`);

eventSource.addEventListener('mention.new', (event) => {
  const mention = JSON.parse(event.data);
  queryClient.setQueryData(['mentions'], (old) => ({
    ...old,
    data: [mention, ...old.data],
  }));
});

eventSource.addEventListener('mention.updated', (event) => {
  const update = JSON.parse(event.data);
  queryClient.setQueryData(['mentions'], (old) => ({
    ...old,
    data: old.data.map(m => m.id === update.id ? { ...m, ...update } : m),
  }));
});
```

---

## 6. Webhook Output

LeadEcho supports outbound webhooks for CRM synchronization. Customers configure a destination URL in the workspace settings. When configured events occur, LeadEcho sends an HTTP POST to the destination URL with a signed payload.

### Webhook Payload Format

All outbound webhooks use a consistent envelope:

```json
{
  "id": "whk_01953f72-3300-7abc-1234-567890abcdef",
  "type": "lead.stage_changed",
  "workspace_id": "org_xyz789",
  "timestamp": "2026-02-23T16:00:00Z",
  "data": {}
}
```

### Webhook Signing

Each webhook request includes a signature header for verification:

```
X-LeadEcho-Signature: sha256=a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2
X-LeadEcho-Timestamp: 1740321600
```

The signature is computed as:

```
HMAC-SHA256(webhook_secret, timestamp + "." + raw_body)
```

The receiver should verify the signature and reject requests where the timestamp is older than 5 minutes (to prevent replay attacks).

### Event Types

#### `lead.stage_changed`

Fired when a lead moves to a new pipeline stage.

```json
{
  "id": "whk_01953f72-3300-7abc-1234-567890abcdef",
  "type": "lead.stage_changed",
  "workspace_id": "org_xyz789",
  "timestamp": "2026-02-23T16:00:00Z",
  "data": {
    "lead_id": "01953c10-4400-7fed-9876-abcdef012345",
    "previous_stage": "replied",
    "new_stage": "clicked",
    "contact": {
      "username": "startup_founder_23",
      "platform": "reddit",
      "profile_url": "https://reddit.com/u/startup_founder_23"
    },
    "mention": {
      "id": "01953a7c-8f00-7def-b234-567890abcdef",
      "platform": "reddit",
      "url": "https://reddit.com/r/SaaS/comments/1abc/looking_for_crm_alternative/kx4m2n9",
      "content_preview": "We've been using HubSpot but the pricing is getting out of control..."
    },
    "estimated_value": 8900,
    "tags": ["saas", "crm-switch", "high-intent", "pricing-viewed"]
  }
}
```

#### `lead.created`

Fired when a new lead is created from a mention.

```json
{
  "id": "whk_01953f73-4400-7def-5678-abcdef012345",
  "type": "lead.created",
  "workspace_id": "org_xyz789",
  "timestamp": "2026-02-23T14:35:00Z",
  "data": {
    "lead_id": "01953c10-4400-7fed-9876-abcdef012345",
    "stage": "detected",
    "contact": {
      "username": "startup_founder_23",
      "platform": "reddit",
      "profile_url": "https://reddit.com/u/startup_founder_23"
    },
    "mention": {
      "id": "01953a7c-8f00-7def-b234-567890abcdef",
      "platform": "reddit",
      "url": "https://reddit.com/r/SaaS/comments/1abc/looking_for_crm_alternative/kx4m2n9",
      "relevance_score": 9.2,
      "intent": "buying_signal"
    },
    "estimated_value": 6900,
    "tags": ["saas", "crm-switch", "high-intent"]
  }
}
```

#### `reply.posted`

Fired when a reply is successfully posted to a platform.

```json
{
  "id": "whk_01953f74-5500-7fed-9012-abcdef345678",
  "type": "reply.posted",
  "workspace_id": "org_xyz789",
  "timestamp": "2026-02-23T15:15:23Z",
  "data": {
    "reply_id": "01953b4d-2100-7abc-cdef-1234567890ab",
    "mention_id": "01953a7c-8f00-7def-b234-567890abcdef",
    "lead_id": "01953c10-4400-7fed-9876-abcdef012345",
    "platform": "reddit",
    "platform_post_url": "https://reddit.com/r/SaaS/comments/1abc/looking_for_crm_alternative/kx9z1m4",
    "content_preview": "We ran into the same problem scaling past 10 people on HubSpot...",
    "posted_by": "user_2abc123def",
    "utm_code": "leh_feb26_reddit_crm"
  }
}
```

#### `mention.high_intent`

Fired when a mention is detected with a relevance score above the workspace's configured threshold (useful for immediate CRM notifications).

```json
{
  "id": "whk_01953f75-6600-7abc-3456-abcdef789012",
  "type": "mention.high_intent",
  "workspace_id": "org_xyz789",
  "timestamp": "2026-02-23T14:30:45Z",
  "data": {
    "mention_id": "01953a7c-8f00-7def-b234-567890abcdef",
    "platform": "reddit",
    "url": "https://reddit.com/r/SaaS/comments/1abc/looking_for_crm_alternative/kx4m2n9",
    "content_preview": "We've been using HubSpot but the pricing is getting out of control...",
    "relevance_score": 9.2,
    "intent": "buying_signal",
    "keyword_matches": ["CRM alternative"],
    "author": {
      "username": "startup_founder_23",
      "platform": "reddit"
    }
  }
}
```

### Webhook Delivery

- **Timeout:** 10 seconds per delivery attempt
- **Retries:** Up to 5 attempts with exponential backoff (10s, 30s, 90s, 270s, 810s)
- **Retry condition:** Any non-2xx response or timeout
- **Deactivation:** After 100 consecutive failures, the webhook is automatically deactivated and the workspace admin is notified via email
- **Ordering:** Events are delivered in order per lead, but no global ordering guarantee across leads
- **Idempotency:** Each webhook has a unique `id` field. Receivers should deduplicate by this ID.

---

## 7. Rate Limiting Strategy

Rate limits protect the API from abuse and ensure fair resource allocation across workspaces. Limits are enforced at the Go middleware layer using Redis token bucket counters.

### Per-Tier Rate Limits

| Tier | Requests/Minute | Requests/Hour | Burst Allowance | SSE Connections |
|------|-----------------|---------------|-----------------|-----------------|
| **Starter (Free)** | 30 | 500 | 10 extra | 1 |
| **Solo ($29)** | 60 | 2,000 | 20 extra | 2 |
| **Growth ($69)** | 120 | 5,000 | 40 extra | 5 |
| **Scale ($199)** | 300 | 15,000 | 100 extra | 15 |

### Rate Limit Scopes

Rate limits are applied at multiple levels:

| Scope | Key | Description |
|-------|-----|-------------|
| **Workspace** | `workspace:{org_id}` | Total requests from all users in the workspace |
| **User** | `user:{user_id}` | Per-user limit (prevents one user from consuming entire workspace quota) |
| **Endpoint** | `endpoint:{org_id}:{path}` | Per-endpoint limits for expensive operations |

### Endpoint-Specific Limits

Some endpoints have additional, tighter limits independent of the global rate:

| Endpoint | Limit | Reason |
|----------|-------|--------|
| `POST /api/v1/mentions/:id/reply` | 20/hour per workspace | Prevents excessive posting, account safety |
| `POST /api/v1/documents` | 10/hour per workspace | Document processing is CPU/GPU intensive |
| `POST /api/v1/documents/search` | 60/hour per workspace | Embedding generation per query |
| `GET /api/v1/mentions/stream` | See SSE connections column | Long-lived connections consume server resources |
| `POST /api/v1/extension/signals` | 120/hour per user | Prevents extension from flooding with signals |

### Burst Allowance

The burst allowance permits short spikes above the per-minute limit. It works as a token bucket:

- Tokens refill at the per-minute rate
- Bucket can hold up to `per_minute_limit + burst_allowance` tokens
- Each request consumes 1 token
- When the bucket is empty, requests receive `429`

For example, a Growth tier workspace (120 req/min + 40 burst) can handle a brief spike of 160 requests in a single minute, but must then wait for the bucket to refill.

### Rate Limit Response

When rate limited:

```
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 120
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1740321120
Retry-After: 45
Content-Type: application/json
```

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "Rate limit exceeded. Try again in 45 seconds.",
    "details": {
      "limit": 120,
      "remaining": 0,
      "reset_at": "2026-02-23T14:32:00Z",
      "retry_after_seconds": 45,
      "scope": "workspace"
    }
  }
}
```

### Implementation Notes

Rate limiting is implemented in Go middleware using Redis:

```go
// Middleware chain in Chi router
r.Use(middleware.RateLimit(redisClient, rateLimitConfig))
```

The Redis implementation uses `MULTI`/`EXEC` for atomic token bucket operations:

1. `GET` current token count
2. If tokens available: `DECR` and set `EXPIRE` (window duration)
3. If no tokens: return `429` with reset time from `TTL`

Extension API keys share the rate limit of their associated user and workspace. Webhook endpoints (Clerk, Stripe, UTM redirect) are exempt from standard rate limiting but have their own abuse protection (IP-based throttling at 100 req/min).
