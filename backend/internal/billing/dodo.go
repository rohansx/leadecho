package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Dodo API base URLs (from the official SDK).
const (
	DodoTestURL = "https://test.dodopayments.com"
	DodoLiveURL = "https://live.dodopayments.com"
)

type DodoConfig struct {
	APIKey    string
	BaseURL   string // DodoTestURL or DodoLiveURL
	Products  Products
	ReturnURL string // where checkout sends the customer back
}

// Dodo is the small slice of the Dodo Payments API LeadEcho uses:
// hosted checkout and the customer portal. nil when not configured.
type Dodo struct {
	cfg  DodoConfig
	http *http.Client
}

func NewDodo(cfg DodoConfig) *Dodo {
	if cfg.APIKey == "" {
		return nil
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DodoTestURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Dodo{cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}}
}

// CheckoutRequest: Item is "starter", "pro" or "topup".
type CheckoutRequest struct {
	Item        string
	WorkspaceID string
	CustomerID  string // existing Dodo customer, if any
	Email, Name string
}

// Checkout creates a hosted checkout session and returns its URL. The
// workspace id rides in metadata so the webhooks can credit the right workspace.
func (d *Dodo) Checkout(ctx context.Context, r CheckoutRequest) (string, error) {
	product := map[string]string{"starter": d.cfg.Products.Starter, "pro": d.cfg.Products.Pro, "topup": d.cfg.Products.Topup}[r.Item]
	if product == "" {
		return "", fmt.Errorf("dodo: no product configured for %q", r.Item)
	}
	customer := map[string]string{"email": r.Email, "name": r.Name}
	if r.CustomerID != "" {
		customer = map[string]string{"customer_id": r.CustomerID}
	}
	body := map[string]any{
		"product_cart": []map[string]any{{"product_id": product, "quantity": 1}},
		"customer":     customer,
		"metadata":     map[string]string{"workspace_id": r.WorkspaceID, "item": r.Item},
	}
	if d.cfg.ReturnURL != "" {
		body["return_url"] = d.cfg.ReturnURL
	}
	var out struct {
		CheckoutURL string `json:"checkout_url"`
	}
	if err := d.post(ctx, "/checkouts", body, &out); err != nil {
		return "", err
	}
	if out.CheckoutURL == "" {
		return "", fmt.Errorf("dodo: checkout returned no url")
	}
	return out.CheckoutURL, nil
}

// PortalLink returns a customer-portal session link (manage plan, cards, invoices).
func (d *Dodo) PortalLink(ctx context.Context, customerID string) (string, error) {
	var out struct {
		Link string `json:"link"`
	}
	if err := d.post(ctx, "/customers/"+url.PathEscape(customerID)+"/customer-portal/session", nil, &out); err != nil {
		return "", err
	}
	return out.Link, nil
}

func (d *Dodo) post(ctx context.Context, path string, in, out any) error {
	var rdr io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("dodo %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("dodo %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(raw[:min(len(raw), 300)])))
	}
	return json.Unmarshal(raw, out)
}
