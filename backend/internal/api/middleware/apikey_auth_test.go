package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/apikey"
	"leadecho/internal/database"
)

type fakeKeyStore struct {
	mu      sync.Mutex
	keys    map[string]database.WorkspaceApiKey // by hash
	touched int
}

func (f *fakeKeyStore) GetActiveWorkspaceAPIKeyByHash(_ context.Context, hash string) (database.WorkspaceApiKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k, ok := f.keys[hash]
	if !ok || k.RevokedAt.Valid {
		return database.WorkspaceApiKey{}, pgx.ErrNoRows
	}
	return k, nil
}

func (f *fakeKeyStore) TouchWorkspaceAPIKey(context.Context, database.TouchWorkspaceAPIKeyParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touched++
	return nil
}

type memCounter struct {
	mu sync.Mutex
	n  map[string]int64
}

func (m *memCounter) Incr(_ context.Context, key string, window time.Duration) (int64, time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.n == nil {
		m.n = map[string]int64{}
	}
	m.n[key]++
	return m.n[key], window, nil
}
func (m *memCounter) Reset(_ context.Context, key string) error { return nil }

type errCounter struct{}

func (errCounter) Incr(context.Context, string, time.Duration) (int64, time.Duration, error) {
	return 0, 0, errors.New("redis down")
}
func (errCounter) Reset(context.Context, string) error { return nil }

func TestAPIKeyAuth(t *testing.T) {
	good, _ := apikey.Generate()
	revoked, _ := apikey.Generate()
	unknown, _ := apikey.Generate()
	store := &fakeKeyStore{keys: map[string]database.WorkspaceApiKey{
		good.Hash:    {ID: "k1", WorkspaceID: "ws-1", KeyHash: good.Hash, Name: "ci", Scopes: []string{"read", "write"}},
		revoked.Hash: {ID: "k2", WorkspaceID: "ws-1", KeyHash: revoked.Hash, RevokedAt: pgtype.Timestamptz{Valid: true}},
	}}

	var gotWS string
	var gotScopes []string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotWS, gotScopes = APIKeyPrincipal(r.Context())
		if WorkspaceID(r.Context()) != gotWS {
			t.Error("APIKeyAuth must set the same claims the JWT middleware sets")
		}
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name   string
		header string
		want   int
		wantWS string
	}{
		{"valid key", "Bearer " + good.Plaintext, 200, "ws-1"},
		{"missing header", "", 401, ""},
		{"wrong scheme", "Basic " + good.Plaintext, 401, ""},
		{"malformed key", "Bearer le_live_nope", 401, ""},
		{"unknown key", "Bearer " + unknown.Plaintext, 401, ""},
		{"revoked key", "Bearer " + revoked.Plaintext, 401, ""},
	}
	h := APIKeyAuth(store, nil, 0)(next)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWS, gotScopes = "", nil
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tt.want, rec.Body)
			}
			if gotWS != tt.wantWS {
				t.Fatalf("workspace %q, want %q", gotWS, tt.wantWS)
			}
			if tt.want == 401 && !strings.Contains(rec.Header().Get("WWW-Authenticate"), "Bearer") {
				t.Fatal("401 must carry WWW-Authenticate: Bearer")
			}
			if tt.want == 200 && len(gotScopes) != 2 {
				t.Fatalf("scopes %v", gotScopes)
			}
		})
	}
}

func TestAPIKeyAuthRateLimit(t *testing.T) {
	good, _ := apikey.Generate()
	store := &fakeKeyStore{keys: map[string]database.WorkspaceApiKey{
		good.Hash: {ID: "k1", WorkspaceID: "ws-1", KeyHash: good.Hash, Scopes: []string{"read"}},
	}}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	call := func(h http.Handler) int {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Authorization", "Bearer "+good.Plaintext)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	h := APIKeyAuth(store, &memCounter{}, 2)(ok)
	for i, want := range []int{200, 200, 429} {
		if got := call(h); got != want {
			t.Fatalf("call %d: status %d, want %d", i, got, want)
		}
	}
	// A broken limiter backend fails open rather than locking every agent out.
	if got := call(APIKeyAuth(store, errCounter{}, 1)(ok)); got != 200 {
		t.Fatalf("fail-open: got %d", got)
	}
}

func TestRequireScope(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	h := RequireScope(apikey.ScopeWrite)(ok)
	for _, tt := range []struct {
		scopes []string
		want   int
	}{{[]string{"read"}, 403}, {[]string{"read", "write"}, 200}, {nil, 403}} {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req = req.WithContext(context.WithValue(req.Context(), apiKeyScopesKey{}, tt.scopes))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Fatalf("scopes %v: got %d want %d", tt.scopes, rec.Code, tt.want)
		}
	}
}

func TestAPIKeyPrincipalWithoutAPIKey(t *testing.T) {
	if ws, _ := APIKeyPrincipal(context.Background()); ws != "" {
		t.Fatalf("must not fall back to the dev workspace, got %q", ws)
	}
}
