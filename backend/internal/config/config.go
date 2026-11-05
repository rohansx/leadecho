package config

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sethvargo/go-envconfig"
)

// devJWTSecret is the insecure default used only for local development. It must
// keep in sync with the JWTSecret env default below.
const devJWTSecret = "leadecho-dev-secret-change-in-prod"

// Validate enforces production secret hygiene. In any non-development
// environment it refuses to boot when the JWT secret is missing/default, when
// the at-rest encryption key is unset, or when the two secrets are reused —
// since one leaked secret would otherwise forge sessions AND decrypt every
// stored BYOK key / session cookie.
func (c *Config) Validate() error {
	if c.Environment == "development" {
		return nil
	}
	if c.JWTSecret == "" || c.JWTSecret == devJWTSecret {
		return errors.New("JWT_SECRET must be set to a strong, non-default value outside development")
	}
	if c.EncryptionKey == "" {
		return errors.New("ENCRYPTION_KEY must be set outside development (do not reuse JWT_SECRET)")
	}
	if c.EncryptionKey == c.JWTSecret {
		return errors.New("ENCRYPTION_KEY must differ from JWT_SECRET")
	}
	return nil
}

type Config struct {
	Port        int    `env:"PORT,default=8090"`
	Environment string `env:"ENVIRONMENT,default=development"`
	LogLevel    string `env:"LOG_LEVEL,default=info"`

	DatabaseURL string `env:"DATABASE_URL,required"`
	RedisURL    string `env:"REDIS_URL,required"`

	// OAuth sign-in. Each provider is enabled when its client ID + secret are
	// set. Redirect URLs default to FRONTEND_URL + /api/v1/auth/<provider>/callback.
	GoogleClientID     string `env:"GOOGLE_CLIENT_ID,default="`
	GoogleClientSecret string `env:"GOOGLE_CLIENT_SECRET,default="`
	GoogleRedirectURL  string `env:"GOOGLE_REDIRECT_URL,default="`
	GitHubClientID     string `env:"GITHUB_CLIENT_ID,default="`
	GitHubClientSecret string `env:"GITHUB_CLIENT_SECRET,default="`
	GitHubRedirectURL  string `env:"GITHUB_REDIRECT_URL,default="`

	// TrustedProxyHops is how many reverse proxies we control sit in front of
	// the API (1 behind Traefik/nginx). 0 = never trust X-Forwarded-For.
	TrustedProxyHops int `env:"TRUSTED_PROXY_HOPS,default=0"`

	// JWT
	JWTSecret string `env:"JWT_SECRET,default=leadecho-dev-secret-change-in-prod"`

	// Encryption key for BYOK API keys (defaults to JWT secret if unset)
	EncryptionKey string `env:"ENCRYPTION_KEY,default="`

	// Frontend URL (for OAuth redirects)
	FrontendURL string `env:"FRONTEND_URL,default=http://localhost:3100"`

	// System-level AI keys (fallback when no BYOK key is set)
	// Priority at startup: NVIDIA → DeepSeek → GLM → OpenAI → Ollama
	NVIDIAAPIKey   string `env:"NVIDIA_API_KEY,default="`
	NVIDIAModel    string `env:"NVIDIA_MODEL,default=nvidia/llama-3.3-nemotron-super-49b-v1"`
	DeepSeekAPIKey string `env:"DEEPSEEK_API_KEY,default="`
	GLMAPIKey      string `env:"GLM_API_KEY,default="`
	OpenAIAPIKey   string `env:"OPENAI_API_KEY,default="`

	// Anthropic Claude (native Messages API)
	AnthropicAPIKey string `env:"ANTHROPIC_API_KEY,default="`

	// OpenRouter (OpenAI-compatible gateway; model ids like "anthropic/claude-...")
	OpenRouterAPIKey string `env:"OPENROUTER_API_KEY,default="`
	OpenRouterModel  string `env:"OPENROUTER_MODEL,default="`

	// Ollama Cloud (OpenAI-compatible; https://ollama.com/v1)
	OllamaAPIKey string `env:"OLLAMA_API_KEY,default="`
	OllamaModel  string `env:"OLLAMA_MODEL,default=gpt-oss:120b"`

	// Voyage AI (embeddings)
	VoyageAPIKey string `env:"VOYAGE_API_KEY,default="`

	// Exa (web-wide semantic discovery source for the monitor)
	ExaAPIKey string `env:"EXA_API_KEY,default="`

	// GoogleNewsEnabled opts in to the Google News RSS source. Default false:
	// Google's feed terms restrict it to personal, non-commercial use.
	GoogleNewsEnabled bool `env:"GOOGLE_NEWS_ENABLED,default=false"`

	// treg feature: hosted data tools (X/LinkedIn search). Empty token keeps
	// the Pinchtab/Camoufox/Scrapling paths. See docs/treg-integration.md.
	TregToken        string `env:"TREG_TOKEN,default="`
	TregOrgID        string `env:"TREG_ORG_ID,default="`
	TregBaseURL      string `env:"TREG_BASE_URL,default=https://treg.to"`
	TregMaxCostMicro int64  `env:"TREG_MAX_COST_MICRO,default=20000"`
	// enrich: treg lead enrichment (qualified leads only) + tracked profiles.
	EnrichDailyCap            int   `env:"ENRICH_DAILY_CAP,default=25"`
	TregCustomerDailyCapMicro int64 `env:"TREG_CUSTOMER_DAILY_CAP_MICRO,default=0"`
	TrackedProfilesPollHours  int   `env:"TRACKED_PROFILES_POLL_HOURS,default=6"`
	// RedditCommentsEnabled searches subreddit comments (free public JSON).
	RedditCommentsEnabled bool `env:"REDDIT_COMMENTS_ENABLED,default=true"`
	// sources: optional Bluesky session for authenticated search. Unset =
	// anonymous search via the public AppView (still works).
	BlueskyHandle      string `env:"BLUESKY_HANDLE,default="`
	BlueskyAppPassword string `env:"BLUESKY_APP_PASSWORD,default="`

	// ExtensionDistDir holds packaged Chrome extension builds, served from
	// Settings so self-hosters can install it without a Web Store listing.
	ExtensionDistDir string `env:"EXTENSION_DIST_DIR,default=../extension/.output"`

	// Resend (email notifications)
	ResendAPIKey string `env:"RESEND_API_KEY,default="`

	// Digests (P1 #13): daily/weekly digest scheduler, runs wherever the monitor runs.
	DigestsEnabled  bool   `env:"DIGESTS_ENABLED,default=true"`
	DigestFromEmail string `env:"DIGEST_FROM_EMAIL,default=LeadEcho <hello@leadecho.io>"`

	// automations (P2 #17): scheduler for user-enabled automations, runs wherever the monitor runs.
	AutomationsEnabled bool `env:"AUTOMATIONS_ENABLED,default=true"`

	// Pinchtab (browser automation sidecar for Reddit + Twitter)
	PinchtabURL   string `env:"PINCHTAB_URL,default=http://localhost:9867"`
	PinchtabToken string `env:"PINCHTAB_TOKEN,default="`

	// Camoufox (Pro-tier stealth Firefox sidecar for LinkedIn)
	CamoufoxURL   string `env:"CAMOUFOX_URL,default="`
	CamoufoxToken string `env:"CAMOUFOX_TOKEN,default="`

	// Scrapling (stealth fallback sidecar — used when Pinchtab/Camoufox unavailable)
	ScraplingURL   string `env:"SCRAPLING_URL,default="`
	ScraplingToken string `env:"SCRAPLING_TOKEN,default="`

	// Redis Streams backbone
	StreamsEnabled                     bool   `env:"STREAMS_ENABLED,default=false"`
	StreamsDualWriteEnabled            bool   `env:"STREAMS_DUAL_WRITE_ENABLED,default=false"`
	StreamsScorerConsumerEnabled       bool   `env:"STREAMS_SCORER_CONSUMER_ENABLED,default=false"`
	StreamsNotifierConsumerEnabled     bool   `env:"STREAMS_NOTIFIER_CONSUMER_ENABLED,default=false"`
	StreamsQualifierConsumerEnabled    bool   `env:"STREAMS_QUALIFIER_CONSUMER_ENABLED,default=false"`
	StreamsReplyDrafterConsumerEnabled bool   `env:"STREAMS_REPLY_DRAFTER_CONSUMER_ENABLED,default=false"`
	StreamsWorkflowConsumerEnabled     bool   `env:"STREAMS_WORKFLOW_CONSUMER_ENABLED,default=false"`
	StreamsRetryConsumerEnabled        bool   `env:"STREAMS_RETRY_CONSUMER_ENABLED,default=false"`
	StreamsInlineFallbackEnabled       bool   `env:"STREAMS_INLINE_FALLBACK_ENABLED,default=true"`
	StreamsConsumerName                string `env:"STREAMS_CONSUMER_NAME,default=api-main"`
	StreamsBatchSize                   int64  `env:"STREAMS_BATCH_SIZE,default=50"`
	StreamsBlockMS                     int64  `env:"STREAMS_BLOCK_MS,default=5000"`
	StreamsClaimIdleMS                 int64  `env:"STREAMS_CLAIM_IDLE_MS,default=120000"`
	StreamsMaxAttempts                 int64  `env:"STREAMS_MAX_ATTEMPTS,default=8"`
	StreamsWorkersInAPI                bool   `env:"STREAMS_WORKERS_IN_API,default=true"`

	// Process role: api (HTTP + optional workers) or worker (monitor + stream consumers).
	ProcessRole string `env:"PROCESS_ROLE,default=api"`

	// Monitor runs in API by default; disabled automatically when PROCESS_ROLE=worker
	// unless explicitly set via MONITOR_ENABLED.
	MonitorEnabled bool `env:"MONITOR_ENABLED,default=true"`

	MetricsEnabled            bool `env:"METRICS_ENABLED,default=false"`
	MetricsPort               int  `env:"METRICS_PORT,default=9090"`
	MetricsCollectIntervalSec int  `env:"METRICS_COLLECT_INTERVAL_SEC,default=30"`
	WorkerHealthPort          int  `env:"WORKER_HEALTH_PORT,default=8091"`

	// mcp: workspace API keys (MCP server + public REST). 0 disables the per-key limit.
	APIKeyRateLimitPerMin int    `env:"API_KEY_RATE_LIMIT_PER_MIN,default=120"`
	AppVersion            string `env:"APP_VERSION,default=dev"` // reported by /api/public/v1/probe and MCP serverInfo
}

func (c *Config) IsWorker() bool {
	return c.ProcessRole == "worker"
}

func (c *Config) ShouldRunMonitor() bool {
	if c.IsWorker() {
		return true
	}
	return c.MonitorEnabled
}

func (c *Config) ShouldRunStreamWorkers() bool {
	if !c.StreamsEnabled {
		return false
	}
	if c.IsWorker() {
		return true
	}
	return c.StreamsWorkersInAPI
}

func (c *Config) MetricsCollectInterval() time.Duration {
	if c.MetricsCollectIntervalSec <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.MetricsCollectIntervalSec) * time.Second
}

// EncryptionKeyOrDefault returns the encryption key, falling back to JWT secret.
// RequireEmailVerification is true when an email provider is configured.
// Without one, password accounts are auto-verified (self-host convenience).
func (c *Config) RequireEmailVerification() bool { return c.ResendAPIKey != "" }

// SecureCookies is true outside local development (cookies sent over HTTPS only).
func (c *Config) SecureCookies() bool { return c.Environment != "development" }

func (c *Config) EncryptionKeyOrDefault() string {
	if c.EncryptionKey != "" {
		return c.EncryptionKey
	}
	return c.JWTSecret
}

func Load(ctx context.Context) (*Config, error) {
	var cfg Config
	if err := envconfig.Process(ctx, &cfg); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	return &cfg, nil
}
