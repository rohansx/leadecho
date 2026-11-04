package crmsync

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/netsafe"
)

const (
	EventLeadCreated      = "lead.created"
	EventLeadStageChanged = "lead.stage_changed"
	EventConversion       = "conversion.recorded"
)

// Event is one lead lifecycle change.
type Event struct {
	Type         string `json:"type"`
	LeadID       string `json:"lead_id"`
	PrevStage    string `json:"previous_stage,omitempty"`
	ConvType     string `json:"conversion_type,omitempty"` // signup | purchase
	RevenueCents int32  `json:"revenue_cents,omitempty"`
}

// LeadPayload is the lead snapshot sent with every event.
type LeadPayload struct {
	ID             string `json:"id"`
	Stage          string `json:"stage"`
	ContactName    string `json:"contact_name,omitempty"`
	ContactEmail   string `json:"contact_email,omitempty"`
	Company        string `json:"company,omitempty"`
	Username       string `json:"username,omitempty"`
	Platform       string `json:"platform,omitempty"`
	ProfileURL     string `json:"profile_url,omitempty"`
	EstimatedValue int32  `json:"estimated_value,omitempty"`
}

// Dispatcher fans events out to every destination a workspace has configured.
// Delivery is best-effort and async: failures are logged, never surfaced to the
// caller, so a down CRM can't break lead creation.
//
// ponytail: fire-and-forget goroutine, no retry queue. Ceiling: events are lost
// if a destination is down or the process restarts mid-send. Upgrade path: put
// these on the Redis Streams backbone with the existing retry manager.
type Dispatcher struct {
	q       *database.Queries
	encKey  []byte
	logger  zerolog.Logger
	webhook *http.Client // SSRF-guarded: user-supplied URL
	api     *http.Client // fixed vendor hosts
	hubspot string       // base URLs, overridable in tests
	attio   string
	locks   sync.Map // leadID -> *sync.Mutex
}

func NewDispatcher(q *database.Queries, encKey []byte, logger zerolog.Logger) *Dispatcher {
	return &Dispatcher{
		q:       q,
		encKey:  encKey,
		logger:  logger.With().Str("component", "crmsync").Logger(),
		webhook: netsafe.Client(10 * time.Second),
		api:     &http.Client{Timeout: 10 * time.Second},
		hubspot: "https://api.hubapi.com",
		attio:   "https://api.attio.com",
	}
}

// Emit delivers ev in the background. Safe to call on a nil Dispatcher.
func (d *Dispatcher) Emit(wsID string, ev Event) {
	if d == nil || ev.LeadID == "" {
		return
	}
	go func() {
		// Serialize per lead so lead.created and a quick follow-up event can't
		// both see "no CRM id yet" and create duplicate contacts.
		// ponytail: in-process lock only. API and worker are separate processes,
		// so a cross-process race is still possible (needs a human action within
		// ms of auto-qualification). Upgrade: pg advisory lock per lead.
		mu, _ := d.locks.LoadOrStore(ev.LeadID, &sync.Mutex{})
		mu.(*sync.Mutex).Lock()
		defer mu.(*sync.Mutex).Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d.deliver(ctx, wsID, ev)
	}()
}

func (d *Dispatcher) deliver(ctx context.Context, wsID string, ev Event) {
	log := d.logger.With().Str("workspace_id", wsID).Str("event", ev.Type).Str("lead_id", ev.LeadID).Logger()

	stored, _, err := LoadStored(ctx, d.q, wsID)
	if err != nil {
		log.Error().Err(err).Msg("load integration config")
		return
	}
	cfg, err := stored.Decrypt(d.encKey)
	if err != nil {
		log.Error().Err(err).Msg("decrypt integration config")
		return
	}
	if cfg.Empty() {
		return
	}

	lead, err := d.q.GetLead(ctx, database.GetLeadParams{ID: ev.LeadID, WorkspaceID: wsID})
	if err != nil {
		log.Error().Err(err).Msg("load lead")
		return
	}
	payload := toPayload(lead)

	if cfg.WebhookURL != "" {
		if err := d.sendWebhook(ctx, cfg, ev, payload); err != nil {
			log.Warn().Err(err).Msg("webhook delivery failed")
		}
	}
	if cfg.HubSpotToken != "" {
		if err := d.syncHubSpot(ctx, wsID, cfg.HubSpotToken, ev, lead, payload); err != nil {
			log.Warn().Err(err).Msg("hubspot sync failed")
		}
	}
	if cfg.AttioToken != "" {
		if err := d.syncAttio(ctx, wsID, cfg.AttioToken, ev, lead, payload); err != nil {
			log.Warn().Err(err).Msg("attio sync failed")
		}
	}
}

func toPayload(l database.Lead) LeadPayload {
	p := LeadPayload{
		ID:             l.ID,
		Stage:          string(l.Stage),
		ContactName:    l.ContactName.String,
		ContactEmail:   l.ContactEmail.String,
		Company:        l.Company.String,
		Username:       l.Username.String,
		ProfileURL:     l.ProfileUrl.String,
		EstimatedValue: l.EstimatedValue.Int32,
	}
	if l.Platform.Valid {
		p.Platform = string(l.Platform.PlatformType)
	}
	return p
}

// displayName picks the best human label for a lead.
func (p LeadPayload) displayName() string {
	for _, v := range []string{p.ContactName, p.Username, p.ContactEmail} {
		if v != "" {
			return v
		}
	}
	return "LeadEcho lead " + p.ID[:8]
}

// leadMetaString reads a string key from lead.metadata.
func leadMetaString(meta []byte, key string) string {
	m := map[string]any{}
	if len(meta) == 0 || json.Unmarshal(meta, &m) != nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func (d *Dispatcher) saveLeadMeta(ctx context.Context, wsID, leadID, key, val string) error {
	b, _ := json.Marshal(map[string]string{key: val})
	return d.q.MergeLeadMetadata(ctx, wsID, leadID, b)
}
