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

// Client calls the Voyage AI embedding API.
type Client struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

// New creates a Voyage AI embedding client using the voyage-3-lite model (1024-dim).
func New(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		model:   "voyage-3-lite",
		baseURL: "https://api.voyageai.com/v1",
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

type embedRequest struct {
	Model     string   `json:"model"`
	Input     []string `json:"input"`
	InputType string   `json:"input_type"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// EmbedTexts generates embeddings for multiple texts in one API call (max 128).
func (c *Client) EmbedTexts(ctx context.Context, texts []string) ([]pgvector.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body := embedRequest{
		Model:     c.model,
		Input:     texts,
		InputType: "document",
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
		return nil, fmt.Errorf("voyage API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result embedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal embed response: %w", err)
	}

	vectors := make([]pgvector.Vector, len(result.Data))
	for _, d := range result.Data {
		vectors[d.Index] = pgvector.NewVector(d.Embedding)
	}
	return vectors, nil
}

// EmbedText generates an embedding for a single text.
func (c *Client) EmbedText(ctx context.Context, text string) (pgvector.Vector, error) {
	vectors, err := c.EmbedTexts(ctx, []string{text})
	if err != nil {
		return pgvector.Vector{}, err
	}
	if len(vectors) == 0 {
		return pgvector.Vector{}, fmt.Errorf("no embedding returned")
	}
	return vectors[0], nil
}
