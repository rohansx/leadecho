package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated
// database (e.g. the e2e Postgres). Covers the OAuth account-linking policy.
func TestOAuthLinkingPolicyDB(t *testing.T) {
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

	newHandler := func(requireVerif bool) *OAuthHandler {
		cfg := Config{JWTSecret: "s", FrontendURL: "http://app.test"}
		if requireVerif {
			cfg.RequireVerification, cfg.ResendAPIKey = true, "re_test"
		}
		core := NewEmailHandler(cfg, q, nil, NewSessionVersions(q), zerolog.Nop())
		return NewOAuthHandler(NewGoogleProvider("id", "s", "cb"), core)
	}
	passwordAccount := func(h *OAuthHandler, verified bool) database.User {
		email := fmt.Sprintf("link-%d@leadecho.test", time.Now().UnixNano())
		u, _, err := h.core.createAccount(ctx, "email_"+email, email, "Link", "", "$2a$10$abcdefghijklmnopqrstuvCDEFGHIJKLMNOPQRSTUVWXYZ012345", verified)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	oauthID := func(u database.User) identity {
		return identity{ID: fmt.Sprint(time.Now().UnixNano()), Email: u.Email, EmailVerified: true, Name: "G"}
	}

	t.Run("verification off: unproven password account is not linked", func(t *testing.T) {
		h := newHandler(false)
		u := passwordAccount(h, false)
		if _, _, _, err := h.signIn(ctx, oauthID(u)); !errors.Is(err, errAccountExists) {
			t.Fatalf("want errAccountExists, got %v", err)
		}
		if got, _ := q.GetUser(ctx, u.ID); !got.PasswordHash.Valid {
			t.Fatal("password must be untouched")
		}
	})

	t.Run("verification on: unproven password is dropped and sessions revoked", func(t *testing.T) {
		h := newHandler(true)
		u := passwordAccount(h, false)
		before, _ := q.GetUserAuthState(ctx, u.ID)
		got, _, created, err := h.signIn(ctx, oauthID(u))
		if err != nil || created || got.ID != u.ID {
			t.Fatalf("want link to %s, got %s created=%v err=%v", u.ID, got.ID, created, err)
		}
		after, _ := q.GetUserAuthState(ctx, u.ID)
		row, _ := q.GetUser(ctx, u.ID)
		if row.PasswordHash.Valid || !after.EmailVerified || after.SessionVersion != before.SessionVersion+1 {
			t.Fatalf("password=%v verified=%v version %d→%d", row.PasswordHash.Valid, after.EmailVerified, before.SessionVersion, after.SessionVersion)
		}
	})

	t.Run("proven password account links and keeps its password", func(t *testing.T) {
		h := newHandler(false)
		u := passwordAccount(h, true)
		got, _, _, err := h.signIn(ctx, oauthID(u))
		if err != nil || got.ID != u.ID {
			t.Fatalf("want link, got %v", err)
		}
		if row, _ := q.GetUser(ctx, u.ID); !row.PasswordHash.Valid {
			t.Fatal("verified account's password must be kept")
		}
	})
}
