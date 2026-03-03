# LeadEcho - RAG Pipeline Implementation Guide

## Overview

The RAG pipeline generates persona-aware reply drafts using the user's own product docs, FAQs, and past successful comments. Unlike generic AI replies, RAG produces responses that sound like the user.

```
User uploads docs → Chunk → Embed (Voyage AI) → Store in pgvector
                                                        ↓
Mention detected → Retrieve relevant chunks → Assemble context → Claude Sonnet → 3 reply variants
```

---

## Document Ingestion Pipeline

### Processing Flow

```go
func (p *Pipeline) IngestDocument(ctx context.Context, doc Document) error {
    // 1. Extract text based on format
    text, err := p.extractText(doc)
    if err != nil {
        return fmt.Errorf("extract: %w", err)
    }

    // 2. Chunk the text
    chunks := p.chunker.Chunk(text, doc.ContentType)

    // 3. Generate embeddings (batch)
    embeddings, err := p.embedder.EmbedBatch(ctx, chunksToTexts(chunks))
    if err != nil {
        return fmt.Errorf("embed: %w", err)
    }

    // 4. Store chunks + embeddings in PostgreSQL
    for i, chunk := range chunks {
        chunk.Embedding = embeddings[i]
        if err := p.db.InsertChunk(ctx, chunk); err != nil {
            return fmt.Errorf("store chunk %d: %w", i, err)
        }
    }

    // 5. Update document metadata
    return p.db.UpdateDocumentChunkCount(ctx, doc.ID, len(chunks))
}
```

### Supported Formats

| Format | Parser | Notes |
|--------|--------|-------|
| Markdown (.md) | goldmark | Split by headers |
| PDF (.pdf) | pdfcpu / unipdf | Extract text, handle multi-page |
| Plain text (.txt) | Direct | Split by paragraphs |
| URL | go-readability | Fetch + extract main content |

---

## Chunking Strategy

### Recursive Splitting

```go
type ChunkerConfig struct {
    MaxTokens   int // 512
    Overlap     int // 50 tokens
    Separators  []string
}

func (c *Chunker) Chunk(text string, docType string) []Chunk {
    switch docType {
    case "faq":
        return c.chunkByQAPairs(text)
    case "comments":
        return c.chunkByComment(text)
    default:
        return c.recursiveChunk(text)
    }
}

func (c *Chunker) recursiveChunk(text string) []Chunk {
    separators := []string{"\n## ", "\n### ", "\n\n", "\n", ". ", " "}

    var chunks []Chunk
    for _, section := range splitBySeparators(text, separators, c.config.MaxTokens) {
        chunks = append(chunks, Chunk{
            Content:    section.Text,
            TokenCount: countTokens(section.Text),
            Section:    section.Title,
        })
    }

    // Add overlap between consecutive chunks
    return addOverlap(chunks, c.config.Overlap)
}
```

### Strategy by Document Type

| Type | Strategy | Chunk Size |
|------|----------|-----------|
| Product docs | Split by headers/sections | 512 tokens |
| FAQs | One Q&A pair per chunk | Variable |
| Past comments | One comment per chunk | Variable |
| Brand guidelines | Split by section | 512 tokens |

---

## Embedding Generation (Voyage AI)

```go
type VoyageEmbedder struct {
    apiKey string
    model  string // "voyage-3"
    client *http.Client
}

func (v *VoyageEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
    // Batch up to 128 texts per API call
    var allEmbeddings [][]float32

    for i := 0; i < len(texts); i += 128 {
        batch := texts[i:min(i+128, len(texts))]

        body := map[string]any{
            "input":      batch,
            "model":      v.model,
            "input_type": "document", // "query" for search queries
        }

        resp, err := v.post(ctx, "https://api.voyageai.com/v1/embeddings", body)
        if err != nil {
            return nil, fmt.Errorf("voyage embed: %w", err)
        }

        for _, emb := range resp.Data {
            allEmbeddings = append(allEmbeddings, emb.Embedding)
        }
    }
    return allEmbeddings, nil
}
```

**Cost:** ~$0.06 per 1M tokens. A 10-page product doc ≈ 5K tokens ≈ $0.0003.

---

## pgvector Configuration

### Index Setup

```sql
-- HNSW index: best for query-time performance
-- m=16: connections per node (higher = better recall, more memory)
-- ef_construction=64: build-time search width (higher = better index quality)
CREATE INDEX idx_chunks_embedding ON document_chunks
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Set search precision at query time
SET hnsw.ef_search = 40; -- default 40, increase for better recall
```

### Vector Query

```sql
-- name: SearchChunksByVector :many
SELECT id, content, section_title, document_id,
       1 - (embedding <=> @query_embedding::vector) AS similarity
FROM document_chunks
WHERE workspace_id = @workspace_id
  AND (1 - (embedding <=> @query_embedding::vector)) > 0.3
ORDER BY embedding <=> @query_embedding::vector
LIMIT @max_results;
```

---

## Hybrid Search (BM25 + Vector)

```go
func (r *Retriever) HybridSearch(ctx context.Context, query string, workspaceID string, limit int) ([]RetrievedChunk, error) {
    // 1. Vector search
    queryEmb, _ := r.embedder.Embed(ctx, query, "query")
    vectorResults, _ := r.db.SearchChunksByVector(ctx, queryEmb, workspaceID, limit*2)

    // 2. BM25 keyword search
    keywordResults, _ := r.db.SearchChunksByKeyword(ctx, query, workspaceID, limit*2)

    // 3. Reciprocal Rank Fusion (RRF)
    return reciprocalRankFusion(vectorResults, keywordResults, limit), nil
}

func reciprocalRankFusion(vectorResults, keywordResults []RetrievedChunk, limit int) []RetrievedChunk {
    const k = 60 // RRF constant
    scores := make(map[string]float64)

    for rank, r := range vectorResults {
        scores[r.ID] += 1.0 / (float64(k + rank + 1))
    }
    for rank, r := range keywordResults {
        scores[r.ID] += 1.0 / (float64(k + rank + 1))
    }

    // Sort by combined RRF score, return top N
    // ...
}
```

### Keyword Search SQL

```sql
-- name: SearchChunksByKeyword :many
SELECT id, content, section_title, document_id,
       ts_rank(content_tsv, websearch_to_tsquery('english', @query)) AS rank
FROM document_chunks
WHERE workspace_id = @workspace_id
  AND content_tsv @@ websearch_to_tsquery('english', @query)
ORDER BY rank DESC
LIMIT @max_results;
```

---

## Reply Generation Pipeline

```go
func (g *Generator) GenerateReplies(ctx context.Context, req ReplyRequest) (*ReplyVariants, error) {
    // Step 1: Retrieve relevant knowledge base chunks
    chunks, _ := g.retriever.HybridSearch(ctx, req.Mention.Content, req.WorkspaceID, 8)

    // Step 2: Retrieve similar successful past replies
    exemplars, _ := g.retriever.GetExemplars(ctx, req.Mention.Content, req.WorkspaceID, 3)

    // Step 3: Get thread context
    thread, _ := g.threadFetcher.GetThread(ctx, req.Mention)

    // Step 4: Assemble prompt
    prompt := g.buildReplyPrompt(req.Mention, chunks, exemplars, thread, req.Persona)

    // Step 5: Generate 3 variants via Claude Sonnet
    resp, err := g.claude.Messages.New(ctx, anthropic.MessageNewParams{
        Model:     anthropic.ModelClaudeSonnet4_6,
        MaxTokens: 2000,
        System:    []anthropic.TextBlockParam{{Text: replySystemPrompt}},
        Messages:  []anthropic.MessageParam{{Role: "user", Content: prompt}},
    })

    // Step 6: Parse variants
    return parseVariants(resp), nil
}
```

### Reply Generation System Prompt

```go
const replySystemPrompt = `You are a social media engagement expert writing replies for a SaaS product.

RULES:
1. Sound like a real person, not a bot. Use the persona examples provided.
2. Never start with "Great question!" or similar filler.
3. Lead with genuine value before any mention of the product.
4. Match the platform's tone: HN=technical, Reddit=casual, LinkedIn=professional, X=concise.
5. Generate exactly 3 variants:
   - value_only: Pure helpful advice, no product mention at all
   - technical: Technical explanation that naturally leads to the product
   - soft_sell: Brief product mention with context on why it's relevant

For each variant, output JSON:
{
  "variants": [
    { "type": "value_only", "content": "...", "estimated_reception": "..." },
    { "type": "technical", "content": "...", "estimated_reception": "..." },
    { "type": "soft_sell", "content": "...", "estimated_reception": "..." }
  ],
  "thread_analysis": {
    "sentiment": "positive|negative|neutral|hostile",
    "solutions_mentioned": ["..."],
    "our_product_mentioned": false,
    "recommended_variant": "value_only",
    "skip_reason": null
  }
}`
```

---

## Persona Matching

```go
func (g *Generator) buildPersonaContext(workspace Workspace, exemplars []RetrievedChunk) string {
    var sb strings.Builder

    sb.WriteString("## Brand Voice\n")
    sb.WriteString(workspace.Settings.BrandVoice) // e.g., "Technical but approachable, uses specific examples"

    sb.WriteString("\n\n## Past Successful Replies (match this style):\n")
    for _, ex := range exemplars {
        sb.WriteString(fmt.Sprintf("- Platform: %s | Score: %.1f\n%s\n\n",
            ex.Metadata["platform"], ex.EffectivenessScore, ex.Content))
    }

    sb.WriteString("\n## Platform-Specific Guidance:\n")
    // Add platform-specific tone guidance based on mention.Platform
    return sb.String()
}
```

---

## Thread-Aware Context

```go
func (g *Generator) buildThreadContext(thread *Thread) string {
    if thread == nil {
        return "No thread context available."
    }

    // Truncate if thread exceeds 4K tokens
    messages := thread.Messages
    totalTokens := 0
    for i, msg := range messages {
        totalTokens += countTokens(msg.Content)
        if totalTokens > 4000 {
            messages = messages[:i]
            break
        }
    }

    var sb strings.Builder
    sb.WriteString("## Full Thread Context\n\n")
    for _, msg := range messages {
        sb.WriteString(fmt.Sprintf("**%s** (%s):\n%s\n\n", msg.Author, msg.Timestamp, msg.Content))
    }
    return sb.String()
}
```

---

## Learning Feedback Loop

### Track Outcomes

```go
// When a reply is posted, track its outcome
func (l *Learner) TrackReply(ctx context.Context, reply Reply) {
    // Posted → check if upvoted/removed after 24h
    // Posted → check UTM clicks
    // Posted → check UTM signups
}

// Promote successful replies to exemplars
func (l *Learner) PromoteExemplar(ctx context.Context, replyID string) error {
    reply, _ := l.db.GetReply(ctx, replyID)

    // Store as exemplar chunk with high retrieval weight
    return l.db.InsertChunk(ctx, DocumentChunk{
        WorkspaceID:        reply.WorkspaceID,
        Content:           reply.EditedContent, // Use human-edited version
        IsExemplar:        true,
        EffectivenessScore: calculateEffectiveness(reply),
        Metadata: map[string]any{
            "platform":    reply.Platform,
            "variant":     reply.Variant,
            "clicks":      reply.UTMClicks,
            "conversions": reply.UTMSignups,
        },
    })
}
```

---

## Cost Estimation

| Task | Model | Tokens/Call | Cost/1K Calls |
|------|-------|------------|---------------|
| Relevance scoring (batch 10) | Haiku 4.5 | ~2K in, ~500 out | ~$0.10 |
| Intent classification | Haiku 4.5 | ~1K in, ~100 out | ~$0.05 |
| Thread analysis | Sonnet 4.6 | ~4K in, ~500 out | ~$0.50 |
| Reply generation (3 variants) | Sonnet 4.6 | ~6K in, ~1.5K out | ~$0.80 |
| Embedding (Voyage) | voyage-3 | ~500 tokens | ~$0.00003 |

**Per 1,000 mentions processed:** ~$150-250 (scoring) + ~$800 (replies for top 10%) ≈ $200-350 total

**Optimization strategies:**
- Only generate replies for mentions with score >= 7.0 (top ~10%)
- Use Haiku for scoring, Sonnet only for generation
- Batch score 10 mentions per API call
- Cache embeddings for unchanged documents
