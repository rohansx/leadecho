package kcadmin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeKeycloak: token endpoint, user create (409 on a known email), search by
// username (Keycloak's exact search can still return near matches; we must
// filter) and delete.
func fakeKeycloak(t *testing.T) (*Client, *[]string, *map[string]any) {
	t.Helper()
	var mu sync.Mutex
	deleted := []string{}
	created := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/realms/leadecho/protocol/openid-connect/token":
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_secret") != "s" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok"}`))
		case r.Header.Get("Authorization") != "Bearer tok":
			w.WriteHeader(http.StatusUnauthorized)
		case r.Method == http.MethodPost && r.URL.Path == "/admin/realms/leadecho/users":
			_ = json.NewDecoder(r.Body).Decode(&created)
			if created["email"] == "taken@x.test" {
				w.WriteHeader(http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/leadecho/users":
			if r.URL.Query().Get("username") != "leadecho-temp-acc1" {
				t.Errorf("searched by %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"id":"kc-real","username":"leadecho-temp-acc10"},{"id":"kc-temp","username":"leadecho-temp-acc1"}]`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/admin/realms/leadecho/users/"):
			deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/admin/realms/leadecho/users/"))
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL+"/realms/leadecho", "leadecho-web", "s"), &deleted, &created
}

func TestNewNeedsFullConfig(t *testing.T) {
	if New("", "id", "s") != nil || New("https://kc.test/realms/x", "id", "") != nil || New("https://kc.test/nope", "id", "s") != nil {
		t.Fatal("incomplete config must disable the client")
	}
}

func TestCreateTempUser(t *testing.T) {
	c, _, created := fakeKeycloak(t)
	if err := c.CreateTempUser(context.Background(), "acc1", "demo@x.test", "Pat Lee", "pw-123456", true); err != nil {
		t.Fatal(err)
	}
	got := *created
	if got["username"] != "leadecho-temp-acc1" || got["email"] != "demo@x.test" || got["emailVerified"] != true || got["firstName"] != "Pat" {
		t.Fatalf("created %v", got)
	}
	if err := c.CreateTempUser(context.Background(), "acc2", "taken@x.test", "T", "pw-123456", true); !errors.Is(err, ErrUserExists) {
		t.Fatalf("want ErrUserExists, got %v", err)
	}
}

func TestDeleteTempUserOnlyDeletesExactUsername(t *testing.T) {
	c, deleted, _ := fakeKeycloak(t)
	if err := c.DeleteTempUserRetry(context.Background(), "acc1"); err != nil {
		t.Fatal(err)
	}
	if len(*deleted) != 1 || (*deleted)[0] != "kc-temp" {
		t.Fatalf("deleted %v", *deleted)
	}
}
