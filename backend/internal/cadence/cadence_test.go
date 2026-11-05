package cadence

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestParseAndValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Map
		wantErr bool
	}{
		{"empty", ``, Map{}, false},
		{"empty object", `{}`, Map{}, false},
		{"valid", `{"twitter":"1h","reddit":"30m"}`, Map{"twitter": "1h", "reddit": "30m"}, false},
		{"bad json", `{"twitter":`, nil, true},
		{"unsupported value", `{"twitter":"7m"}`, nil, true},
		{"unknown platform", `{"myspace":"1h"}`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.in))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInterval(t *testing.T) {
	m := Map{"twitter": "6h", "reddit": "15m"}
	if got := m.Interval("twitter"); got != 6*time.Hour {
		t.Errorf("twitter = %v", got)
	}
	if got := m.Interval("hackernews"); got != Default {
		t.Errorf("default = %v, want %v", got, Default)
	}
	if got := (Map{"x": "garbage"}).Interval("x"); got != Default {
		t.Errorf("garbage falls back to default, got %v", got)
	}
}

func TestDue(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cad := Map{"reddit": "30m", "twitter": "6h"}
	last := map[string]time.Time{
		"reddit":     now.Add(-31 * time.Minute), // due
		"twitter":    now.Add(-time.Hour),        // not due (6h)
		"hackernews": now.Add(-50 * time.Minute), // not due (default 1h)
	}
	got := Due([]string{"reddit", "twitter", "hackernews", "devto"}, cad, last, now)
	want := []string{"reddit", "devto"} // devto never scanned
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Due = %v, want %v", got, want)
	}
	// Small clock skew tolerance: a scan just under the interval counts as due,
	// so a 1h cadence on a 1h tick does not slip to every other tick.
	last2 := map[string]time.Time{"hackernews": now.Add(-time.Hour + 30*time.Second)}
	if got := Due([]string{"hackernews"}, nil, last2, now); len(got) != 1 {
		t.Errorf("want due within tolerance, got %v", got)
	}
}

func TestPreview(t *testing.T) {
	p := Preview([]string{"twitter", "linkedin", "reddit", "bluesky"}, Map{"twitter": "1h", "linkedin": "24h"})
	byPlatform := map[string]PlatformCost{}
	for _, c := range p.Platforms {
		byPlatform[c.Platform] = c
	}
	if got := byPlatform["twitter"].CallsPerMonth; got != 720 {
		t.Errorf("twitter calls = %d, want 720", got)
	}
	if got := byPlatform["twitter"].USDPerMonth; math.Abs(got-0.54) > 1e-9 {
		t.Errorf("twitter usd = %v, want 0.54", got)
	}
	if got := byPlatform["linkedin"].CallsPerMonth; got != 30 {
		t.Errorf("linkedin calls = %d, want 30", got)
	}
	if got := byPlatform["linkedin"].USDPerMonth; math.Abs(got-0.06) > 1e-9 {
		t.Errorf("linkedin usd = %v, want 0.06", got)
	}
	if byPlatform["reddit"].USDPerMonth != 0 || byPlatform["bluesky"].USDPerMonth != 0 {
		t.Error("free platforms must cost 0")
	}
	if math.Abs(p.USDPerMonth-0.60) > 1e-9 {
		t.Errorf("total = %v, want 0.60", p.USDPerMonth)
	}
	if byPlatform["reddit"].Cadence != "1h" {
		t.Errorf("default cadence label = %q, want 1h", byPlatform["reddit"].Cadence)
	}
}
