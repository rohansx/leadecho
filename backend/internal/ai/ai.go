package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Provider configures which LLM provider to use.
type Provider struct {
	Name    string // "glm" or "openai"
	APIKey  string
	BaseURL string // e.g. "https://open.bigmodel.cn/api/paas/v4" or "https://api.openai.com/v1"
	Model   string // e.g. "glm-4-flash" or "gpt-4o-mini"
}

// ClassifyResult holds the output from intent classification.
type ClassifyResult struct {
	Intent                string  `json:"intent"`
	ConversionProbability float64 `json:"conversion_probability"`
	RelevanceScore        float64 `json:"relevance_score"`
	Reasoning             string  `json:"reasoning"`
}

// DraftReplyResult holds the output from reply drafting.
type DraftReplyResult struct {
	Reply string `json:"reply"`
	Tone  string `json:"tone"`
}

// chatMessage is an OpenAI-compatible message.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// DefaultProvider returns provider config based on provider name.
func DefaultProvider(name, apiKey string) Provider {
	switch name {
	case "glm":
		return Provider{
			Name:    "glm",
			APIKey:  apiKey,
			BaseURL: "https://open.bigmodel.cn/api/paas/v4",
			Model:   "glm-4.5-flash",
		}
	default:
		return Provider{
			Name:    "openai",
			APIKey:  apiKey,
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o-mini",
		}
	}
}

// callChat makes an OpenAI-compatible chat completion request.
func callChat(ctx context.Context, p Provider, messages []chatMessage, temp float64) (string, error) {
	body := chatRequest{
		Model:       p.Model,
		Messages:    messages,
		Temperature: temp,
		MaxTokens:   4096,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := p.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// ClassifyIntent classifies a social mention into intent categories.
func ClassifyIntent(ctx context.Context, p Provider, title, content, platform string) (*ClassifyResult, error) {
	systemPrompt := `You are an intent classifier for social media mentions. Analyze the post and classify it.

Return ONLY valid JSON (no markdown, no code fences) with these fields:
- "intent": one of "buy_signal", "complaint", "recommendation_ask", "comparison", "general"
- "conversion_probability": float 0.0-1.0 (how likely this person will buy/convert)
- "relevance_score": float 0.0-10.0 (how relevant this is for a B2B SaaS product)
- "reasoning": brief one-sentence explanation

Intent definitions:
- buy_signal: User is actively looking to buy/switch to a product
- complaint: User is complaining about a competitor or existing solution
- recommendation_ask: User is asking for recommendations/suggestions
- comparison: User is comparing multiple products or solutions
- general: General discussion, not clearly actionable`

	userPrompt := fmt.Sprintf("Platform: %s\nTitle: %s\nContent: %s", platform, title, content)

	result, err := callChat(ctx, p, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, 0.1)
	if err != nil {
		return nil, err
	}

	// Strip possible markdown code fences
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var cr ClassifyResult
	if err := json.Unmarshal([]byte(result), &cr); err != nil {
		return nil, fmt.Errorf("parse classification result: %w (raw: %s)", err, result)
	}
	return &cr, nil
}

// DraftReply generates a reply draft for a social mention using optional KB context.
func DraftReply(ctx context.Context, p Provider, title, content, platform, intent string, kbContext string) (*DraftReplyResult, error) {
	systemPrompt := `You are a helpful social media engagement specialist. Draft a natural, authentic reply to the social media post below.

Rules:
- Sound like a genuine community member, NOT a salesperson
- Be helpful first — provide real value before any subtle mention of your product
- Match the platform's tone (Reddit: casual/technical, HN: technical/insightful, Twitter: concise, LinkedIn: professional)
- Keep it concise (2-4 sentences for Reddit/HN, 1-2 for Twitter)
- If knowledge base context is provided, use it to make the reply more specific and helpful
- Never be pushy or overtly promotional

Return ONLY valid JSON (no markdown, no code fences) with:
- "reply": the drafted reply text
- "tone": one of "helpful", "empathetic", "technical", "casual"`

	userMsg := fmt.Sprintf("Platform: %s\nIntent: %s\nTitle: %s\nContent: %s", platform, intent, title, content)
	if kbContext != "" {
		userMsg += fmt.Sprintf("\n\nKnowledge Base Context (use this to inform your reply):\n%s", kbContext)
	}

	result, err := callChat(ctx, p, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg},
	}, 0.7)
	if err != nil {
		return nil, err
	}

	// Strip possible markdown code fences
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var dr DraftReplyResult
	if err := json.Unmarshal([]byte(result), &dr); err != nil {
		return nil, fmt.Errorf("parse draft reply result: %w (raw: %s)", err, result)
	}
	return &dr, nil
}
