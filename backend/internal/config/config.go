package config

import (
	"context"
	"errors"
	"fmt"

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

	// Google OAuth
	GoogleClientID     string `env:"GOOGLE_CLIENT_ID,default="`
	GoogleClientSecret string `env:"GOOGLE_CLIENT_SECRET,default="`
	GoogleRedirectURL  string `env:"GOOGLE_REDIRECT_URL,default=http://localhost:8090/api/v1/auth/google/callback"`

	// JWT
	JWTSecret string `env:"JWT_SECRET,default=leadecho-dev-secret-change-in-prod"`

	// Encryption key for BYOK API keys (defaults to JWT secret if unset)
	EncryptionKey string `env:"ENCRYPTION_KEY,default="`

	// Frontend URL (for OAuth redirects)
	FrontendURL string `env:"FRONTEND_URL,default=http://localhost:3100"`

	// System-level AI keys (fallback when no BYOK key is set)
	GLMAPIKey      string `env:"GLM_API_KEY,default="`
	DeepSeekAPIKey string `env:"DEEPSEEK_API_KEY,default="`
	OpenAIAPIKey   string `env:"OPENAI_API_KEY,default="`

	// Voyage AI (embeddings)
	VoyageAPIKey string `env:"VOYAGE_API_KEY,default="`

	// Resend (email notifications)
	ResendAPIKey string `env:"RESEND_API_KEY,default="`

	// Pinchtab (browser automation sidecar for Reddit + Twitter)
	PinchtabURL   string `env:"PINCHTAB_URL,default=http://localhost:9867"`
	PinchtabToken string `env:"PINCHTAB_TOKEN,default="`

	// Camoufox (Pro-tier stealth Firefox sidecar for LinkedIn)
	CamoufoxURL   string `env:"CAMOUFOX_URL,default="`
	CamoufoxToken string `env:"CAMOUFOX_TOKEN,default="`

	// Scrapling (stealth fallback sidecar — used when Pinchtab/Camoufox unavailable)
	ScraplingURL   string `env:"SCRAPLING_URL,default="`
	ScraplingToken string `env:"SCRAPLING_TOKEN,default="`
}

// EncryptionKeyOrDefault returns the encryption key, falling back to JWT secret.
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
