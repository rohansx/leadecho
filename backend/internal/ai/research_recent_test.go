package ai

import (
	"testing"
	"time"
)

func TestResearchBriefRecentReason(t *testing.T) {
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		b    *ResearchBrief
		want bool
	}{
		{nil, false},
		{&ResearchBrief{WhyNow: "raised", WhyNowDate: "2026-07"}, true},
		{&ResearchBrief{WhyNow: "raised", WhyNowDate: "2025-03"}, false}, // 20 months ago (smallcase, live)
		{&ResearchBrief{WhyNow: "raised", WhyNowDate: ""}, false},
		{&ResearchBrief{WhyNow: "", WhyNowDate: "2026-07"}, false},
		{&ResearchBrief{WhyNow: "raised", WhyNowDate: "2027-01"}, false}, // future
	}
	for _, c := range cases {
		if got := c.b.RecentReason(now); got != c.want {
			t.Errorf("RecentReason(%+v) = %v", c.b, got)
		}
	}
	if monthOrEmpty(" 2026-07 ") != "2026-07" || monthOrEmpty("July 2026") != "" {
		t.Fatal("monthOrEmpty keeps only YYYY-MM")
	}
}
