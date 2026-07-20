package knowledge

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	pgvector "github.com/pgvector/pgvector-go"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/llm"
)

// Embedder embeds text batches for indexing and retrieval queries.
type Embedder interface {
	EmbedTexts(ctx context.Context, workspaceID string, task llm.Task, texts []string) ([]pgvector.Vector, error)
}

// Service indexes workspace documents into vector chunks and retrieves relevant context.
type Service struct {
	q        *database.Queries
	embedder Embedder
	logger   zerolog.Logger
}

func NewService(q *database.Queries, embedder Embedder, logger zerolog.Logger) *Service {
	return &Service{
		q:        q,
		embedder: embedder,
		logger:   logger.With().Str("component", "knowledge").Logger(),
	}
}

// IndexDocument replaces all chunks for a document with freshly embedded segments.
func (s *Service) IndexDocument(ctx context.Context, workspaceID, documentID, title, content string) error {
	if s.embedder == nil {
		return fmt.Errorf("embedding provider not configured")
	}

	chunks := Chunk(content, defaultMaxChunkRunes, defaultOverlapRunes)
	if len(chunks) == 0 {
		if err := s.q.DeleteDocumentChunks(ctx, database.DeleteDocumentChunksParams{
			DocumentID:  documentID,
			WorkspaceID: workspaceID,
		}); err != nil {
			return fmt.Errorf("delete empty chunks: %w", err)
		}
		return s.q.UpdateDocumentChunkCount(ctx, database.UpdateDocumentChunkCountParams{
			ID: documentID, ChunkCount: 0,
		})
	}

	vectors, err := s.embedder.EmbedTexts(ctx, workspaceID, llm.TaskEmbedDocuments, chunks)
	if err != nil {
		return fmt.Errorf("embed document chunks: %w", err)
	}
	if len(vectors) != len(chunks) {
		return fmt.Errorf("embedder returned %d vectors for %d chunks", len(vectors), len(chunks))
	}

	if err := s.q.DeleteDocumentChunks(ctx, database.DeleteDocumentChunksParams{
		DocumentID:  documentID,
		WorkspaceID: workspaceID,
	}); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}

	for i, chunk := range chunks {
		section := title
		if len(chunks) > 1 {
			section = fmt.Sprintf("%s §%d", title, i+1)
		}
		if _, err := s.q.InsertDocumentChunk(ctx, database.InsertDocumentChunkParams{
			DocumentID:   documentID,
			WorkspaceID:  workspaceID,
			Content:      chunk,
			Embedding:    &vectors[i],
			ChunkIndex:   int32(i),
			SectionTitle: pgtype.Text{String: section, Valid: true},
		}); err != nil {
			return fmt.Errorf("insert chunk %d: %w", i, err)
		}
	}

	return s.q.UpdateDocumentChunkCount(ctx, database.UpdateDocumentChunkCountParams{
		ID: documentID, ChunkCount: int32(len(chunks)),
	})
}

// DeleteDocument removes all vector chunks for a document.
func (s *Service) DeleteDocument(ctx context.Context, workspaceID, documentID string) error {
	return s.q.DeleteDocumentChunks(ctx, database.DeleteDocumentChunksParams{
		DocumentID:  documentID,
		WorkspaceID: workspaceID,
	})
}

// Retrieve returns the top-k most similar KB chunks for a query string.
func (s *Service) Retrieve(ctx context.Context, workspaceID, query string, topK int32) (string, error) {
	if s.embedder == nil || strings.TrimSpace(query) == "" {
		return "", nil
	}
	if topK <= 0 {
		topK = 5
	}

	vectors, err := s.embedder.EmbedTexts(ctx, workspaceID, llm.TaskEmbedDocuments, []string{query})
	if err != nil {
		return "", err
	}
	if len(vectors) == 0 {
		return "", nil
	}

	rows, err := s.q.FindSimilarDocumentChunks(ctx, database.FindSimilarDocumentChunksParams{
		QueryEmbedding: &vectors[0],
		WorkspaceID:    workspaceID,
		Lim:            topK,
	})
	if err != nil {
		return "", err
	}

	var parts []string
	for _, row := range rows {
		if row.Similarity < 0.35 {
			continue
		}
		label := row.SectionTitle.String
		if label == "" {
			label = "Knowledge"
		}
		parts = append(parts, label+":\n"+row.Content)
	}
	return strings.Join(parts, "\n---\n"), nil
}
