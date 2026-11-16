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

// fakeKeycloak: token endpoint, user create (409 on a known email, Location
// with the new id) and delete by id.
func fakeKeycloak(t *testing.T) (*Client, *[]string, *map[string]any) {
	t.Helper()
	var mu sync.Mutex
	deleted := []string{}
	created := map[string]any{}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			w.Header().Set("Location", srv.URL+"/admin/realms/leadecho/users/kc-123")
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/admin/realms/leadecho/users/"):
			deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/admin/realms/leadecho/users/"))
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL+"/realms/leadecho", "", "leadecho-web", "s"), &deleted, &created
}

func TestNewNeedsFullConfig(t *testing.T) {
	if New("", "", "id", "s") != nil || New("https://kc.test/realms/x", "", "id", "") != nil || New("https://kc.test/nope", "", "id", "s") != nil {
		t.Fatal("incomplete config must disable the client")
	}
}

func TestAdminHostOverride(t *testing.T) {
	c := New("https://auth.example.com/realms/leadecho", "https://kc-admin.example.com/", "id", "s")
	if c.tokenURL != "https://auth.example.com/realms/leadecho/protocol/openid-connect/token" ||
		c.adminURL != "https://kc-admin.example.com/admin/realms/leadecho" {
		t.Fatalf("token %q admin %q", c.tokenURL, c.adminURL)
	}
	if d := New("https://auth.example.com/realms/leadecho", "", "id", "s"); d.adminURL != "https://auth.example.com/admin/realms/leadecho" {
		t.Fatalf("default admin %q", d.adminURL)
	}
}

func TestCreateUserReturnsID(t *testing.T) {
	c, _, created := fakeKeycloak(t)
	id, err := c.CreateUser(context.Background(), "demo@x.test", "Pat Lee", "pw-123456", true)
	if err != nil || id != "kc-123" {
		t.Fatalf("id %q, err %v", id, err)
	}
	got := *created
	if got["email"] != "demo@x.test" || got["emailVerified"] != true || got["firstName"] != "Pat" || got["lastName"] != "Lee" {
		t.Fatalf("created %v", got)
	}
	if _, err := c.CreateUser(context.Background(), "taken@x.test", "T", "pw-123456", true); !errors.Is(err, ErrUserExists) {
		t.Fatalf("want ErrUserExists, got %v", err)
	}
}

func TestDeleteUserByID(t *testing.T) {
	c, deleted, _ := fakeKeycloak(t)
	if err := c.DeleteUserRetry(context.Background(), "kc-123"); err != nil {
		t.Fatal(err)
	}
	if len(*deleted) != 1 || (*deleted)[0] != "kc-123" {
		t.Fatalf("deleted %v", *deleted)
	}
}
