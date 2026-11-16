package kcadmin

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// Live: runs only with KCADMIN_TEST_ISSUER/CLIENT_ID/SECRET pointing at a
// test realm whose client has a service account with manage-users.
func TestLiveKeycloakCreateAndDelete(t *testing.T) {
	c := New(os.Getenv("KCADMIN_TEST_ISSUER"), os.Getenv("KCADMIN_TEST_ADMIN_HOST"), os.Getenv("KCADMIN_TEST_CLIENT_ID"), os.Getenv("KCADMIN_TEST_CLIENT_SECRET"))
	if c == nil {
		t.Skip("KCADMIN_TEST_* not set")
	}
	ctx := context.Background()
	email := fmt.Sprintf("live-%d@leadecho.test", time.Now().UnixNano())
	id, err := c.CreateUser(ctx, email, "Live Test", "pw-12345678", true)
	if err != nil || id == "" {
		t.Fatalf("create: %q %v", id, err)
	}
	if _, err := c.CreateUser(ctx, email, "Again", "pw-12345678", true); err != ErrUserExists {
		t.Fatalf("duplicate: want ErrUserExists, got %v", err)
	}
	if err := c.DeleteUserRetry(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := c.CreateUser(ctx, email, "After delete", "pw-12345678", true); err != nil {
		t.Fatalf("email should be free after delete: %v", err)
	}
}
