package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated database.
func TestClaimableAccountsDB(t *testing.T) {
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
	q := New(pool)
	uniq := func(p string) string { return fmt.Sprintf("%s-%d@leadecho.test", p, time.Now().UnixNano()) }

	placeholder := uniq("demo+acct")
	id, ws, err := q.CreateClaimableAccount(ctx, placeholder, "Demo", "$2a$10$hash")
	if err != nil || id == "" || ws == "" {
		t.Fatalf("create: %q %q %v", id, ws, err)
	}
	if _, _, err := q.CreateClaimableAccount(ctx, placeholder, "Demo", "$2a$10$hash"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate temp email: want ErrEmailTaken, got %v", err)
	}
	if ok, _ := q.IsClaimable(ctx, id); !ok {
		t.Fatal("new temp account must be claimable")
	}

	// An email another account already uses is refused and nothing changes.
	other := uniq("taken")
	if _, _, err := q.CreateClaimableAccount(ctx, other, "Other", "$2a$10$hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ClaimAccount(ctx, id, other); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("conflict: want ErrEmailTaken, got %v", err)
	}
	if u, _ := q.GetUser(ctx, id); u.Email != placeholder || !u.PasswordHash.Valid {
		t.Fatalf("refused claim changed the account: %+v", u)
	}

	// Unknown account.
	if _, err := q.ClaimAccount(ctx, "00000000-0000-0000-0000-000000000000", uniq("x")); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing account: want ErrNoRows, got %v", err)
	}

	// Access made while building the account: a reset token sent to the
	// placeholder mailbox, a workspace API key and an extension token.
	tag := fmt.Sprint(time.Now().UnixNano())
	if err := q.CreateAuthToken(ctx, id, "reset", "h-"+tag, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_api_keys (workspace_id, name, prefix, key_hash) VALUES ($1, 'k', 'le_', $2)`, ws, "k-"+tag); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO extension_tokens (workspace_id, token) VALUES ($1, $2)`, ws, "e-"+tag); err != nil {
		t.Fatal(err)
	}

	// Success: email swapped and verified, temp password dropped, sessions revoked, flag cleared.
	real := uniq("pat")
	u, err := q.ClaimAccount(ctx, id, real)
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != real || !u.EmailVerified || u.PasswordHash.Valid || u.SessionVersion != 1 || u.ClerkUserID != "email_"+real {
		t.Fatalf("claimed user: %+v", u)
	}
	if ok, _ := q.IsClaimable(ctx, id); ok {
		t.Fatal("claimed account must not be claimable again")
	}
	if _, err := q.ConsumeAuthToken(ctx, "reset", "h-"+tag); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("pre-claim reset token still works: %v", err)
	}
	var liveKeys, extTokens int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM workspace_api_keys WHERE workspace_id = $1 AND revoked_at IS NULL`, ws).Scan(&liveKeys)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM extension_tokens WHERE workspace_id = $1`, ws).Scan(&extTokens)
	if liveKeys != 0 || extTokens != 0 {
		t.Fatalf("setup access survived the claim: %d api keys, %d extension tokens", liveKeys, extTokens)
	}
	if _, err := q.ClaimAccount(ctx, id, uniq("again")); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second claim: want ErrNoRows, got %v", err)
	}

	// First password only once.
	if set, err := q.SetInitialPassword(ctx, id, "$2a$10$new"); err != nil || !set {
		t.Fatalf("initial password: %v %v", set, err)
	}
	if set, _ := q.SetInitialPassword(ctx, id, "$2a$10$other"); set {
		t.Fatal("initial password must not overwrite an existing one")
	}
}
