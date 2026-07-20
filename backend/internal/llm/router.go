package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	pgvector "github.com/pgvector/pgvector-go"
	"github.com/rs/zerolog"

	"leadecho/internal/ai"
	"leadecho/internal/crypto"
	"leadecho/internal/database"
	"leadecho/internal/embedding"
)

type Task string

const (
	TaskFilter         Task = "filter"
	TaskClassify       Task = "classify"
	TaskPreFilterReply Task = "pre_filter_reply"
	TaskDraftReply     Task = "draft_reply"
	TaskAnalyzeProduct Task = "analyze_product"
	TaskEmbedMentions  Task = "embed_mentions"
	TaskEmbedProfiles  Task = "embed_profiles"
	TaskEmbedDocuments Task = "embed_documents"
)

const (
	TierCheap     = "cheap"
	TierStrong    = "strong"
	TierEmbedding = "embedding"
)

type SystemKeys struct {
	NVIDIAAPIKey   string
	NVIDIAModel    string
	DeepSeekAPIKey string
	GLMAPIKey      string
	OpenAIAPIKey   string
	VoyageAPIKey   string
}

type ProviderSettings struct {
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

type ModelTarget struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url,omitempty"`
}

type Config struct {
	Providers map[string]ProviderSettings `json:"providers"`
	Models    map[string]ModelTarget      `json:"models"`
	Routing   map[string]string           `json:"routing"`
	Fallbacks map[string][]ModelTarget    `json:"fallbacks"`
}

type ProviderStatus struct {
	Provider          string   `json:"provider"`
	DisplayName       string   `json:"display_name"`
	Capabilities      []string `json:"capabilities"`
	DefaultModel      string   `json:"default_model,omitempty"`
	RecommendedModels []string `json:"recommended_models,omitempty"`
	IsSet             bool     `json:"is_set"`
	MaskedKey         string   `json:"masked_key,omitempty"`
	KeySource         string   `json:"key_source,omitempty"`
	BaseURL           string   `json:"base_url,omitempty"`
	Enabled           bool     `json:"enabled"`
}

type PublicConfig struct {
	Providers []ProviderStatus         `json:"providers"`
	Models    map[string]ModelTarget   `json:"models"`
	Routing   map[string]string        `json:"routing"`
	Fallbacks map[string][]ModelTarget `json:"fallbacks"`
	Health    HealthStatus             `json:"health"`
}

type HealthStatus struct {
	Ready    bool     `json:"ready"`
	ChatOK   bool     `json:"chat_ok"`
	StrongOK bool     `json:"strong_ok"`
	EmbedOK  bool     `json:"embed_ok"`
	Warnings []string `json:"warnings"`
}

type Router struct {
	q      *database.Queries
	encKey []byte
	system SystemKeys
	logger zerolog.Logger
}

func NewRouter(q *database.Queries, encKey []byte, system SystemKeys, logger zerolog.Logger) *Router {
	return &Router{q: q, encKey: encKey, system: system, logger: logger}
}

var providerRegistry = []ProviderStatus{
	{Provider: "nvidia", DisplayName: "NVIDIA Nemotron", Capabilities: []string{"chat"}, DefaultModel: "nvidia/llama-3.3-nemotron-super-49b-v1", RecommendedModels: []string{"nvidia/llama-3.3-nemotron-super-49b-v1", "nvidia/llama-3.1-nemotron-70b-instruct"}, Enabled: true},
	{Provider: "deepseek", DisplayName: "DeepSeek", Capabilities: []string{"chat"}, DefaultModel: "deepseek-chat", RecommendedModels: []string{"deepseek-chat", "deepseek-reasoner"}, Enabled: true},
	{Provider: "glm", DisplayName: "GLM / ZhipuAI", Capabilities: []string{"chat"}, DefaultModel: "glm-4.5-flash", RecommendedModels: []string{"glm-4.5-flash", "glm-4-plus", "glm-4-air"}, Enabled: true},
	{Provider: "openai", DisplayName: "OpenAI", Capabilities: []string{"chat"}, DefaultModel: "gpt-4o-mini", RecommendedModels: []string{"gpt-4o-mini", "gpt-4o", "gpt-4.1-mini", "gpt-4.1"}, Enabled: true},
	{Provider: "openrouter", DisplayName: "OpenRouter", Capabilities: []string{"chat"}, DefaultModel: "openai/gpt-4o-mini", RecommendedModels: []string{"openai/gpt-4o-mini", "openai/gpt-4o", "anthropic/claude-3.5-sonnet", "google/gemini-2.0-flash-001", "deepseek/deepseek-chat"}, Enabled: true},
	{Provider: "voyage", DisplayName: "Voyage AI", Capabilities: []string{"embedding"}, DefaultModel: "voyage-3", RecommendedModels: []string{"voyage-3"}, Enabled: true},
}

func DefaultConfig() Config {
	return Config{
		Providers: map[string]ProviderSettings{},
		Models: map[string]ModelTarget{
			TierCheap:     {Provider: "", Model: ""},
			TierStrong:    {Provider: "", Model: ""},
			TierEmbedding: {Provider: "voyage", Model: "voyage-3"},
		},
		Routing: map[string]string{
			string(TaskFilter):         TierCheap,
			string(TaskClassify):       TierCheap,
			string(TaskPreFilterReply): TierCheap,
			string(TaskDraftReply):     TierStrong,
			string(TaskAnalyzeProduct): TierStrong,
			string(TaskEmbedMentions):  TierEmbedding,
			string(TaskEmbedProfiles):  TierEmbedding,
			string(TaskEmbedDocuments): TierEmbedding,
		},
		Fallbacks: map[string][]ModelTarget{
			TierCheap:     {},
			TierStrong:    {},
			TierEmbedding: {},
		},
	}
}

func (r *Router) PublicConfig(ctx context.Context, workspaceID string) (PublicConfig, error) {
	cfg, legacy, err := r.loadConfig(ctx, workspaceID)
	if err != nil {
		return PublicConfig{}, err
	}
	statuses := make([]ProviderStatus, 0, len(providerRegistry))
	for _, meta := range providerRegistry {
		status := meta
		status.Enabled = true
		if ps, ok := cfg.Providers[meta.Provider]; ok {
			status.BaseURL = ps.BaseURL
			if ps.Enabled != nil {
				status.Enabled = *ps.Enabled
			}
		}
		if key, source := r.maskedKeyForProvider(meta.Provider, cfg, legacy); key != "" {
			status.IsSet = true
			status.MaskedKey = key
			status.KeySource = source
		}
		statuses = append(statuses, status)
	}
	return PublicConfig{Providers: statuses, Models: cfg.Models, Routing: cfg.Routing, Fallbacks: cfg.Fallbacks, Health: r.health(cfg, legacy)}, nil
}

func (r *Router) SaveConfig(ctx context.Context, workspaceID string, cfg Config) (PublicConfig, error) {
	current, err := r.loadSettings(ctx, workspaceID)
	if err != nil {
		return PublicConfig{}, err
	}
	existing, _, err := parseConfig(current)
	if err != nil {
		return PublicConfig{}, err
	}
	if cfg.Providers == nil {
		cfg.Providers = existing.Providers
	}
	if cfg.Models == nil {
		cfg.Models = existing.Models
	}
	if cfg.Routing == nil {
		cfg.Routing = existing.Routing
	}
	if cfg.Fallbacks == nil {
		cfg.Fallbacks = existing.Fallbacks
	}
	cfg = normalizeConfig(cfg)
	data, err := json.Marshal(cfg)
	if err != nil {
		return PublicConfig{}, err
	}
	current["llm"] = data
	if err := r.saveSettings(ctx, workspaceID, current); err != nil {
		return PublicConfig{}, err
	}
	return r.PublicConfig(ctx, workspaceID)
}

func (r *Router) SaveProviderKey(ctx context.Context, workspaceID, provider, apiKey, baseURL string) (ProviderStatus, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !knownProvider(provider) {
		return ProviderStatus{}, fmt.Errorf("unsupported provider")
	}
	settings, err := r.loadSettings(ctx, workspaceID)
	if err != nil {
		return ProviderStatus{}, err
	}
	cfg, _, _ := parseConfig(settings)
	cfg = normalizeConfig(cfg)
	encrypted := ""
	if strings.TrimSpace(apiKey) != "" {
		encrypted, err = crypto.Encrypt(r.encKey, strings.TrimSpace(apiKey))
		if err != nil {
			return ProviderStatus{}, err
		}
	}
	enabled := true
	cfg.Providers[provider] = ProviderSettings{APIKey: encrypted, BaseURL: strings.TrimSpace(baseURL), Enabled: &enabled}
	data, _ := json.Marshal(cfg)
	settings["llm"] = data
	if err := r.saveSettings(ctx, workspaceID, settings); err != nil {
		return ProviderStatus{}, err
	}
	pub, err := r.PublicConfig(ctx, workspaceID)
	if err != nil {
		return ProviderStatus{}, err
	}
	for _, s := range pub.Providers {
		if s.Provider == provider {
			return s, nil
		}
	}
	return ProviderStatus{}, fmt.Errorf("provider saved but not found")
}

func (r *Router) DeleteProviderKey(ctx context.Context, workspaceID, provider string) (ProviderStatus, error) {
	settings, err := r.loadSettings(ctx, workspaceID)
	if err != nil {
		return ProviderStatus{}, err
	}
	cfg, _, _ := parseConfig(settings)
	cfg = normalizeConfig(cfg)
	ps := cfg.Providers[provider]
	ps.APIKey = ""
	cfg.Providers[provider] = ps
	data, _ := json.Marshal(cfg)
	settings["llm"] = data
	if err := r.saveSettings(ctx, workspaceID, settings); err != nil {
		return ProviderStatus{}, err
	}
	pub, err := r.PublicConfig(ctx, workspaceID)
	if err != nil {
		return ProviderStatus{}, err
	}
	for _, s := range pub.Providers {
		if s.Provider == provider {
			return s, nil
		}
	}
	return ProviderStatus{Provider: provider}, nil
}

func (r *Router) ClassifyIntent(ctx context.Context, workspaceID, title, content, platform string) (*ai.ClassifyResult, error) {
	var out *ai.ClassifyResult
	err := r.withChatProvider(ctx, workspaceID, TaskClassify, func(p ai.Provider) error {
		res, err := ai.ClassifyIntent(ctx, p, title, content, platform)
		out = res
		return err
	})
	return out, err
}

func (r *Router) FilterMention(ctx context.Context, workspaceID, title, content, platform string) (*ai.FilterResult, error) {
	var out *ai.FilterResult
	err := r.withChatProvider(ctx, workspaceID, TaskFilter, func(p ai.Provider) error {
		res, err := ai.FilterMention(ctx, p, title, content, platform)
		out = res
		return err
	})
	return out, err
}

func (r *Router) PreFilterForReply(ctx context.Context, workspaceID, title, content, platform, intent string) (*ai.PreFilterResult, error) {
	var out *ai.PreFilterResult
	err := r.withChatProvider(ctx, workspaceID, TaskPreFilterReply, func(p ai.Provider) error {
		res, err := ai.PreFilterForReply(ctx, p, title, content, platform, intent)
		out = res
		return err
	})
	return out, err
}

func (r *Router) DraftReplyEnhanced(ctx context.Context, workspaceID string, opts ai.DraftReplyOptions) (*ai.EnhancedDraftResult, error) {
	var out *ai.EnhancedDraftResult
	err := r.withChatProvider(ctx, workspaceID, TaskDraftReply, func(p ai.Provider) error {
		res, err := ai.DraftReplyEnhanced(ctx, p, opts)
		out = res
		return err
	})
	return out, err
}

func (r *Router) AnalyzeProductPage(ctx context.Context, workspaceID, pageText string) (*ai.ProductAnalysis, error) {
	var out *ai.ProductAnalysis
	err := r.withChatProvider(ctx, workspaceID, TaskAnalyzeProduct, func(p ai.Provider) error {
		res, err := ai.AnalyzeProductPage(ctx, p, pageText)
		out = res
		return err
	})
	return out, err
}

func (r *Router) EmbedTexts(ctx context.Context, workspaceID string, task Task, texts []string) ([]pgvector.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	cfg, legacy, err := r.loadConfig(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	targets := r.targetsForTask(task, cfg, legacy)
	if len(targets) == 0 {
		return nil, errors.New("no embedding provider configured")
	}
	var lastErr error
	for idx, target := range targets {
		provider := strings.ToLower(target.Provider)
		if provider != "voyage" {
			lastErr = fmt.Errorf("provider %s does not support embeddings in this MVP", provider)
			continue
		}
		key, source, err := r.resolveKey(provider, cfg, legacy)
		if err != nil {
			lastErr = err
			continue
		}
		start := time.Now()
		client := embedding.New(key)
		vectors, err := client.EmbedTexts(ctx, texts)
		r.recordUsage(ctx, workspaceID, task, provider, defaultModel(target, provider), source, idx, time.Since(start), err)
		if err == nil {
			return vectors, nil
		}
		lastErr = err
		if !retryable(err) {
			break
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no embedding provider configured")
	}
	return nil, lastErr
}

func (r *Router) VerifyProvider(ctx context.Context, workspaceID, provider string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	cfg, legacy, err := r.loadConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	key, source, err := r.resolveKey(provider, cfg, legacy)
	if err != nil {
		return err
	}
	start := time.Now()
	if provider == "voyage" {
		_, err = embedding.New(key).EmbedTexts(ctx, []string{"verification ping"})
		r.recordUsage(ctx, workspaceID, TaskEmbedDocuments, provider, defaultModel(ModelTarget{}, provider), source, 0, time.Since(start), err)
		return err
	}
	p := providerForTarget(provider, key, ModelTarget{Provider: provider}, r.system)
	_, err = ai.ClassifyIntent(ctx, p, "Verification", "I am looking for a simple project management tool for my team because our current workflow is too slow.", "internal")
	r.recordUsage(ctx, workspaceID, TaskFilter, p.Name, p.Model, source, 0, time.Since(start), err)
	return err
}

func (r *Router) UsageSummary(ctx context.Context, workspaceID string) ([]database.LLMUsageSummaryRow, error) {
	return r.q.LLMUsageSummary(ctx, workspaceID)
}

func (r *Router) withChatProvider(ctx context.Context, workspaceID string, task Task, fn func(ai.Provider) error) error {
	cfg, legacy, err := r.loadConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	targets := r.targetsForTask(task, cfg, legacy)
	if len(targets) == 0 {
		return errors.New("no AI provider configured")
	}
	var lastErr error
	for idx, target := range targets {
		providerName := strings.ToLower(target.Provider)
		key, source, err := r.resolveKey(providerName, cfg, legacy)
		if err != nil {
			lastErr = err
			continue
		}
		provider := providerForTarget(providerName, key, target, r.system)
		start := time.Now()
		err = fn(provider)
		r.recordUsage(ctx, workspaceID, task, provider.Name, provider.Model, source, idx, time.Since(start), err)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable(err) {
			break
		}
	}
	return lastErr
}

func (r *Router) targetsForTask(task Task, cfg Config, legacy map[string]string) []ModelTarget {
	tier := cfg.Routing[string(task)]
	if tier == "" {
		if isEmbeddingTask(task) {
			tier = TierEmbedding
		} else {
			tier = TierCheap
		}
	}
	primary := cfg.Models[tier]
	if primary.Provider == "" {
		if isEmbeddingTask(task) {
			primary = ModelTarget{Provider: "voyage", Model: "voyage-3"}
		} else {
			primary = r.defaultChatTarget(cfg, legacy)
		}
	}
	if primary.Provider == "" {
		return nil
	}
	targets := []ModelTarget{withProviderDefaults(primary, cfg)}
	for _, fb := range cfg.Fallbacks[tier] {
		if fb.Provider != "" {
			targets = append(targets, withProviderDefaults(fb, cfg))
		}
	}
	return targets
}

func withProviderDefaults(target ModelTarget, cfg Config) ModelTarget {
	if target.BaseURL == "" {
		if ps, ok := cfg.Providers[strings.ToLower(target.Provider)]; ok {
			target.BaseURL = ps.BaseURL
		}
	}
	return target
}

func (r *Router) defaultChatTarget(cfg Config, legacy map[string]string) ModelTarget {
	for _, provider := range []string{"nvidia", "deepseek", "glm", "openai", "openrouter"} {
		if hasConfiguredKey(provider, cfg, legacy) || r.hasSystemKey(provider) {
			return ModelTarget{Provider: provider, Model: defaultModel(ModelTarget{}, provider)}
		}
	}
	return ModelTarget{}
}

func (r *Router) resolveKey(provider string, cfg Config, legacy map[string]string) (string, string, error) {
	if ps, ok := cfg.Providers[provider]; ok {
		if ps.Enabled != nil && !*ps.Enabled {
			return "", "", fmt.Errorf("provider %s is disabled", provider)
		}
		if ps.APIKey != "" {
			key, err := crypto.Decrypt(r.encKey, ps.APIKey)
			if err != nil {
				return "", "", fmt.Errorf("decrypt %s key: %w", provider, err)
			}
			return key, "workspace", nil
		}
	}
	if enc := legacy[provider]; enc != "" {
		key, err := crypto.Decrypt(r.encKey, enc)
		if err != nil {
			return "", "", fmt.Errorf("decrypt legacy %s key: %w", provider, err)
		}
		return key, "workspace_legacy", nil
	}
	if key := r.systemKey(provider); key != "" {
		return key, "env", nil
	}
	return "", "", fmt.Errorf("no key configured for %s", provider)
}

func (r *Router) recordUsage(ctx context.Context, workspaceID string, task Task, provider, model, source string, fallbackIndex int, latency time.Duration, callErr error) {
	status := "success"
	errText := pgtype.Text{}
	if callErr != nil {
		status = "error"
		errText = pgtype.Text{String: truncate(callErr.Error(), 500), Valid: true}
	}
	fallbackFrom := pgtype.Text{}
	if fallbackIndex > 0 {
		fallbackFrom = pgtype.Text{String: "primary", Valid: true}
	}
	if err := r.q.CreateLLMUsageEvent(ctx, database.CreateLLMUsageEventParams{
		WorkspaceID: workspaceID, Task: string(task), Provider: provider, Model: model, Status: status,
		EstimatedCostUSD: "0", LatencyMs: int32(latency.Milliseconds()), ErrorMessage: errText,
		FallbackFrom: fallbackFrom, KeySource: source,
	}); err != nil {
		r.logger.Warn().Err(err).Str("task", string(task)).Msg("llm: failed to record usage")
	}
}

func (r *Router) loadConfig(ctx context.Context, workspaceID string) (Config, map[string]string, error) {
	settings, err := r.loadSettings(ctx, workspaceID)
	if err != nil {
		return Config{}, nil, err
	}
	cfg, legacy, err := parseConfig(settings)
	return normalizeConfig(cfg), legacy, err
}

func (r *Router) loadSettings(ctx context.Context, workspaceID string) (map[string]json.RawMessage, error) {
	raw, err := r.q.GetWorkspaceSettings(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	settings := map[string]json.RawMessage{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &settings)
	}
	return settings, nil
}

func (r *Router) saveSettings(ctx context.Context, workspaceID string, settings map[string]json.RawMessage) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return r.q.UpdateWorkspaceSettings(ctx, database.UpdateWorkspaceSettingsParams{ID: workspaceID, Settings: raw})
}

func parseConfig(settings map[string]json.RawMessage) (Config, map[string]string, error) {
	cfg := DefaultConfig()
	if raw, ok := settings["llm"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return cfg, nil, err
		}
	}
	legacy := map[string]string{}
	if raw, ok := settings["api_keys"]; ok && len(raw) > 0 {
		_ = json.Unmarshal(raw, &legacy)
	}
	return cfg, legacy, nil
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderSettings{}
	}
	if cfg.Models == nil {
		cfg.Models = map[string]ModelTarget{}
	}
	if cfg.Routing == nil {
		cfg.Routing = map[string]string{}
	}
	if cfg.Fallbacks == nil {
		cfg.Fallbacks = map[string][]ModelTarget{}
	}
	for k, v := range def.Models {
		if _, ok := cfg.Models[k]; !ok {
			cfg.Models[k] = v
		}
	}
	for k, v := range def.Routing {
		if _, ok := cfg.Routing[k]; !ok {
			cfg.Routing[k] = v
		}
	}
	for k, v := range def.Fallbacks {
		if _, ok := cfg.Fallbacks[k]; !ok {
			cfg.Fallbacks[k] = v
		}
	}
	return cfg
}

func (r *Router) maskedKeyForProvider(provider string, cfg Config, legacy map[string]string) (string, string) {
	if ps, ok := cfg.Providers[provider]; ok && ps.APIKey != "" {
		if key, err := crypto.Decrypt(r.encKey, ps.APIKey); err == nil {
			return crypto.MaskKey(key), "workspace"
		}
	}
	if enc := legacy[provider]; enc != "" {
		if key, err := crypto.Decrypt(r.encKey, enc); err == nil {
			return crypto.MaskKey(key), "workspace_legacy"
		}
	}
	if key := r.systemKey(provider); key != "" {
		return crypto.MaskKey(key), "env"
	}
	return "", ""
}

func (r *Router) health(cfg Config, legacy map[string]string) HealthStatus {
	chat := false
	strong := false
	embed := false
	warnings := []string{}
	if t := r.defaultChatTarget(cfg, legacy); t.Provider != "" {
		chat = true
	}
	if target := cfg.Models[TierStrong]; target.Provider != "" && (hasConfiguredKey(target.Provider, cfg, legacy) || r.hasSystemKey(target.Provider)) {
		strong = true
	} else {
		strong = chat
	}
	if hasConfiguredKey("voyage", cfg, legacy) || r.hasSystemKey("voyage") {
		embed = true
	}
	if !chat {
		warnings = append(warnings, "No chat provider configured")
	}
	if !embed {
		warnings = append(warnings, "No embedding provider configured")
	}
	return HealthStatus{Ready: chat && embed, ChatOK: chat, StrongOK: strong, EmbedOK: embed, Warnings: warnings}
}

func providerForTarget(name, key string, target ModelTarget, sys SystemKeys) ai.Provider {
	p := ai.DefaultProvider(name, key)
	if name == "openrouter" {
		p = ai.Provider{Name: "openrouter", APIKey: key, BaseURL: "https://openrouter.ai/api/v1", Model: "openai/gpt-4o-mini"}
	}
	if name == "nvidia" && sys.NVIDIAModel != "" {
		p.Model = sys.NVIDIAModel
	}
	if target.Model != "" {
		p.Model = target.Model
	}
	if target.BaseURL != "" {
		p.BaseURL = target.BaseURL
	}
	return p
}

func defaultModel(target ModelTarget, provider string) string {
	if target.Model != "" {
		return target.Model
	}
	for _, meta := range providerRegistry {
		if meta.Provider == provider && meta.DefaultModel != "" {
			return meta.DefaultModel
		}
	}
	return "gpt-4o-mini"
}
func (r *Router) systemKey(provider string) string {
	switch provider {
	case "nvidia":
		return r.system.NVIDIAAPIKey
	case "deepseek":
		return r.system.DeepSeekAPIKey
	case "glm":
		return r.system.GLMAPIKey
	case "openai":
		return r.system.OpenAIAPIKey
	case "voyage":
		return r.system.VoyageAPIKey
	default:
		return ""
	}
}

func (r *Router) hasSystemKey(provider string) bool { return r.systemKey(provider) != "" }

func hasConfiguredKey(provider string, cfg Config, legacy map[string]string) bool {
	if ps, ok := cfg.Providers[provider]; ok {
		if ps.Enabled != nil && !*ps.Enabled {
			return false
		}
		if ps.APIKey != "" {
			return true
		}
	}
	return legacy[provider] != ""
}

func knownProvider(provider string) bool {
	for _, p := range providerRegistry {
		if p.Provider == provider {
			return true
		}
	}
	return false
}

func isEmbeddingTask(task Task) bool {
	return task == TaskEmbedMentions || task == TaskEmbedProfiles || task == TaskEmbedDocuments
}

func retryable(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "timeout") || strings.Contains(s, "429") || strings.Contains(s, "5") || strings.Contains(s, "temporar")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
