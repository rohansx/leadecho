package crmsync

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"leadecho/internal/database"
)

// ---- Signed webhook -------------------------------------------------------

// Sign returns the hex HMAC-SHA256 of "<timestamp>.<body>". Receivers verify by
// recomputing it and comparing X-LeadEcho-Signature: sha256=<hex>, and should
// reject timestamps older than a few minutes to stop replays.
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateWebhookURL requires an absolute https URL. Internal addresses are
// blocked at dial time by netsafe.
func ValidateWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("invalid webhook URL")
	}
	if u.Scheme != "https" {
		return errors.New("webhook URL must be https")
	}
	return nil
}

func (d *Dispatcher) sendWebhook(ctx context.Context, cfg Config, ev Event, lead LeadPayload) error {
	body, err := json.Marshal(map[string]any{"event": ev, "lead": lead, "sent_at": time.Now().UTC()})
	if err != nil {
		return err
	}
	return PostSigned(ctx, d.webhook, cfg.WebhookURL, cfg.WebhookSecret, ev.Type, body)
}

// PostSigned sends one signed webhook. Exported for the dashboard "send test" button.
func PostSigned(ctx context.Context, client *http.Client, target, secret, eventType string, body []byte) error {
	if err := ValidateWebhookURL(target); err != nil {
		return err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LeadEcho-Webhooks/1")
	req.Header.Set("X-LeadEcho-Event", eventType)
	req.Header.Set("X-LeadEcho-Timestamp", ts)
	if secret != "" {
		req.Header.Set("X-LeadEcho-Signature", "sha256="+Sign(secret, ts, body))
	}
	return do(client, req, nil)
}

// ---- HubSpot --------------------------------------------------------------

// hubspotLifecycle maps lead stages to HubSpot's default lifecyclestage values.
// HubSpot rejects backwards moves; those errors are logged and ignored.
var hubspotLifecycle = map[string]string{
	"prospect":  "lead",
	"qualified": "salesqualifiedlead",
	"engaged":   "opportunity",
	"converted": "customer",
}

const hubspotIDKey = "hubspot_contact_id"

// HubSpot 409 body contains "Contact already exists. Existing ID: 12345".
var hubspotExistingID = regexp.MustCompile(`Existing ID: (\d+)`)

func (d *Dispatcher) syncHubSpot(ctx context.Context, wsID, token string, ev Event, lead database.Lead, p LeadPayload) error {
	props := map[string]string{}
	if stage, ok := hubspotLifecycle[p.Stage]; ok {
		props["lifecyclestage"] = stage
	}

	id := leadMetaString(lead.Metadata, hubspotIDKey)
	if id == "" {
		first, last := splitName(p.displayName())
		props["firstname"] = first
		if last != "" {
			props["lastname"] = last
		}
		if p.ContactEmail != "" {
			props["email"] = p.ContactEmail
		}
		if p.Company != "" {
			props["company"] = p.Company
		}
		if p.ProfileURL != "" {
			props["website"] = p.ProfileURL
		}
		hubspotEnriched(props, p, false) // enrich
		var out struct {
			ID string `json:"id"`
		}
		err := d.vendorJSON(ctx, http.MethodPost, d.hubspot+"/crm/v3/objects/contacts", token, map[string]any{"properties": props}, &out)
		var se *StatusError
		if errors.As(err, &se) && se.Code == http.StatusConflict {
			// Email already exists in HubSpot: adopt that contact instead of failing forever.
			if m := hubspotExistingID.FindStringSubmatch(se.Body); m != nil {
				out.ID, err = m[1], nil
			}
		}
		if err != nil {
			return fmt.Errorf("create contact: %w", err)
		}
		if out.ID == "" {
			return errors.New("create contact: empty id")
		}
		return d.saveLeadMeta(ctx, wsID, lead.ID, hubspotIDKey, out.ID)
	}

	if ev.Type == EventLeadEnriched { // enrich: fill the existing contact
		hubspotEnriched(props, p, true)
	}
	if len(props) == 0 {
		return nil // e.g. moved to "lost": nothing to map
	}
	return d.vendorJSON(ctx, http.MethodPatch, d.hubspot+"/crm/v3/objects/contacts/"+url.PathEscape(id), token, map[string]any{"properties": props}, nil)
}

// ---- Attio ----------------------------------------------------------------

const attioIDKey = "attio_record_id"

func (d *Dispatcher) syncAttio(ctx context.Context, wsID, token string, ev Event, lead database.Lead, p LeadPayload) error {
	id := leadMetaString(lead.Metadata, attioIDKey)
	if id == "" {
		// Attio's personal-name string form is "Last, First"; the object form is unambiguous.
		full := p.displayName()
		first, last := splitName(full)
		values := map[string]any{"name": map[string]string{"first_name": first, "last_name": last, "full_name": full}}
		if p.ContactEmail != "" {
			values["email_addresses"] = []string{p.ContactEmail}
		}
		if p.JobTitle != "" { // enrich
			values["job_title"] = p.JobTitle
		}
		var out struct {
			Data struct {
				ID struct {
					RecordID string `json:"record_id"`
				} `json:"id"`
			} `json:"data"`
		}
		if err := d.vendorJSON(ctx, http.MethodPost, d.attio+"/v2/objects/people/records", token, map[string]any{"data": map[string]any{"values": values}}, &out); err != nil {
			return fmt.Errorf("create person: %w", err)
		}
		id = out.Data.ID.RecordID
		if id == "" {
			return errors.New("create person: empty record_id")
		}
		if err := d.saveLeadMeta(ctx, wsID, lead.ID, attioIDKey, id); err != nil {
			return fmt.Errorf("save attio id: %w", err)
		}
	} else if v := attioEnriched(p); ev.Type == EventLeadEnriched && len(v) > 0 { // enrich
		if err := d.vendorJSON(ctx, http.MethodPatch, d.attio+"/v2/objects/people/records/"+url.PathEscape(id), token, map[string]any{"data": map[string]any{"values": v}}, nil); err != nil {
			return fmt.Errorf("update person: %w", err)
		}
	}

	// Attio people have no default stage attribute, so lifecycle changes land as notes.
	note := map[string]any{"data": map[string]any{
		"parent_object":    "people",
		"parent_record_id": id,
		"title":            "LeadEcho: " + ev.Type,
		"format":           "plaintext",
		"content":          noteContent(ev, p),
	}}
	return d.vendorJSON(ctx, http.MethodPost, d.attio+"/v2/notes", token, note, nil)
}

func noteContent(ev Event, p LeadPayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Stage: %s", p.Stage)
	if ev.PrevStage != "" {
		fmt.Fprintf(&b, " (was %s)", ev.PrevStage)
	}
	if ev.ConvType != "" {
		fmt.Fprintf(&b, "\nConversion: %s", ev.ConvType)
		if ev.RevenueCents > 0 {
			fmt.Fprintf(&b, " · $%.2f", float64(ev.RevenueCents)/100)
		}
	}
	if p.Platform != "" {
		fmt.Fprintf(&b, "\nSource: %s", p.Platform)
	}
	if p.ProfileURL != "" {
		fmt.Fprintf(&b, "\nProfile: %s", p.ProfileURL)
	}
	enrichmentNote(&b, p) // enrich
	return b.String()
}

// ---- shared ---------------------------------------------------------------

func (d *Dispatcher) vendorJSON(ctx context.Context, method, target, token string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return do(d.api, req, out)
}

// StatusError is a non-2xx response. Body is for server logs only; never echo
// it to users (it would turn the webhook tester into a fetch-anything proxy).
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string { return fmt.Sprintf("status %d: %s", e.Code, e.Body) }

func do(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &StatusError{Code: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func splitName(full string) (string, string) {
	full = strings.TrimSpace(full)
	if i := strings.LastIndex(full, " "); i > 0 {
		return full[:i], full[i+1:]
	}
	return full, ""
}
