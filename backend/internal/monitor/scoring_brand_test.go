package monitor

import (
	"strings"
	"testing"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

func TestWithBrand(t *testing.T) {
	bp := database.BrandProfile{Product: "Acme", Overview: "CRM for agencies.", TargetAudience: " Agency owners "}
	sc := withBrand(ai.ScoringContext{Product: "- Buyers: asks for a CRM"}, bp)
	if !strings.HasPrefix(sc.Product, "- Acme: CRM for agencies.") || !strings.Contains(sc.Product, "- Buyers: asks for a CRM") {
		t.Fatalf("product = %q", sc.Product)
	}
	if sc.Buyers != "Agency owners" {
		t.Fatalf("buyers = %q", sc.Buyers)
	}
	if got := withBrand(ai.ScoringContext{Product: "x"}, database.BrandProfile{}); got.Product != "x" || got.Buyers != "" {
		t.Fatalf("empty brand changed context: %+v", got)
	}
}
