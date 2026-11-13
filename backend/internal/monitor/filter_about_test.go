package monitor

import (
	"strings"
	"testing"

	"leadecho/internal/database"
)

// The noise filter is told what the product is and who buys it; with no
// description there is nothing useful to tell it.
func TestFilterAbout(t *testing.T) {
	got := filterAbout(database.BrandProfile{Product: "Utkrusht", Overview: "Technical skill assessments.", TargetAudience: " CTOs hiring developers "})
	for _, want := range []string{"Utkrusht: Technical skill assessments.", "Bought by: CTOs hiring developers"} {
		if !strings.Contains(got, want) {
			t.Errorf("about = %q, lacks %q", got, want)
		}
	}
	if filterAbout(database.BrandProfile{Product: "Utkrusht"}) != "" {
		t.Error("no overview: nothing to say")
	}
}
