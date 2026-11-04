package treg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// SetCustomerDailyBudget caps one workspace's treg spend per day
// (PUT /orgs/<org>/budgets/customer/<ws>). Admin token required. treg's caps
// are advisory (may overshoot by about one in-flight call).
func (c *Client) SetCustomerDailyBudget(ctx context.Context, workspaceID string, capMicro int64) error {
	if !validTag(workspaceID) {
		return fmt.Errorf("treg: invalid workspace id %q", workspaceID)
	}
	if capMicro < 0 {
		return errors.New("treg: daily cap must be >= 0")
	}
	body, _ := json.Marshal(map[string]int64{"daily_cap_micro": capMicro})
	return c.admin(ctx, http.MethodPut, "/budgets/customer/"+workspaceID, body, nil)
}

type TagUsageRow struct {
	Value        string  `json:"value"`
	ChargedMicro int64   `json:"charged_micro"`
	ChargedUSD   float64 `json:"charged_usd"`
	Calls        int64   `json:"calls"`
}

type TagUsage struct {
	Key               string        `json:"key"`
	Days              int           `json:"days"`
	Rows              []TagUsageRow `json:"rows"`
	AttributedMicro   int64         `json:"attributed_micro"`
	UnattributedMicro int64         `json:"unattributed_micro"`
	TotalMicro        int64         `json:"total_micro"`
}

// Reconciled is treg's invoice invariant: attributed + unattributed == total.
// Don't bill from a report that fails it.
func (u *TagUsage) Reconciled() bool {
	return u.AttributedMicro+u.UnattributedMicro == u.TotalMicro
}

// UsageByTag reads the billing ledger grouped by a tag key (GET
// /orgs/<org>/usage/by-tag). Invoice from this, not from our treg_calls.
func (c *Client) UsageByTag(ctx context.Context, key string, days int) (*TagUsage, error) {
	if !validTag(key) || days <= 0 {
		return nil, errors.New("treg: invalid usage query")
	}
	q := url.Values{"key": {key}, "days": {strconv.Itoa(days)}}
	var out TagUsage
	if err := c.admin(ctx, http.MethodGet, "/usage/by-tag?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) admin(ctx context.Context, method, path string, body []byte, out any) error {
	if !c.Enabled() {
		return ErrNotConfigured
	}
	if c.cfg.OrgID == "" {
		return ErrNoOrg
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+"/orgs/"+url.PathEscape(c.cfg.OrgID)+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("treg: build request: %w", err)
	}
	req.Header.Set("X-Treg-Token", c.cfg.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("treg: %s %s: %w", method, path, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("treg: read: %w", err)
	}
	if res.StatusCode >= 300 {
		code := errorCode(data)
		if code == "" {
			code = "http_" + strconv.Itoa(res.StatusCode)
		}
		return &APIError{Status: res.StatusCode, Code: code, Treg: res.Header.Get("X-Treg-Error") == "1"}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("treg: decode: %w", err)
	}
	return nil
}
