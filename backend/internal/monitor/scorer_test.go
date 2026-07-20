package monitor

import "testing"

func TestScoreStage1Rules(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"too short", "short", false},
		{"spam pattern", "Click here to win a prize! " + string(make([]byte, 50)), false},
		{"valid post", "We are a 10-person team looking for a CRM that integrates with Reddit monitoring. Budget is around $100/mo.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scoreStage1Rules(tt.content); got != tt.want {
				t.Errorf("scoreStage1Rules() = %v, want %v", got, tt.want)
			}
		})
	}
}
