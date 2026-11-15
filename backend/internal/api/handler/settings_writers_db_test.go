package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/auth"
	"leadecho/internal/database"
	"leadecho/internal/llm"
)

// DB-backed: each handler that used to write the whole settings object back
// now stores its own key, and the keys other processes own are still there.
func TestSettingsWritersChangeOnlyTheirKeyDB(t *testing.T) {
	dsn := os.Getenv("LEADECHO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEADECHO_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := database.New(pool)
	const others = `{"first_leads_email":{"sent_at":"2026-11-15T08:30:00Z"},"hiring_signals":{"last_run":"2026-11-15T08:00:00Z"}}`
	var ws string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug, settings) VALUES ('sw-'||gen_random_uuid(), 'sw', 'sw-'||gen_random_uuid(), $1::jsonb) RETURNING id`, others).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)
	call := func(h http.HandlerFunc, method, body string) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/", strings.NewReader(body))
		h(rec, req.WithContext(middleware.WithClaims(ctx, &auth.Claims{WorkspaceID: ws})))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
	key := make([]byte, 32)

	onb := NewOnboardingHandler(q, nil, nil, zerolog.Nop())
	call(onb.UpdateOnboarding, http.MethodPatch, `{"step":2}`)
	onb.markOnboardingComplete(ctx, ws)
	notif := NewNotificationHandler(q, "")
	call(notif.GetWebhookConfig, http.MethodGet, ``) // stores a conversion secret
	call(notif.SaveWebhookConfig, http.MethodPut, `{"email_to":"sales@acme.test","enabled":true}`)
	call(NewSettingsHandler(q, key).SaveAPIKey, http.MethodPut, `{"provider":"glm","api_key":"k-123"}`)
	if _, err := llm.NewRouter(q, key, llm.SystemKeys{}, zerolog.Nop()).SaveProviderKey(ctx, ws, "openai", "sk-123", ""); err != nil {
		t.Fatal(err)
	}

	raw, err := q.GetWorkspaceSettings(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Email      map[string]any `json:"first_leads_email"`
		Hiring     map[string]any `json:"hiring_signals"`
		Onboarding onboardingState
		Webhooks   map[string]any
		APIKeys    map[string]string `json:"api_keys"`
		LLM        struct {
			Providers map[string]any
		}
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Email["sent_at"] != "2026-11-15T08:30:00Z" || got.Hiring["last_run"] != "2026-11-15T08:00:00Z" {
		t.Errorf("another process's keys were lost: %s", raw)
	}
	secret, _ := got.Webhooks["conversion_secret"].(string)
	if !got.Onboarding.Completed || got.Onboarding.CompletedAt == nil || got.Webhooks["email_to"] != "sales@acme.test" ||
		!strings.HasPrefix(secret, "whsec_") || got.APIKeys["glm"] == "" || got.LLM.Providers["openai"] == nil {
		t.Errorf("a handler's own key was not stored: %s", raw)
	}
}
