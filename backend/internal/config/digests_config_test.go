package config

import (
	"context"
	"testing"
)

// digests (P1 #13): defaults keep digests on and a usable From header.
func TestDigestConfigDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("REDIS_URL", "redis://x")
	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DigestsEnabled || cfg.DigestFromEmail != "LeadEcho <hello@leadecho.io>" {
		t.Fatalf("enabled=%v from=%q", cfg.DigestsEnabled, cfg.DigestFromEmail)
	}
}
