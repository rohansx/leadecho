# Redis Streams Event Backbone

## Executive Summary

LeadEcho's mention-to-reply pipeline previously ran as a single inline path inside the monitor worker: ingest, score, qualify, notify, and draft were tightly coupled in one process tick. That design is acceptable for early development, but it does not survive restarts gracefully, cannot scale stages independently, and makes failures difficult to isolate.

The **Redis Streams Event Backbone** replaces that monolith with a durable, domain-oriented event pipeline. Producers emit versioned events into Redis Streams after persisting them to PostgreSQL. Independent consumer groups execute each stage of the pipeline with idempotency, checkpointing, dead-letter handling, and optional replay.

This document describes the architecture as implemented in the `chris-dev` branch: the mention, reply, and workflow domains are now connected through a single event contract. The system supports a **gradual migration** from inline execution to fully asynchronous processing without requiring a big-bang cutover.

For the long-term workflow product vision, see [08-workflow-engine.md](./08-workflow-engine.md). This backbone is the runtime substrate that vision depends on.

---

## Design Goals

The backbone is governed by six architectural principles.

1. **Durability before dispatch**

   Every event is written to `event_log` in PostgreSQL before it is published to Redis. If Redis publish fails, the record remains auditable and recoverable.

2. **Domain separation**

   Mention scoring, lead qualification, notifications, reply drafting, and workflow execution are separate concerns. Each concern maps to its own consumer group and can be enabled, scaled, or deployed independently.

3. **Exactly-once semantics at the business layer**

   Redis Streams provides at-least-once delivery. LeadEcho compensates with `consumer_processed_events` idempotency keys and stream-level idempotency on publish. Duplicate delivery must not create duplicate leads, notifications, or workflow executions.

4. **Progressive rollout**

   Feature flags allow dual-write (inline + events), per-consumer activation, and inline fallback. Production can migrate one stage at a time.

5. **Operational visibility**

   Consumer checkpoints, dead-letter queues, and admin APIs expose lag, failures, and replay controls. Operators should not need to grep monitor logs to understand pipeline health.

6. **Thin orchestration, fat services**

   Consumers are transport adapters. Business logic lives in reusable services (`monitor`, `reply`, `workflow`) that can be invoked from HTTP handlers or stream workers interchangeably.

---

## High-Level Architecture

```text
                         ┌─────────────────────────────────────────────┐
                         │              Producers                       │
                         │  Monitor (ingest/score) │ API (draft/approve) │
                         └────────────┬────────────────────┬─────────────┘
                                      │                    │
                                      v                    v
                         ┌────────────────────────────────────────────┐
                         │         publishers.Publisher               │
                         │   1. INSERT event_log (Postgres)           │
                         │   2. XADD envelope (Redis Stream)          │
                         └────────────────────┬───────────────────────┘
                                              │
              ┌───────────────────────────────┼───────────────────────────────┐
              │                               │                               │
              v                               v                               v
   leadecho:mention_events        leadecho:reply_events         leadecho:workflow_events
              │                               │                               │
   ┌──────────┼──────────┐                    │                               │
   │          │          │                    v                               v
   v          v          v            reply_drafters              workflow_executors
mention_  mention_  mention_
scorers   qualifiers notifiers
   │          │          │
   v          v          v
 monitor   monitor    monitor
 services  services   services
```

The API process (`cmd/api/main.go`) currently hosts both the HTTP server and stream workers. This is a deployment convenience, not an architectural constraint. Any consumer group can later be extracted into a dedicated worker process without changing event contracts.

---

## Event Envelope

All stream messages share a versioned envelope defined in `internal/events/model.go`.

| Field | Purpose |
|-------|---------|
| `event_id` | Globally unique event identifier (UUID v4) |
| `event_type` | Namespaced type string, e.g. `mention.scored.v1` |
| `schema_version` | Payload schema version (currently `1`) |
| `workspace_id` | Tenant isolation boundary |
| `aggregate_type` | Domain entity: `mention`, `reply`, `workflow` |
| `aggregate_id` | Entity ID the event pertains to |
| `producer` | Emitting component: `monitor`, `scorer`, `api`, `workflow_engine` |
| `payload` | JSON document; typed per `event_type` |
| `idempotency_key` | Dedup key scoped to `(stream_name, idempotency_key)` |
| `occurred_at` | UTC timestamp of business occurrence |

Envelope validation runs at publish time. Invalid envelopes are rejected before they reach Redis.

---

## Streams and Consumer Groups

Stream routing is centralized in `internal/events/streams.go`.

| Stream | Event Types | Consumer Groups |
|--------|-------------|-----------------|
| `leadecho:mention_events` | `mention.ingested.v1`, `mention.scored.v1`, `mention.qualified.v1`, `mention.notification_requested.v1` | `mention_scorers`, `mention_qualifiers`, `mention_notifiers` |
| `leadecho:reply_events` | `reply.draft_requested.v1`, `reply.approved.v1` | `reply_drafters` |
| `leadecho:workflow_events` | `workflow.trigger_requested.v1` | `workflow_executors` |
| `leadecho:ops_dead_letter` | Operational dead letters | — |
| `leadecho:ops_audit` | Unclassified / audit events | — |

Each consumer group uses `XREADGROUP` with explicit `XACK` on success. A shared **Retry Manager** periodically runs `XAUTOCLAIM` across all registered groups to recover messages left pending by crashed consumers.

---

## Domain Pipelines

### Mention Domain

The mention pipeline is the core signal path from crawl/extension ingest to outbound notification.

```text
mention.ingested.v1
        │
        v  [mention_scorers]
   ScoreMentionBatch()
        │
        ├── mention.scored.v1
        │         │
        │         v  [mention_qualifiers]  (optional, when enabled)
        │    QualifyMentionFromScore()
        │         │
        │         └── mention.qualified.v1
        │
        ├── mention.notification_requested.v1
        │         │
        │         v  [mention_notifiers]
        │    NotifyMentions()
        │
        └── workflow.trigger_requested.v1
                  │
                  v  [workflow_executors]
             EvaluateAndTrigger()
```

**Scoring (`mention_scorers`)**

- Consumes: `mention.ingested.v1`
- Executes: `monitor.ScoreMentionBatch()` — rules filter, embedding, intent classification
- Emits: `mention.scored.v1`, `mention.notification_requested.v1`, `workflow.trigger_requested.v1`

**Qualification (`mention_qualifiers`)**

- Consumes: `mention.scored.v1`
- Executes: `monitor.QualifyMentionFromScore()` — auto-creates leads when relevance ≥ 7.0 and intent is `buy_signal`, `recommendation_ask`, or `complaint`
- Emits: `mention.qualified.v1`
- **Decoupling note:** When `STREAMS_QUALIFIER_CONSUMER_ENABLED=true`, inline qualification inside the scorer is disabled. This is the intended production configuration for full eventization.

**Notification (`mention_notifiers`)**

- Consumes: `mention.notification_requested.v1`
- Executes: `monitor.NotifyMentions()` — Slack, Discord, email webhooks per workspace config

### Reply Domain

The reply domain separates **draft generation** from the HTTP request lifecycle.

```text
reply.draft_requested.v1
        │
        v  [reply_drafters]
   reply.Drafter.DraftForMention()
        │
        └── INSERT replies (status = draft)
```

**Producers of `reply.draft_requested.v1`:**

| Producer | Trigger |
|----------|---------|
| API | `POST /mentions/{id}/draft-reply` when async mode is enabled |
| Workflow engine | `ai_draft` action in an matched workflow's action chain |

**Async API behavior**

When `STREAMS_ENABLED` and `STREAMS_REPLY_DRAFTER_CONSUMER_ENABLED` are both true, the draft-reply endpoint publishes an event and returns `202 Accepted` with `{ status: "queued", event_id }`. The client polls or subscribes for the resulting draft reply.

When async mode is off, the handler calls `reply.Drafter` synchronously and returns the draft inline. This preserves backward compatibility.

**Approval events**

`PATCH /replies/{id}/status` with `status: "approved"` publishes `reply.approved.v1` when streams are enabled. No consumer is wired yet; this event is reserved for future post-approval automation (e.g. posting, workflow resume).

### Workflow Domain

The workflow domain connects scored mentions to user-defined automation rules stored in PostgreSQL.

```text
workflow.trigger_requested.v1
        │
        v  [workflow_executors]
   workflow.Engine.EvaluateAndTrigger()
        │
        ├── Match active workflows (trigger_config)
        ├── Dedup via workflow_executions
        ├── INSERT workflow_executions
        └── For each action in action_chain:
              ai_draft  → publish reply.draft_requested.v1
              others    → marked deferred (v1)
```

**Trigger matching** evaluates `trigger_config` JSON against the scored mention:

- `platforms` — platform allowlist
- `min_score` — minimum relevance score
- `intent_types` — intent allowlist
- `keywords` — optional content keyword filter

**v1 execution scope**

The workflow executor is intentionally minimal. It creates durable execution records and handles the `ai_draft` action by emitting into the reply stream. Actions such as `notify_slack`, `approval_gate`, and `post_reply` are recorded as `deferred` in step results. Full multi-step orchestration is deferred to a later phase; the event contracts and execution table are already in place.

---

## Persistence Layer

Migration `00014_redis_streams_backbone.sql` introduces four operational tables.

| Table | Role |
|-------|------|
| `event_log` | Authoritative event store; publish status tracking; replay source |
| `consumer_processed_events` | Per-group idempotency: `(consumer_group, event_id)` unique |
| `dead_letter_events` | Failed consumptions with error classification |
| `consumer_checkpoints` | Per-consumer heartbeat, last message ID, pending count |
| `event_replays` | Replay job tracking |

Workflow execution persistence uses the existing `workflows` and `workflow_executions` tables. New queries live in `internal/database/queries/workflows.sql`.

---

## Publish Path

`internal/events/publishers/publisher.go` implements the outbox pattern:

1. Insert row into `event_log` with `publish_status = 'pending'`
2. `XADD` serialized envelope to the target stream
3. Update `event_log` with `redis_message_id` and `publish_status = 'published'`
4. On Redis failure, mark `publish_status = 'failed'` with `last_error`

Stream selection is derived from `event_type` via `events.StreamForEventType()`. Producers do not choose streams manually.

---

## Consumer Infrastructure

### Worker packages

| Package | Responsibility |
|---------|----------------|
| `internal/events/consumers/mention_workers.go` | Scorer, qualifier, notifier handlers |
| `internal/events/consumers/reply_workers.go` | Reply draft handler |
| `internal/events/consumers/workflow_workers.go` | Workflow trigger handler |
| `internal/events/consumers/retry_manager.go` | Cross-stream `XAUTOCLAIM` recovery |
| `internal/events/consumers/util.go` | Shared loop, DLQ, idempotency helpers |

### Consumption contract

Every handler follows the same sequence:

1. Deserialize envelope from Redis message
2. Filter on `event_type` (ignore unrelated types in the same stream)
3. Check `consumer_processed_events` — return early if already processed
4. Execute domain service
5. Record processed event
6. `XACK` the message

On failure, the handler writes to `dead_letter_events` and updates `consumer_checkpoints` with the error. The message remains in the pending entries list until claimed by the retry manager or manually resolved.

### Reusable domain services

| Service | Package | Callable from |
|---------|---------|---------------|
| Scoring | `monitor.ScoreMentionBatch` | Scorer consumer, inline monitor |
| Qualification | `monitor.QualifyMentionFromScore` | Qualifier consumer, inline scorer (legacy) |
| Notification | `monitor.NotifyMentions` | Notifier consumer, inline monitor |
| Reply drafting | `reply.Drafter.DraftForMention` | Reply consumer, sync API |
| Workflow evaluation | `workflow.Engine.EvaluateAndTrigger` | Workflow consumer |

This layout ensures HTTP and stream paths share identical business logic.

---

## Configuration

All flags default to `false` except `STREAMS_INLINE_FALLBACK_ENABLED` (default `true`). Streams are opt-in.

| Environment Variable | Default | Effect |
|---------------------|---------|--------|
| `STREAMS_ENABLED` | `false` | Master switch for event publishing and worker startup |
| `STREAMS_DUAL_WRITE_ENABLED` | `false` | Monitor publishes events while retaining inline processing |
| `STREAMS_INLINE_FALLBACK_ENABLED` | `true` | When streams are on, inline monitor path still runs |
| `STREAMS_SCORER_CONSUMER_ENABLED` | `false` | Start `mention_scorers` worker |
| `STREAMS_QUALIFIER_CONSUMER_ENABLED` | `false` | Start `mention_qualifiers` worker; disable inline qualify in scorer |
| `STREAMS_NOTIFIER_CONSUMER_ENABLED` | `false` | Start `mention_notifiers` worker |
| `STREAMS_REPLY_DRAFTER_CONSUMER_ENABLED` | `false` | Start `reply_drafters` worker; API draft-reply returns 202 |
| `STREAMS_WORKFLOW_CONSUMER_ENABLED` | `false` | Start `workflow_executors` worker |
| `STREAMS_RETRY_CONSUMER_ENABLED` | `false` | Start cross-group retry manager |
| `STREAMS_CONSUMER_NAME` | `api-main` | Redis consumer name (unique per process instance) |
| `STREAMS_BATCH_SIZE` | `50` | `XREADGROUP` batch size |
| `STREAMS_BLOCK_MS` | `5000` | Blocking read timeout |
| `STREAMS_CLAIM_IDLE_MS` | `120000` | Idle threshold for `XAUTOCLAIM` |
| `STREAMS_MAX_ATTEMPTS` | `8` | Max retry attempts before permanent DLQ (reserved) |
| `STREAMS_WORKERS_IN_API` | `true` | Run stream consumers inside the API process |
| `PROCESS_ROLE` | `api` | `api` for HTTP server, `worker` for dedicated stream worker |
| `MONITOR_ENABLED` | `true` | Disable on API when monitor runs in worker process |
| `METRICS_ENABLED` | `false` | Expose Prometheus `/metrics` |
| `METRICS_PORT` | `9090` | Reserved for sidecar scrape configs |
| `METRICS_COLLECT_INTERVAL_SEC` | `30` | Poll interval for pending/DLQ gauges |
| `WORKER_HEALTH_PORT` | `8091` | Worker `/healthz`, `/readyz`, `/metrics` port |

### Split deployment (API + Worker)

For production, run two processes from the same Docker image:

| Process | Binary | Responsibilities |
|---------|--------|------------------|
| API | `/leadecho-api` | HTTP routes, event publishing, no monitor/workers when split |
| Worker | `/leadecho-worker` | Monitor crawl loop, all stream consumers, health/metrics |

**API environment (split mode):**

```bash
PROCESS_ROLE=api
MONITOR_ENABLED=false
STREAMS_WORKERS_IN_API=false
STREAMS_ENABLED=true          # publishers still active
METRICS_ENABLED=true          # optional: /metrics on :8090
```

**Worker environment:**

```bash
PROCESS_ROLE=worker
STREAMS_ENABLED=true
STREAMS_CONSUMER_NAME=worker-1   # unique per replica
METRICS_ENABLED=true
WORKER_HEALTH_PORT=8091
# enable individual consumer flags as needed
```

`docker-compose.prod.yml` includes a `worker` service with this layout. Scale workers horizontally with distinct `STREAMS_CONSUMER_NAME` values.

### Prometheus metrics

When `METRICS_ENABLED=true`, the following metrics are exposed:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `leadecho_stream_consumer_pending` | Gauge | `stream`, `group` | Redis `XPENDING` count |
| `leadecho_stream_messages_processed_total` | Counter | `stream`, `group`, `result` | Handled messages (`success` / `error`) |
| `leadecho_stream_message_duration_seconds` | Histogram | `stream`, `group` | Handler latency |
| `leadecho_stream_dlq_open_total` | Gauge | — | Open dead-letter rows |

- API serves `/metrics` on the main port (`8090`) when enabled.
- Worker serves `/metrics` on `WORKER_HEALTH_PORT` (`8091`) alongside `/healthz` and `/readyz`.

Suggested alerts:

- `leadecho_stream_consumer_pending > 100` for 5m → consumer lag
- `leadecho_stream_dlq_open_total` increasing → handler failures
- `rate(leadecho_stream_messages_processed_total{result="error"}[5m]) > 0` → sustained errors

---

**Phase 1 — Observe (no behavior change)**

```bash
STREAMS_ENABLED=true
STREAMS_DUAL_WRITE_ENABLED=true
STREAMS_INLINE_FALLBACK_ENABLED=true
# All consumer flags remain false
```

Events flow into `event_log` and Redis. Inline monitor behavior is unchanged. Validate event volume and payload shape.

**Phase 2 — Async consumers with fallback**

```bash
STREAMS_SCORER_CONSUMER_ENABLED=true
STREAMS_NOTIFIER_CONSUMER_ENABLED=true
STREAMS_RETRY_CONSUMER_ENABLED=true
```

Consumers process events in parallel with inline path. Compare outcomes before disabling fallback.

**Phase 3 — Full domain separation**

```bash
STREAMS_QUALIFIER_CONSUMER_ENABLED=true
STREAMS_REPLY_DRAFTER_CONSUMER_ENABLED=true
STREAMS_WORKFLOW_CONSUMER_ENABLED=true
STREAMS_INLINE_FALLBACK_ENABLED=false
```

Qualification, drafting, and workflow triggering run exclusively through streams.

---

## Admin and Operability

HTTP endpoints under `/api/v1/streams/` (authenticated):

| Endpoint | Purpose |
|----------|---------|
| `GET /streams/status` | Consumer checkpoint snapshot |
| `GET /streams/dead-letters` | Open DLQ entries |
| `GET /streams/replays` | Replay job history |
| `POST /streams/replays` | Trigger replay from `event_log` |

Replay re-publishes historical events from PostgreSQL into Redis. This supports recovery after consumer bugs or deployment of fixed handlers.

---

## Package Layout

```text
backend/
├── cmd/
│   ├── api/main.go               # HTTP API server
│   └── worker/main.go            # Monitor + stream consumers
├── migrations/00014_redis_streams_backbone.sql
├── internal/
│   ├── platform/bootstrap.go     # Shared wiring for api + worker
│   ├── streamworkers/runtime.go  # Consumer startup orchestration
│   ├── metrics/                  # Prometheus metrics + collector
│   ├── events/
│   │   ├── model.go              # Envelope + payload types
│   │   ├── streams.go            # Stream/group constants
│   │   ├── publishers/           # Outbox publisher
│   │   ├── redis/                # XADD, XREADGROUP, XACK, XAUTOCLAIM
│   │   └── consumers/            # Worker loops + retry manager
│   ├── monitor/
│   │   ├── scorer.go             # Scoring + event emission
│   │   ├── services.go           # ScoreMentionBatch, QualifyMentionFromScore, NotifyMentions
│   │   └── stream_pipeline.go    # Dual-write ingest path
│   ├── reply/
│   │   └── drafter.go            # Shared draft generation service
│   └── workflow/
│       ├── types.go              # TriggerConfig, ActionConfig
│       └── engine.go             # Trigger evaluation + v1 execution
```

---

## What Changed vs. the Previous Architecture

| Concern | Before | After |
|---------|--------|-------|
| Pipeline shape | Single inline monitor tick | Staged event pipeline with independent consumer groups |
| Lead qualification | Embedded in scorer | Optional dedicated `mention_qualifiers` consumer |
| Reply drafting | Synchronous HTTP only | Event-driven async path via `reply_drafters` |
| Workflow execution | Schema only, no runtime | Trigger listener + execution records + `ai_draft` emission |
| Failure handling | Log and continue | DLQ, checkpoints, autoclaim retry |
| Idempotency | None | `consumer_processed_events` + publish idempotency keys |
| Scaling | Scale entire API/monitor | Enable and scale individual consumer groups |
| Migration | All-or-nothing | Dual-write + per-flag rollout |

---

## Current Limitations and Next Steps

The backbone is production-ready for the paths described above. The following items are explicitly out of scope for this phase:

1. **Workflow action completeness** — Only `ai_draft` executes. Other actions are deferred and recorded in `workflow_executions.steps`.
2. **`reply.approved.v1` consumer** — Event is published; no downstream handler for auto-post or workflow resume yet.
3. **Separate worker deployment** — Implemented via `cmd/worker` and `PROCESS_ROLE=worker`. API can disable workers with `STREAMS_WORKERS_IN_API=false`.
4. **Cross-process consumer naming** — Multiple worker replicas must use distinct `STREAMS_CONSUMER_NAME` values to avoid claim conflicts within the same group.
5. **Metrics export** — Prometheus metrics implemented; Grafana dashboard definitions are not yet checked in.

Recommended next increments:

- Wire `reply.approved.v1` to an approval-gate resume handler
- Implement `notify_slack` and `approval_gate` workflow actions as stream consumers
- Add integration tests that exercise the full mention → workflow → draft chain against Redis testcontainers
- Extract high-volume consumer groups into dedicated Railway services

---

## Related Documents

- [01-system-architecture.md](./01-system-architecture.md) — Overall system context
- [05-signal-engine.md](./05-signal-engine.md) — Mention scoring pipeline design
- [08-workflow-engine.md](./08-workflow-engine.md) — Workflow product model and long-term executor design
- [10-infrastructure-deployment.md](./10-infrastructure-deployment.md) — Deployment topology
