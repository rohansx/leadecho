package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"leadecho/internal/api/middleware"
	"leadecho/internal/crmsync"
	"leadecho/internal/crypto"
	"leadecho/internal/database"
	"leadecho/internal/netsafe"
)

// IntegrationsHandler manages outbound integrations: signed webhook, HubSpot, Attio.
type IntegrationsHandler struct {
	q      *database.Queries
	encKey []byte
}

func NewIntegrationsHandler(q *database.Queries, encKey []byte) *IntegrationsHandler {
	return &IntegrationsHandler{q: q, encKey: encKey}
}

type integrationsStatus struct {
	WebhookURL       string `json:"webhook_url"`
	WebhookSecretSet bool   `json:"webhook_secret_set"`
	HubSpotConnected bool   `json:"hubspot_connected"`
	AttioConnected   bool   `json:"attio_connected"`
}

func statusOf(sc crmsync.StoredConfig) integrationsStatus {
	return integrationsStatus{
		WebhookURL:       sc.WebhookURL,
		WebhookSecretSet: sc.WebhookSecretEnc != "",
		HubSpotConnected: sc.HubSpotTokenEnc != "",
		AttioConnected:   sc.AttioTokenEnc != "",
	}
}

// Get returns integration status. Secrets are never returned.
func (h *IntegrationsHandler) Get(w http.ResponseWriter, r *http.Request) {
	sc, _, err := crmsync.LoadStored(r.Context(), h.q, middleware.WorkspaceID(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load integrations")
		return
	}
	writeJSON(w, http.StatusOK, statusOf(sc))
}

// Save updates integrations. For each field: omitted = unchanged, "" = remove.
func (h *IntegrationsHandler) Save(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)

	var body struct {
		WebhookURL    *string `json:"webhook_url"`
		WebhookSecret *string `json:"webhook_secret"`
		HubSpotToken  *string `json:"hubspot_token"`
		AttioToken    *string `json:"attio_token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sc, _, err := crmsync.LoadStored(ctx, h.q, wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load integrations")
		return
	}

	if body.WebhookURL != nil {
		u := strings.TrimSpace(*body.WebhookURL)
		if u != "" {
			if err := crmsync.ValidateWebhookURL(u); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		sc.WebhookURL = u
		if u == "" {
			sc.WebhookSecretEnc = ""
		}
	}
	for _, f := range []struct {
		in  *string
		dst *string
	}{
		{body.WebhookSecret, &sc.WebhookSecretEnc},
		{body.HubSpotToken, &sc.HubSpotTokenEnc},
		{body.AttioToken, &sc.AttioTokenEnc},
	} {
		if f.in == nil {
			continue
		}
		v := strings.TrimSpace(*f.in)
		if v == "" {
			*f.dst = ""
			continue
		}
		enc, err := crypto.Encrypt(h.encKey, v)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt secret")
			return
		}
		*f.dst = enc
	}

	raw, err := json.Marshal(sc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save integrations")
		return
	}
	// Write only our key so concurrent saves of other settings aren't clobbered.
	if err := h.q.SetWorkspaceSettingsKey(ctx, wsID, crmsync.SettingsKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save integrations")
		return
	}
	writeJSON(w, http.StatusOK, statusOf(sc))
}

// TestWebhook sends a signed "test" event to the configured webhook URL.
func (h *IntegrationsHandler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sc, _, err := crmsync.LoadStored(ctx, h.q, middleware.WorkspaceID(ctx))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load integrations")
		return
	}
	cfg, err := sc.Decrypt(h.encKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read webhook secret")
		return
	}
	if cfg.WebhookURL == "" {
		writeError(w, http.StatusBadRequest, "no webhook URL configured")
		return
	}
	body, _ := json.Marshal(map[string]any{
		"event":   map[string]string{"type": "test"},
		"sent_at": time.Now().UTC(),
	})
	if err := crmsync.PostSigned(ctx, netsafe.Client(10*time.Second), cfg.WebhookURL, cfg.WebhookSecret, "test", body); err != nil {
		// Don't echo response bodies or dial errors: they'd make this a network probe.
		var se *crmsync.StatusError
		if errors.As(err, &se) {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("webhook endpoint returned HTTP %d", se.Code))
			return
		}
		writeError(w, http.StatusBadGateway, "could not reach webhook URL")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
