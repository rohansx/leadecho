package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/apikey"
	"leadecho/internal/database"
)

type fakeAPIKeyStore struct {
	created *database.CreateWorkspaceAPIKeyParams
	revoked *database.RevokeWorkspaceAPIKeyParams
	listWS  string
}

func (f *fakeAPIKeyStore) CreateWorkspaceAPIKey(_ context.Context, a database.CreateWorkspaceAPIKeyParams) (database.WorkspaceApiKey, error) {
	f.created = &a
	return database.WorkspaceApiKey{ID: "k1", WorkspaceID: a.WorkspaceID, Name: a.Name, Prefix: a.Prefix, KeyHash: a.KeyHash, Scopes: a.Scopes}, nil
}
func (f *fakeAPIKeyStore) ListWorkspaceAPIKeys(_ context.Context, ws string) ([]database.WorkspaceApiKey, error) {
	f.listWS = ws
	return []database.WorkspaceApiKey{{ID: "k1", Name: "ci", Prefix: "le_live_abcdef", KeyHash: "secret-hash", Scopes: []string{"read"}, RevokedAt: pgtype.Timestamptz{Valid: true}}}, nil
}
func (f *fakeAPIKeyStore) RevokeWorkspaceAPIKey(_ context.Context, a database.RevokeWorkspaceAPIKeyParams) (database.WorkspaceApiKey, error) {
	if a.ID != "11111111-1111-1111-1111-111111111111" {
		return database.WorkspaceApiKey{}, pgx.ErrNoRows
	}
	f.revoked = &a
	return database.WorkspaceApiKey{ID: a.ID, Name: "ci", Scopes: []string{"read"}, RevokedAt: pgtype.Timestamptz{Valid: true}}, nil
}

func TestWorkspaceAPIKeyCreate(t *testing.T) {
	tests := []struct {
		name, body string
		want       int
		wantScopes []string
	}{
		{"read default", `{"name":"Claude"}`, 201, []string{"read"}},
		{"write implies read", `{"name":"Cursor","scopes":["write"]}`, 201, []string{"read", "write"}},
		{"missing name", `{"name":"  "}`, 400, nil},
		{"long name", `{"name":"` + strings.Repeat("n", 101) + `"}`, 400, nil},
		{"bad scope", `{"name":"x","scopes":["admin"]}`, 400, nil},
		{"bad json", `{`, 400, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAPIKeyStore{}
			h := NewWorkspaceAPIKeyHandler(store)
			rec := httptest.NewRecorder()
			h.Create(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body)))
			if rec.Code != tt.want {
				t.Fatalf("status %d want %d: %s", rec.Code, tt.want, rec.Body)
			}
			if tt.want != 201 {
				return
			}
			var resp struct {
				Secret string `json:"secret"`
				Key    struct {
					Prefix string   `json:"prefix"`
					Scopes []string `json:"scopes"`
				} `json:"key"`
			}
			json.Unmarshal(rec.Body.Bytes(), &resp)
			if !apikey.LooksLikeKey(resp.Secret) || store.created.KeyHash != apikey.Hash(resp.Secret) {
				t.Fatalf("secret %q not stored as its hash", resp.Secret)
			}
			if !strings.HasPrefix(resp.Secret, resp.Key.Prefix) || strings.Join(resp.Key.Scopes, ",") != strings.Join(tt.wantScopes, ",") {
				t.Fatalf("resp %+v", resp)
			}
			if strings.Contains(rec.Body.String(), "key_hash") {
				t.Fatal("hash must never be returned")
			}
		})
	}
}

func TestWorkspaceAPIKeyListAndRevoke(t *testing.T) {
	store := &fakeAPIKeyStore{}
	h := NewWorkspaceAPIKeyHandler(store)
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "secret-hash") || !strings.Contains(rec.Body.String(), `"revoked":true`) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if store.listWS == "" {
		t.Fatal("list must be workspace-scoped")
	}

	revoke := func(id string) int {
		req := httptest.NewRequest(http.MethodDelete, "/"+id, nil)
		rc := chi.NewRouteContext()
		rc.URLParams.Add("id", id)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
		rec := httptest.NewRecorder()
		h.Revoke(rec, req)
		return rec.Code
	}
	if got := revoke("11111111-1111-1111-1111-111111111111"); got != 200 || store.revoked.WorkspaceID == "" {
		t.Fatalf("revoke: %d", got)
	}
	if got := revoke("99999999-9999-9999-9999-999999999999"); got != 404 {
		t.Fatalf("revoke missing: %d", got)
	}
	if got := revoke("not-a-uuid"); got != 400 {
		t.Fatalf("revoke bad id: %d", got)
	}
}
