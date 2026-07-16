# AI Router Architecture

## Executive Summary

The AI Router introduces a workspace-aware control plane for all LLM and embedding calls in LeadEcho. Its job is to replace scattered provider selection with one routing layer that understands provider capabilities, per-workspace BYOK credentials, model tiers, task routing, verification, fallback behavior, and usage tracking.

This is intentionally implemented as a thin internal router rather than a generic LLM orchestration framework. LeadEcho does not need chains, agents, memory abstractions, or tool orchestration for this problem. It needs deterministic routing, secure key handling, operational visibility, and a Settings experience that is easy for users to reason about.

## Design Goals

The router is designed around five architecture goals.

1. **Workspace-level BYOK**

   Each workspace can bring its own provider keys. Those keys are encrypted at rest and take priority over system-level environment keys.

2. **Provider/model decoupling**

   Product code should request a domain task such as classification, reply drafting, product analysis, or embedding. It should not know which provider or model is currently configured.

3. **Cost and quality control**

   High-volume tasks can use faster and cheaper models, while higher-value generation tasks can use stronger models. Embeddings remain separate because they are not interchangeable with chat models.

4. **Single source of truth**

   The backend owns the provider catalog, supported capabilities, default models, and recommended models. The frontend renders what the backend declares instead of maintaining a second hard-coded provider list.

5. **Operational visibility**

   Every model attempt can be recorded with task, provider, model, key source, latency, status, and fallback metadata.

## High-Level Architecture

```text
Dashboard Settings
  |
  | GET /api/v1/llm/config
  | PUT /api/v1/llm/config
  | PUT /api/v1/llm/providers/{provider}/key
  | POST /api/v1/llm/providers/{provider}/verify
  v
LLM HTTP Handler
  |
  v
internal/llm.Router
  |
  |-- provider catalog
  |-- encrypted workspace keys
  |-- system key fallback
  |-- task -> tier routing
  |-- tier -> provider/model target
  |-- usage recording
  |
  +--> internal/ai providers
  |
  +--> internal/embedding providers
```

The router sits between LeadEcho's product pipeline and provider-specific clients. Callers no longer instantiate providers directly. They ask the router to perform a domain-level operation, and the router resolves the concrete provider/model at runtime.

## Core Concepts

### Provider

A provider is an external AI service such as OpenAI, OpenRouter, DeepSeek, GLM, NVIDIA, or Voyage AI.

Provider metadata is declared in the backend registry:

```go
ProviderStatus{
    Provider:          "openai",
    DisplayName:       "OpenAI",
    Capabilities:      []string{"chat"},
    DefaultModel:      "gpt-4o-mini",
    RecommendedModels: []string{"gpt-4o-mini", "gpt-4o"},
}
```

This metadata is returned to the frontend through `GET /api/v1/llm/config`. The frontend uses it to render provider cards and model dropdowns.

### Capability

Capabilities describe what a provider can be used for.

Current MVP capabilities:

- `chat`: filtering, classification, reply drafting, product analysis
- `embedding`: semantic matching and vector generation

This separation matters because chat models and embedding models are not interchangeable. An embedding model produces vectors. A chat model produces text.

### Model Target

A model target is the concrete runtime selection:

```json
{
  "provider": "openai",
  "model": "gpt-4o-mini",
  "base_url": ""
}
```

Targets can be assigned to tiers such as `cheap`, `strong`, and `embedding`.

### Model Tier

The router uses three model tiers:

| Tier | Purpose | Typical Tasks |
|---|---|---|
| `cheap` | Low-cost, low-latency chat calls | spam filtering, intent classification, reply pre-checks |
| `strong` | Higher-quality chat calls | reply drafting, product analysis |
| `embedding` | Vector generation | mention/profile/document embeddings |

The names are implementation-facing. Product UI can present them as `Fast model`, `Quality model`, and `Embedding model` if we want friendlier wording.

### Task Routing

Each AI task maps to a tier:

```text
filter             -> cheap
classify           -> cheap
pre_filter_reply   -> cheap
draft_reply        -> strong
analyze_product    -> strong
embed_mentions     -> embedding
embed_profiles     -> embedding
embed_documents    -> embedding
```

This lets the product change model behavior without changing scoring, onboarding, profile, or reply code.

## Backend Responsibilities

The backend is the source of truth for provider behavior.

It owns:

- supported provider IDs
- display names
- capabilities
- default models
- recommended models
- system key fallback
- key decryption
- provider verification
- runtime routing
- usage recording

The frontend should not maintain its own provider/model catalog. If the frontend hard-codes provider options, it can drift from actual backend support. For example, the UI might display Gemini even though the router cannot call Gemini yet. That creates a broken product path.

The correct boundary is:

```text
Backend: declares what the system supports.
Frontend: renders choices and captures user intent.
User: selects provider/model and supplies keys.
Router: executes calls according to saved workspace config.
```

## Frontend Responsibilities

The Settings UI is a control surface, not an AI provider registry.

It is responsible for:

- loading `/api/v1/llm/config`
- rendering provider cards
- collecting provider API keys
- showing whether a key is configured
- verifying provider keys
- rendering tier-level provider/model selectors
- allowing custom model IDs
- saving routing configuration
- displaying recent usage

The UI should allow provider/model selection even when a key is not configured. Missing keys should be shown clearly, but selection should not be blocked. Verification and runtime calls require keys.

## BYOK Key Model

Provider keys can come from two sources.

1. **Workspace BYOK key**

   Stored in `workspaces.settings["llm"]`, encrypted at rest.

2. **System key**

   Loaded from environment variables such as `OPENAI_API_KEY`, `DEEPSEEK_API_KEY`, or `VOYAGE_API_KEY`.

Resolution order:

```text
workspace key -> system key -> error
```

This gives self-hosted teams a default deployment path while still allowing each workspace to override provider credentials.

## Runtime Flow

### Chat Task

```text
AI pipeline stage
  -> router.ClassifyIntent / DraftReplyEnhanced / AnalyzeProductPage
  -> resolve task tier
  -> resolve model target
  -> resolve workspace/system key
  -> instantiate provider
  -> execute provider call
  -> record usage event
```

### Embedding Task

```text
monitor/profile/onboarding embedding call
  -> router.EmbedTexts
  -> resolve embedding tier
  -> resolve embedding provider/model
  -> resolve workspace/system key
  -> execute embedding client
  -> record usage event
```

In the current MVP, embedding is implemented through Voyage AI because existing pgvector columns expect 1024-dimensional vectors from `voyage-3`.

## HTTP API

The LLM Router exposes a compact configuration API:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/v1/llm/config` | Load provider catalog, current workspace config, and health |
| `PUT` | `/api/v1/llm/config` | Save model tiers and task routing |
| `PUT` | `/api/v1/llm/providers/{provider}/key` | Save encrypted provider key |
| `DELETE` | `/api/v1/llm/providers/{provider}/key` | Remove workspace provider key |
| `POST` | `/api/v1/llm/providers/{provider}/verify` | Verify configured provider key |
| `GET` | `/api/v1/llm/usage` | Return 30-day usage summary |

## Data Storage

### Workspace Settings

The router stores configuration inside workspace settings JSON:

```json
{
  "llm": {
    "providers": {
      "openai": {
        "api_key": "<encrypted>",
        "base_url": "",
        "enabled": true
      }
    },
    "models": {
      "cheap": {
        "provider": "openai",
        "model": "gpt-4o-mini"
      },
      "strong": {
        "provider": "openai",
        "model": "gpt-4o"
      },
      "embedding": {
        "provider": "voyage",
        "model": "voyage-3"
      }
    },
    "routing": {
      "classify": "cheap",
      "draft_reply": "strong",
      "embed_mentions": "embedding"
    },
    "fallbacks": {}
  }
}
```

The router also reads legacy `settings["api_keys"]` so existing saved keys continue to work during the migration to the new LLM configuration model.

### Usage Events

Usage events are stored in `llm_usage_events`:

```text
workspace_id
task
provider
model
status
prompt_tokens
completion_tokens
total_tokens
estimated_cost_usd
latency_ms
error_message
fallback_from
key_source
created_at
```

Token and cost fields are present even if the current MVP adapters do not yet extract provider usage metadata. The schema is designed for future cost reporting.

## Migration Notes

The usage table is introduced through SQL migrations. Existing local databases may already be past the original migration version, so a later idempotent migration can be used to ensure the table exists:

```sql
CREATE TABLE IF NOT EXISTS llm_usage_events (...);
```

The runtime also handles a missing usage table gracefully. Usage recording should not break AI execution, and usage summary should return an empty result rather than making Settings unusable.

## Security Considerations

The security boundary is workspace scoped.

- Provider keys are encrypted before being stored.
- Raw keys are never returned by API responses.
- Public config responses only include masked keys and key source.
- Workspace keys take priority over system keys.
- Settings routes are protected by normal workspace authentication middleware.
- Verification calls use the same key resolution path as runtime calls.

Operationally, the encryption key must not be reused with unrelated production secrets.

## Failure Behavior

The router should fail in ways that are explicit and debuggable:

| Scenario | Expected Behavior |
|---|---|
| Provider not known | Reject configuration or key save |
| No key configured | Return a clear `no key configured` error |
| Usage table missing | Skip usage recording and return empty usage summary |
| Provider verification fails | Return a provider-level error |
| Embedding provider unsupported | Return an explicit unsupported provider error |

This keeps product flows understandable while avoiding hidden misrouting.

## Current MVP Boundaries

Supported chat providers:

- NVIDIA
- DeepSeek
- GLM / ZhipuAI
- OpenAI
- OpenRouter

Supported embedding provider:

- Voyage AI

Current limitations:

- Anthropic and Gemini may be reachable through OpenRouter, but are not first-class direct adapters yet.
- Ollama/local models are not wired as first-class providers yet.
- Embedding is currently Voyage-only because vector dimensions must remain compatible with existing pgvector columns.
- Usage tracking records attempts and latency, but token/cost extraction is still adapter-dependent future work.
- Fallback UI is minimal even though the backend configuration shape already reserves fallback targets.

## Why Not LangChain?

The router solves provider selection, key resolution, tier routing, and observability. Those are control-plane concerns. LangChain-style abstractions are better suited for agent workflows, chain composition, tools, memory, and multi-step orchestration.

For this MVP, adopting a larger framework would add surface area without solving the core problem better than a small internal router.

The router therefore stays close to LeadEcho's domain:

- classify a mention
- pre-filter a reply
- draft a reply
- analyze a product page
- embed profile/mention/document text

That keeps runtime behavior predictable and easier to debug.

## Extension Path

The architecture leaves room for future expansion.

1. **Direct Anthropic/Gemini adapters**

   Add provider metadata, system keys, verification, and provider construction.

2. **Local Ollama support**

   Add a provider with a configurable base URL and no cloud key requirement.

3. **Provider-level model discovery**

   Some providers expose model list endpoints. The backend can merge dynamic model discovery with static recommended defaults.

4. **Cost reporting**

   Add provider-specific token parsing and cost tables.

5. **Fallback UI**

   Expose tier fallback chains in Settings so users can configure failover order.

6. **Per-task overrides**

   Let advanced workspaces route individual tasks directly to provider/model targets instead of tier names.

## Architectural Position

The AI Router is the boundary between product intent and AI infrastructure.

Product code should express what it wants to do. The router decides how that intent maps to the current workspace's AI configuration.

This keeps LeadEcho's core workflows stable while allowing AI provider strategy to evolve independently.
