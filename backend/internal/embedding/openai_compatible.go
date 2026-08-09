package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	pgvector "github.com/pgvector/pgvector-go"
)

// EmbeddingDims is the vector width every stored embedding must have: the
// pgvector columns are declared vector(1024), so a provider returning any other
// width cannot be written. Providers that support a dimensions parameter are
// asked for exactly this.
const EmbeddingDims = 1024

// CompatClient calls an OpenAI-compatible /embeddings endpoint.
//
// This exists so a workspace can run the whole pipeline on a single API key.
// Embeddings used to be Voyage-only, which quietly made BYOK a two-key
// requirement — a user with just an OpenAI or GLM key got chat and drafting but
// no semantic matching, the very thing that separates this from keyword alerts.
type CompatClient struct {
	apiKey  string
	model   string
	baseURL string
	// sendDimensions is false for providers that reject the parameter.
	sendDimensions bool
	http           *http.Client
}

// NewCompatible builds a client for an OpenAI-shaped embeddings API.
func NewCompatible(apiKey, baseURL, model string, sendDimensions bool) *CompatClient {
	return &CompatClient{
		apiKey:         apiKey,
		model:          model,
		baseURL:        baseURL,
		sendDimensions: sendDimensions,
		http:           &http.Client{Timeout: 60 * time.Second},
	}
}

type compatRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type compatResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// EmbedTexts embeds a batch of texts in one call.
func (c *CompatClient) EmbedTexts(ctx context.Context, texts []string) ([]pgvector.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body := compatRequest{Model: c.model, Input: texts}
	if c.sendDimensions {
		body.Dimensions = EmbeddingDims
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embed response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result compatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal embed response: %w", err)
	}
	if result.Error != nil && result.Error.Message != "" {
		return nil, fmt.Errorf("embedding API error: %s", result.Error.Message)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("embedding API returned no vectors")
	}

	vectors := make([]pgvector.Vector, len(result.Data))
	for _, d := range result.Data {
		// Guard the storage contract explicitly: a silently wrong width fails
		// later at INSERT with an opaque pgvector error.
		if len(d.Embedding) != EmbeddingDims {
			return nil, fmt.Errorf(
				"model %s returned %d-dim vectors, need %d — pick a model that supports %d dimensions",
				c.model, len(d.Embedding), EmbeddingDims, EmbeddingDims,
			)
		}
		if d.Index < 0 || d.Index >= len(vectors) {
			return nil, fmt.Errorf("embedding API returned out-of-range index %d", d.Index)
		}
		vectors[d.Index] = pgvector.NewVector(d.Embedding)
	}
	return vectors, nil
}

// EmbedText embeds a single text.
func (c *CompatClient) EmbedText(ctx context.Context, text string) (pgvector.Vector, error) {
	vectors, err := c.EmbedTexts(ctx, []string{text})
	if err != nil {
		return pgvector.Vector{}, err
	}
	if len(vectors) == 0 {
		return pgvector.Vector{}, fmt.Errorf("no embedding returned")
	}
	return vectors[0], nil
}
