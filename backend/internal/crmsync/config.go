// Package crmsync pushes lead lifecycle events to outbound destinations:
// a signed generic webhook, HubSpot, and Attio.
package crmsync

import (
	"context"
	"encoding/json"
	"fmt"

	"leadecho/internal/crypto"
	"leadecho/internal/database"
)

// SettingsKey is the workspace settings JSONB key holding integration config.
const SettingsKey = "integrations"

// StoredConfig is what lives in settings["integrations"]. Secrets are AES-encrypted.
type StoredConfig struct {
	WebhookURL       string `json:"webhook_url,omitempty"`
	WebhookSecretEnc string `json:"webhook_secret_enc,omitempty"`
	HubSpotTokenEnc  string `json:"hubspot_token_enc,omitempty"`
	AttioTokenEnc    string `json:"attio_token_enc,omitempty"`
}

// Config is the decrypted, ready-to-use form.
type Config struct {
	WebhookURL    string
	WebhookSecret string
	HubSpotToken  string
	AttioToken    string
}

func (c Config) Empty() bool {
	return c.WebhookURL == "" && c.HubSpotToken == "" && c.AttioToken == ""
}

// LoadStored reads the raw (still-encrypted) integration config for a workspace.
func LoadStored(ctx context.Context, q *database.Queries, wsID string) (StoredConfig, map[string]json.RawMessage, error) {
	raw, err := q.GetWorkspaceSettings(ctx, wsID)
	if err != nil {
		return StoredConfig{}, nil, fmt.Errorf("load settings: %w", err)
	}
	settings := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return StoredConfig{}, nil, fmt.Errorf("parse settings: %w", err)
		}
	}
	var sc StoredConfig
	if v, ok := settings[SettingsKey]; ok {
		if err := json.Unmarshal(v, &sc); err != nil {
			return StoredConfig{}, nil, fmt.Errorf("parse integrations: %w", err)
		}
	}
	return sc, settings, nil
}

// Decrypt turns stored config into usable secrets.
func (sc StoredConfig) Decrypt(key []byte) (Config, error) {
	c := Config{WebhookURL: sc.WebhookURL}
	for _, f := range []struct {
		enc  string
		dst  *string
		name string
	}{
		{sc.WebhookSecretEnc, &c.WebhookSecret, "webhook secret"},
		{sc.HubSpotTokenEnc, &c.HubSpotToken, "hubspot token"},
		{sc.AttioTokenEnc, &c.AttioToken, "attio token"},
	} {
		if f.enc == "" {
			continue
		}
		v, err := crypto.Decrypt(key, f.enc)
		if err != nil {
			return Config{}, fmt.Errorf("decrypt %s: %w", f.name, err)
		}
		*f.dst = v
	}
	return c, nil
}
