package automation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInputNormalize(t *testing.T) {
	tests := []struct {
		name    string
		in      Input
		check   func(t *testing.T, got Input)
		wantErr string
	}{
		{
			name: "triage defaults",
			in:   Input{Name: " Morning triage ", Kind: KindDailyTriage},
			check: func(t *testing.T, got Input) {
				if got.Name != "Morning triage" || got.Config.TopN != 10 || got.Config.NoiseBelow != 3 || got.Schedule.Frequency != "daily" {
					t.Fatalf("got %+v", got)
				}
			},
		},
		{
			name: "sweep defaults and clamps",
			in:   Input{Name: "s", Kind: KindLeadSweep, Config: Config{MaxLeads: 1000}},
			check: func(t *testing.T, got Input) {
				if got.Config.MinScore != 6 || got.Config.MaxLeads != 100 {
					t.Fatalf("got %+v", got.Config)
				}
			},
		},
		{
			name: "review defaults",
			in:   Input{Name: "r", Kind: KindAttributionReview},
			check: func(t *testing.T, got Input) {
				if got.Config.Days != 7 || got.Config.MinMentions != 5 {
					t.Fatalf("got %+v", got.Config)
				}
			},
		},
		{name: "name required", in: Input{Kind: KindLeadSweep}, wantErr: "name"},
		{name: "name too long", in: Input{Name: strings.Repeat("x", 201), Kind: KindLeadSweep}, wantErr: "name"},
		{name: "bad kind", in: Input{Name: "x", Kind: "nope"}, wantErr: "kind"},
		{name: "bad schedule", in: Input{Name: "x", Kind: KindLeadSweep, Schedule: Schedule{Frequency: "hourly"}}, wantErr: "frequency"},
		{name: "score out of range", in: Input{Name: "x", Kind: KindLeadSweep, Config: Config{MinScore: 11}}, wantErr: "min_score"},
		{name: "custom needs steps", in: Input{Name: "x", Kind: KindCustom}, wantErr: "steps"},
		{name: "custom too many steps", in: Input{Name: "x", Kind: KindCustom, Config: Config{Steps: make([]Step, 6)}}, wantErr: "steps"},
		{name: "custom step without tool", in: Input{Name: "x", Kind: KindCustom, Config: Config{Steps: []Step{{}}}}, wantErr: "tool"},
		{name: "custom args must be object", in: Input{Name: "x", Kind: KindCustom, Config: Config{Steps: []Step{{Tool: "weekly_summary", Args: json.RawMessage(`[1]`)}}}}, wantErr: "args"},
		{
			name: "custom ok",
			in:   Input{Name: "x", Kind: KindCustom, Config: Config{Steps: []Step{{Tool: " weekly_summary "}}}},
			check: func(t *testing.T, got Input) {
				if got.Config.Steps[0].Tool != "weekly_summary" || string(got.Config.Steps[0].Args) != "{}" {
					t.Fatalf("got %+v", got.Config.Steps)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Normalize()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, got)
		})
	}
}
