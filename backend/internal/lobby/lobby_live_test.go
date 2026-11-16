package lobby

import (
	"context"
	"errors"
	"os"
	"testing"
)

// Live: runs only with LOBBY_TEST_KEY (a waitlist sk_ key) and
// LOBBY_TEST_TESTER (an email added as a tester on that waitlist).
func TestLiveFeedbackReporter(t *testing.T) {
	c := New("https://trylobby.app", os.Getenv("LOBBY_TEST_KEY"))
	if c == nil || os.Getenv("LOBBY_TEST_TESTER") == "" {
		t.Skip("LOBBY_TEST_KEY / LOBBY_TEST_TESTER not set")
	}
	ctx := context.Background()
	pk, err := c.PublicKey(ctx)
	if err != nil || len(pk) < 4 || pk[:3] != "pk_" {
		t.Fatalf("public key %q: %v", pk, err)
	}
	tok, err := c.ReporterToken(ctx, os.Getenv("LOBBY_TEST_TESTER"))
	if err != nil || len(tok) < 4 || tok[:3] != "rt_" {
		t.Fatalf("reporter token for tester: len %d, %v", len(tok), err)
	}
	if _, err := c.ReporterToken(ctx, "nobody-on-the-list@leadecho.test"); !errors.Is(err, ErrFeedbackOff) {
		t.Fatalf("non-tester: want ErrFeedbackOff, got %v", err)
	}
}
