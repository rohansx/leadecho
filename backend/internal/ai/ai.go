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
	Name    string // "glm", "deepseek", or "openai"
	APIKey  string
	BaseURL string // e.g. "https://open.bigmodel.cn/api/paas/v4", "https://api.deepseek.com/v1", or "https://api.openai.com/v1"
	Model   string // e.g. "glm-4-flash", "deepseek-chat", or "gpt-4o-mini"
}

// ClassifyResult holds the output from intent classification.
type ClassifyResult struct {
	Intent                string  `json:"intent"`
	ConversionProbability float64 `json:"conversion_probability"`
	RelevanceScore        float64 `json:"relevance_score"`
	Reasoning             string  `json:"reasoning"`
	AwarenessLevel        string  `json:"awareness_level"`
}

// ProductAnalysis holds the output from analyzing a product URL.
type ProductAnalysis struct {
	ProductName         string     `json:"product_name"`
	Description         string     `json:"description"`
	Features            []string   `json:"features"`
	TargetAudience      flexString `json:"target_audience"`
	PainPoints          []string   `json:"pain_points"`
	Competitors         []string   `json:"competitors"`
	SuggestedKeywords   []string   `json:"suggested_keywords"`
	SuggestedSubreddits []string   `json:"suggested_subreddits"`
	SuggestedPlatforms  []string   `json:"suggested_platforms"`
}

// flexString accepts either a JSON string or a JSON array of strings (joined with ", "),
// so the parser tolerates the common LLM mistake of returning an array for a singular field.
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*f = flexString(strings.Join(arr, ", "))
		return nil
	}
	return fmt.Errorf("flexString: expected string or []string, got %s", string(data))
}

func (f flexString) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(f))
}

// PreFilterResult holds the output from the reply pre-filter.
type PreFilterResult struct {
	ShouldReply    bool   `json:"should_reply"`
	Reason         string `json:"reason"`
	AwarenessLevel string `json:"awareness_level"`
}

// FilterResult holds the output from the ingestion spam/relevance gate (Stage 1).
type FilterResult struct {
	Pass   bool   `json:"pass"`
	Reason string `json:"reason"`
}

// DraftReplyOptions configures the enhanced reply drafting.
type DraftReplyOptions struct {
	Title          string
	Content        string
	Platform       string
	Intent         string
	AwarenessLevel string
	ThreadContext  string
	KBContext      string
	ProductName    string
	TemplateStyle  string
}

// EnhancedDraftResult holds the output from enhanced reply drafting.
type EnhancedDraftResult struct {
	Reply         string `json:"reply"`
	Tone          string `json:"tone"`
	TemplateStyle string `json:"template_style"`
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

// prependNemotronDirective inserts "detailed_thinking off" so Nemotron answers
// directly instead of streaming its chain-of-thought first.
func prependNemotronDirective(messages []chatMessage) []chatMessage {
	const directive = "detailed_thinking off"
	if len(messages) > 0 && messages[0].Role == "system" {
		out := make([]chatMessage, len(messages))
		copy(out, messages)
		out[0] = chatMessage{Role: "system", Content: directive + "\n\n" + messages[0].Content}
		return out
	}
	return append([]chatMessage{{Role: "system", Content: directive}}, messages...)
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
	case "deepseek":
		return Provider{
			Name:    "deepseek",
			APIKey:  apiKey,
			BaseURL: "https://api.deepseek.com/v1",
			Model:   "deepseek-chat",
		}
	case "nvidia":
		return Provider{
			Name:    "nvidia",
			APIKey:  apiKey,
			BaseURL: "https://integrate.api.nvidia.com/v1",
			Model:   "nvidia/llama-3.3-nemotron-super-49b-v1",
		}
	case "ollama":
		// Ollama Cloud — OpenAI-compatible endpoint, Bearer API key.
		return Provider{
			Name:    "ollama",
			APIKey:  apiKey,
			BaseURL: "https://ollama.com/v1",
			Model:   "gpt-oss:120b",
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
	// NVIDIA Nemotron has reasoning enabled by default — disable it for structured-output
	// calls so latency matches a standard Llama-3.3-70B.
	if p.Name == "nvidia" && strings.Contains(p.Model, "nemotron") {
		messages = prependNemotronDirective(messages)
	}

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

	client := &http.Client{Timeout: 120 * time.Second}
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

// stripCodeFences removes markdown code fences and the JSON-invalid artifacts
// (// and /* */ comments, trailing commas) that some LLMs emit despite being
// told to return strict JSON — e.g. NVIDIA Nemotron writing `[], // none` which
// encoding/json rejects with "invalid character '/'".
func stripCodeFences(s string) string {
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return sanitizeJSON(strings.TrimSpace(s))
}

// sanitizeJSON strips JS-style comments and trailing commas that fall outside
// string literals; anything inside a string (e.g. "https://x") is left intact.
// ponytail: single-pass scanner, O(n); handles the JSON LLMs actually emit.
func sanitizeJSON(s string) string {
	buf := make([]byte, 0, len(s))
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			buf = append(buf, c)
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr = true
			buf = append(buf, c)
		case c == '/' && i+1 < len(s) && s[i+1] == '/': // line comment
			for i+1 < len(s) && s[i+1] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*': // block comment
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i++ // consume the closing '/'
		case c == '}' || c == ']': // drop any trailing comma before a closer
			for len(buf) > 0 {
				last := buf[len(buf)-1]
				if last == ' ' || last == '\t' || last == '\n' || last == '\r' {
					buf = buf[:len(buf)-1]
					continue
				}
				break
			}
			if len(buf) > 0 && buf[len(buf)-1] == ',' {
				buf = buf[:len(buf)-1]
			}
			buf = append(buf, c)
		default:
			buf = append(buf, c)
		}
	}
	return string(buf)
}

// ClassifyIntent classifies a social mention into intent categories.
func ClassifyIntent(ctx context.Context, p Provider, title, content, platform string) (*ClassifyResult, error) {
	systemPrompt := `You are an intent classifier for social media mentions. Analyze the post and classify it.

Return ONLY valid JSON (no markdown, no code fences) with these fields:
- "intent": one of "buy_signal", "complaint", "recommendation_ask", "comparison", "general"
- "conversion_probability": float 0.0-1.0 (how likely this person will buy/convert)
- "relevance_score": float 0.0-10.0 (how relevant this is for a B2B SaaS product)
- "reasoning": brief one-sentence explanation
- "awareness_level": one of "problem_aware", "solution_aware", "product_aware", "purchase_ready"

Intent definitions:
- buy_signal: User is actively looking to buy/switch to a product
- complaint: User is complaining about a competitor or existing solution
- recommendation_ask: User is asking for recommendations/suggestions
- comparison: User is comparing multiple products or solutions
- general: General discussion, not clearly actionable

Awareness level definitions:
- problem_aware: Knows they have a problem but hasn't started looking for solutions
- solution_aware: Actively researching solution categories
- product_aware: Comparing specific products by name
- purchase_ready: Ready to buy, mentions budget/timeline/specific requirements`

	userPrompt := fmt.Sprintf("Platform: %s\nTitle: %s\nContent: %s", platform, title, content)

	result, err := callChat(ctx, p, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, 0.1)
	if err != nil {
		return nil, err
	}

	result = stripCodeFences(result)

	var cr ClassifyResult
	if err := json.Unmarshal([]byte(result), &cr); err != nil {
		return nil, fmt.Errorf("parse classification result: %w (raw: %s)", err, result)
	}
	return &cr, nil
}

// AnalyzeProductPage extracts product info from scraped page text.
func AnalyzeProductPage(ctx context.Context, p Provider, pageText string) (*ProductAnalysis, error) {
	if len(pageText) > 8000 {
		pageText = pageText[:8000]
	}

	systemPrompt := `You are a product analyst. Given the text content of a product website, extract structured information.

Return ONLY valid JSON (no markdown, no code fences) with these fields:
- "product_name": the product name
- "description": one-sentence description of what the product does
- "features": array of 3-8 key features
- "target_audience": who this product is for (e.g. "SaaS founders", "enterprise DevOps teams")
- "pain_points": array of 5-10 pain points this product solves (phrased as problems users face, e.g. "too expensive monitoring tools", "hard to set up observability")
- "competitors": array of competitor names mentioned or implied
- "suggested_keywords": array of 5-15 monitoring keywords (product name, competitor names, pain-point phrases people would search for)
- "suggested_subreddits": array of 3-8 relevant subreddit names (without r/ prefix) where the target audience hangs out
- "suggested_platforms": array of platforms to monitor, from: "reddit", "hackernews", "twitter", "linkedin", "devto", "lobsters", "indiehackers"

Be thorough with pain_points — think about what problems potential customers would post about on Reddit/HN. Focus on real frustrations, not marketing language.`

	result, err := callChat(ctx, p, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "Website content:\n" + pageText},
	}, 0.3)
	if err != nil {
		return nil, err
	}

	result = stripCodeFences(result)

	var pa ProductAnalysis
	if err := json.Unmarshal([]byte(result), &pa); err != nil {
		return nil, fmt.Errorf("parse product analysis: %w (raw: %s)", err, result)
	}
	return &pa, nil
}

// FilterMention is Stage 1 of the scoring pipeline: cheap spam and noise rejection.
func FilterMention(ctx context.Context, p Provider, title, content, platform string) (*FilterResult, error) {
	systemPrompt := `You are a spam and noise filter for B2B social listening. Decide if a post should enter intent scoring.

Return ONLY valid JSON (no markdown, no code fences):
- "pass": boolean — true if the post might be a genuine discussion worth scoring for sales intent
- "reason": brief one-sentence explanation

Return pass: false for:
- Obvious spam, scams, crypto pumps, follower buying, affiliate spam
- Pure self-promotion with no question or pain point
- Bot-generated or nonsensical content
- Off-topic content with no business relevance
- Duplicate/template promo posts

Return pass: true for:
- Questions, complaints, comparisons, buying signals, or genuine discussions
- When uncertain, prefer pass: true (downstream stages will filter further)`

	userPrompt := fmt.Sprintf("Platform: %s\nTitle: %s\nContent: %s", platform, title, content)

	result, err := callChat(ctx, p, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, 0.0)
	if err != nil {
		return nil, err
	}
	result = stripCodeFences(result)

	var fr FilterResult
	if err := json.Unmarshal([]byte(result), &fr); err != nil {
		return nil, fmt.Errorf("parse filter result: %w (raw: %s)", err, result)
	}
	return &fr, nil
}

// PreFilterForReply determines if a mention is worth replying to.
func PreFilterForReply(ctx context.Context, p Provider, title, content, platform, intent string) (*PreFilterResult, error) {
	systemPrompt := `You evaluate whether a social media post is worth replying to for marketing purposes.

Return ONLY valid JSON (no markdown, no code fences) with:
- "should_reply": boolean — true if a helpful reply would add value
- "reason": brief explanation
- "awareness_level": one of "problem_aware", "solution_aware", "product_aware", "purchase_ready"

Return should_reply: false for:
- News articles or link shares with no discussion
- Already answered questions with accepted/highly-upvoted solutions
- Announcements or press releases
- Self-promotional posts (someone showing off their product)
- Posts with no question or pain point expressed
- Very old discussions unlikely to get engagement

Return should_reply: true for:
- Questions asking for help or recommendations
- Complaints about existing solutions
- Comparisons or "which tool should I use" discussions
- Pain point expressions where advice would be welcome`

	userPrompt := fmt.Sprintf("Platform: %s\nIntent: %s\nTitle: %s\nContent: %s", platform, intent, title, content)

	result, err := callChat(ctx, p, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, 0.1)
	if err != nil {
		return nil, err
	}

	result = stripCodeFences(result)

	var pf PreFilterResult
	if err := json.Unmarshal([]byte(result), &pf); err != nil {
		return nil, fmt.Errorf("parse pre-filter result: %w (raw: %s)", err, result)
	}
	return &pf, nil
}

// templatePrompts maps template styles to system prompt variations.
var templatePrompts = map[string]string{
	"value_first": `Draft a reply that directly answers the question or addresses the problem first.
Provide real, actionable value. Only after giving a genuinely helpful answer, you may briefly mention the product as one option among others.
Structure: helpful answer → brief credibility → soft product mention as "one option worth checking out".`,

	"storytelling": `Draft a reply as a personal story. Share a relatable experience with the same problem.
Describe what you tried, what didn't work, and what eventually helped (including the product naturally).
Structure: "I ran into the same issue when..." → what failed → what worked → casual product mention.`,

	"technical_deep_dive": `Draft a technically detailed reply that demonstrates deep knowledge of the problem domain.
Provide specific technical context, explain trade-offs between approaches, and position the product as one viable solution among several.
Structure: technical analysis → comparison of approaches → product as one option with specific strengths.`,

	"casual_helpful": `Draft a brief, genuinely helpful reply. Do NOT mention any product at all.
Just be helpful. Build goodwill and karma. Keep it short and friendly.
Structure: quick helpful answer → maybe a follow-up tip.`,

	"contrarian": `Draft a reply that offers a fresh or alternative perspective that others haven't mentioned.
Challenge conventional wisdom constructively, then present the product's approach as an interesting alternative.
Structure: "Actually, I'd push back on..." → alternative viewpoint → product as example of different approach.`,
}

// DraftReplyEnhanced generates a reply using template-based prompts and thread context.
func DraftReplyEnhanced(ctx context.Context, p Provider, opts DraftReplyOptions) (*EnhancedDraftResult, error) {
	templateGuide := templatePrompts[opts.TemplateStyle]
	if templateGuide == "" {
		templateGuide = templatePrompts["value_first"]
		opts.TemplateStyle = "value_first"
	}

	systemPrompt := fmt.Sprintf(`You are a helpful community member drafting a reply to a social media post.

STYLE: %s

RULES (apply to ALL styles):
- Sound like a genuine community member, NOT a salesperson or AI
- NEVER include direct links to products — mention by name only, let people Google it
- NEVER start with "As someone who..." or "I've been using..." (these are pattern-detected as bots)
- Match the platform tone (Reddit: casual/technical, HN: technical/insightful, Twitter: concise, LinkedIn: professional)
- Keep it concise (2-4 sentences for Reddit/HN, 1-2 for Twitter)
- If thread context is provided, read it and don't repeat what others already said
- Vary your opening — don't use the same first words across replies
- For problem_aware users: focus on the problem, minimal product mention
- For purchase_ready users: be more direct about product capabilities

Return ONLY valid JSON (no markdown, no code fences) with:
- "reply": the drafted reply text
- "tone": one of "helpful", "empathetic", "technical", "casual"
- "template_style": "%s"`, templateGuide, opts.TemplateStyle)

	userMsg := fmt.Sprintf("Platform: %s\nIntent: %s\nAwareness: %s\nTitle: %s\nContent: %s",
		opts.Platform, opts.Intent, opts.AwarenessLevel, opts.Title, opts.Content)
	if opts.ThreadContext != "" {
		userMsg += fmt.Sprintf("\n\nThread context (other comments in this thread):\n%s", opts.ThreadContext)
	}
	if opts.KBContext != "" {
		userMsg += fmt.Sprintf("\n\nProduct knowledge (use to inform your reply):\n%s", opts.KBContext)
	}
	if opts.ProductName != "" {
		userMsg += fmt.Sprintf("\n\nProduct name: %s", opts.ProductName)
	}

	// Vary temperature per call for diversity
	temp := 0.7 + float64(time.Now().UnixNano()%3)*0.1 // 0.7, 0.8, or 0.9

	result, err := callChat(ctx, p, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg},
	}, temp)
	if err != nil {
		return nil, err
	}

	result = stripCodeFences(result)

	var dr EnhancedDraftResult
	if err := json.Unmarshal([]byte(result), &dr); err != nil {
		return nil, fmt.Errorf("parse enhanced draft result: %w (raw: %s)", err, result)
	}
	dr.TemplateStyle = opts.TemplateStyle
	return &dr, nil
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

	result = stripCodeFences(result)

	var dr DraftReplyResult
	if err := json.Unmarshal([]byte(result), &dr); err != nil {
		return nil, fmt.Errorf("parse draft reply result: %w (raw: %s)", err, result)
	}
	return &dr, nil
}
