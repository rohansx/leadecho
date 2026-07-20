# Intent Core Foundation — Design

**Goal:** Ship Triage LLM filter, Knowledge Base vector RAG, and Researcher Person360 L1 with clean package boundaries.

## Architecture

```
Monitor.batchScoreMentions
  Stage 0: rules (free)
  Stage 1: llm.FilterMention (cheap tier)
  Stage 2–4: unchanged

DocumentHandler ──► knowledge.Service.IndexDocument
reply.Drafter     ──► knowledge.Service.Retrieve(query embedding)

Monitor.qualifyAsLead ──► researcher.Service.EnrichLead (async goroutine)

GET /mentions/{id}/person360 ──► person + identities
```

## Packages

| Package | Responsibility |
|---------|----------------|
| `internal/knowledge` | Chunk, embed, store, cosine-retrieve document chunks |
| `internal/researcher` | Person360 identity stitching; GitHub L1 enrichment |
| `internal/ai` | FilterMention prompt (cheap model spam gate) |

## Principles

- Fail-open on LLM filter errors (log + proceed) to avoid dropping real leads
- Fail-closed on enrichment (no auto-merge below confidence 0.6)
- Document re-index is replace-all (delete chunks → insert fresh)
- BYOK: all LLM/embedding via existing `llm.Router`
