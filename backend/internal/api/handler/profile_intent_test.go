package handler

import (
	"strings"
	"testing"

	"leadecho/internal/database"
)

func TestMergeListenerIntent(t *testing.T) {
	str := func(s string) *string { return &s }
	existing := database.MonitoringProfile{LooksFor: "old looks", Skips: "old skips"}
	tests := []struct {
		name                string
		looks, skips        *string
		wantLooks, wantSkip string
		wantChanged         bool
		wantErr             bool
	}{
		{"nothing sent keeps values", nil, nil, "old looks", "old skips", false, false},
		{"set both trimmed", str("  buyers  "), str(" students "), "buyers", "students", true, false},
		{"explicit empty clears", str(""), nil, "", "old skips", true, false},
		{"too long rejected", str(strings.Repeat("x", maxListenerIntentLen+1)), nil, "", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			looks, skips, changed, err := mergeListenerIntent(existing, tt.looks, tt.skips)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if tt.wantErr {
				return
			}
			if looks != tt.wantLooks || skips != tt.wantSkip || changed != tt.wantChanged {
				t.Fatalf("got %q %q %v", looks, skips, changed)
			}
		})
	}
}
