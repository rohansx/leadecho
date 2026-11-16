package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/lobby"
)

const testAccountID = "6f1c2a1e-6b1d-4c3e-9a51-3f1b8e4b2c10"

type fakeClaimStore struct {
	claimErr      error
	gotID, gotEml string
	pwSet         bool
}

func (f *fakeClaimStore) ClaimAccount(_ context.Context, id, email string) (database.User, error) {
	f.gotID, f.gotEml = id, email
	if f.claimErr != nil {
		return database.User{}, f.claimErr
	}
	return database.User{ID: id, WorkspaceID: "ws-1", Email: email, Name: "Pat", Role: database.UserRoleAdmin, SessionVersion: 3}, nil
}

func (f *fakeClaimStore) SetInitialPassword(context.Context, string, string) (bool, error) {
	f.pwSet = true
	return true, nil
}

// lobbyStub mocks Lobby's redeem endpoint: 200 with a redemption, or status.
func lobbyStub(t *testing.T, status int) (*lobby.Client, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/claims/redeem" || r.Header.Get("Authorization") != "Bearer sk_test" {
			t.Errorf("unexpected request %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body struct{ Code string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Code != "abc123" {
			t.Errorf("code = %q", body.Code)
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"account_ref": testAccountID, "email": "Pat@Acme.com", "label": "Pat at Acme",
				"claim_id": "cl_1", "signup_id": "su_1", "reporter_token": "rt_1",
			})
		} else {
			_, _ = w.Write([]byte(`{"ok":false,"error":"not found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return lobby.New(srv.URL, "sk_test"), &calls
}

func newClaimHandler(l *lobby.Client, store claimStore) *ClaimHandler {
	core := NewEmailHandler(Config{JWTSecret: "secret", FrontendURL: "http://app.test"}, nil, nil, nil, zerolog.Nop())
	return &ClaimHandler{core: core, lobby: l, store: store}
}

func redeem(h *ClaimHandler) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.Redeem(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/claim?claim=abc123", nil))
	return w
}

func cookie(w *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == name && c.MaxAge > 0 {
			return c
		}
	}
	return nil
}

func TestClaimRedeemSuccessSwapsEmailAndStartsSession(t *testing.T) {
	l, _ := lobbyStub(t, http.StatusOK)
	store := &fakeClaimStore{}
	h := newClaimHandler(l, store)

	w := redeem(h)

	if w.Code != http.StatusFound || w.Header().Get("Location") != "http://app.test/set-password" {
		t.Fatalf("got %d → %q", w.Code, w.Header().Get("Location"))
	}
	if store.gotID != testAccountID || store.gotEml != "pat@acme.com" {
		t.Fatalf("claimed %q with %q", store.gotID, store.gotEml)
	}
	s := cookie(w, "session")
	if s == nil {
		t.Fatal("no session cookie")
	}
	claims, err := ValidateToken("secret", s.Value)
	if err != nil || claims.UserID != testAccountID || claims.Email != "pat@acme.com" || claims.SessionVersion != 3 {
		t.Fatalf("session claims %+v, err %v", claims, err)
	}

	// The set-password step needs that session plus the ticket cookie.
	pw := cookie(w, claimPwCookie)
	if pw == nil {
		t.Fatal("no password ticket cookie")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/claim/password", strings.NewReader(`{"password":"correct horse"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(s)
	req.AddCookie(pw)
	pwW := httptest.NewRecorder()
	h.SetPassword(pwW, req)
	if pwW.Code != http.StatusOK || !store.pwSet {
		t.Fatalf("set password: %d %s", pwW.Code, pwW.Body)
	}
}

func TestClaimSetPasswordNeedsTicket(t *testing.T) {
	store := &fakeClaimStore{}
	h := newClaimHandler(nil, store)
	tok, _ := IssueToken("secret", testAccountID, "ws-1", "pat@acme.com", "Pat", "admin", 3)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/claim/password", strings.NewReader(`{"password":"correct horse"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: tok})
	w := httptest.NewRecorder()
	h.SetPassword(w, req)
	if w.Code != http.StatusForbidden || store.pwSet {
		t.Fatalf("session alone set a password: %d", w.Code)
	}
}

func TestClaimRedeemRefusals(t *testing.T) {
	cases := []struct {
		name      string
		lobby     int
		storeErr  error
		wantError string
		wantStore bool
	}{
		{"lobby 404 shows expired page", http.StatusNotFound, nil, "claim_expired", false},
		{"lobby 500 shows expired page", http.StatusInternalServerError, nil, "claim_expired", false},
		{"email already has an account", http.StatusOK, database.ErrEmailTaken, "claim_email_taken", true},
		{"missing account", http.StatusOK, pgx.ErrNoRows, "claim_expired", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, calls := lobbyStub(t, tc.lobby)
			store := &fakeClaimStore{claimErr: tc.storeErr}
			w := redeem(newClaimHandler(l, store))

			if want := "http://app.test/login?error=" + tc.wantError; w.Header().Get("Location") != want {
				t.Fatalf("redirect %q, want %q", w.Header().Get("Location"), want)
			}
			if cookie(w, "session") != nil || cookie(w, claimPwCookie) != nil {
				t.Fatal("refused claim must not start a session")
			}
			if *calls != 1 || (store.gotID != "") != tc.wantStore {
				t.Fatalf("lobby calls %d, store called %v", *calls, store.gotID != "")
			}
		})
	}
}

func TestClaimRedeemWithoutCodeSkipsLobby(t *testing.T) {
	l, calls := lobbyStub(t, http.StatusOK)
	w := httptest.NewRecorder()
	newClaimHandler(l, &fakeClaimStore{}).Redeem(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/claim", nil))
	if *calls != 0 || w.Header().Get("Location") != "http://app.test/login" {
		t.Fatalf("calls %d, location %q", *calls, w.Header().Get("Location"))
	}
}
