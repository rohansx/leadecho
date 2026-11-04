package digest

import (
	"strings"
	"testing"
	"time"
)

func sampleData() Data {
	return Data{
		Kind:   KindWeekly,
		Period: Period{Start: time.Date(2026, 10, 26, 9, 30, 0, 0, time.UTC), End: time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)},
		TopLeads: []Lead{
			{Title: "Looking for a <script>alert(1)</script> tool", URL: "https://reddit.com/r/saas/1", Platform: "reddit", Author: "bob", Intent: "buy_signal", Score: 8.7},
			{Title: "", URL: "https://news.ycombinator.com/item?id=2", Platform: "hackernews", Author: "", Intent: "", Score: 6.1},
		},
		Platforms:     []Count{{"reddit", 30}, {"hackernews", 12}},
		Intents:       []Count{{"buy_signal", 9}, {"recommendation_ask", 2}, {"comparison", 1}, {"complaint", 4}, {"general", 20}},
		NoisyKeywords: []KeywordStat{{"crm", 40, 1.2}, {"ai", 25, 2.0}},
		RepliesPosted: 3,
		Clicks:        17,
		Signups:       2,
		RevenueCents:  123450,
		DashboardURL:  "https://app.leadecho.io",
	}
}

func TestNoisyKeywords(t *testing.T) {
	tests := []struct {
		name string
		in   []KeywordStat
		want string
	}{
		{"none", nil, ""},
		{"few mentions is not noisy", []KeywordStat{{"a", 9, 0.5}}, ""},
		{"high fit is not noisy", []KeywordStat{{"a", 50, 6.0}}, ""},
		{"boundary score is not noisy", []KeywordStat{{"a", 50, NoisyMaxAvgScore}}, ""},
		{"sorted by volume and capped", []KeywordStat{
			{"a", 10, 1}, {"b", 60, 1}, {"c", 30, 3.9}, {"d", 11, 2}, {"e", 12, 2}, {"f", 13, 2}, {"g", 200, 7},
		}, "b,c,f,e,d"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var terms []string
			for _, k := range NoisyKeywords(tc.in) {
				terms = append(terms, k.Term)
			}
			if got := strings.Join(terms, ","); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTemplateSummary(t *testing.T) {
	tests := []struct {
		name string
		d    Data
		want string
	}{
		{"full week", sampleData(), "12 buyers, 4 competitor complaints, 2 noisy keywords, $1,234.50 attributed. 42 mentions, 3 replies posted, 17 clicks."},
		{"singulars and zero", Data{Intents: []Count{{"buy_signal", 1}, {"complaint", 1}}, NoisyKeywords: []KeywordStat{{"x", 10, 1}}, Platforms: []Count{{"reddit", 1}}, RepliesPosted: 1, Clicks: 1},
			"1 buyer, 1 competitor complaint, 1 noisy keyword, $0 attributed. 1 mention, 1 reply posted, 1 click."},
		{"empty", Data{}, "0 buyers, 0 competitor complaints, 0 noisy keywords, $0 attributed. 0 mentions, 0 replies posted, 0 clicks."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := TemplateSummary(tc.d); got != tc.want {
				t.Fatalf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestFormatUSD(t *testing.T) {
	tests := map[int64]string{0: "$0", 99: "$0.99", 100: "$1", 123450: "$1,234.50", 100000000: "$1,000,000", -500: "-$5"}
	for in, want := range tests {
		if got := FormatUSD(in); got != want {
			t.Errorf("FormatUSD(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestEmpty(t *testing.T) {
	if !(Data{}).Empty() {
		t.Fatal("zero data should be empty")
	}
	for name, d := range map[string]Data{
		"mentions": {Platforms: []Count{{"reddit", 1}}},
		"replies":  {RepliesPosted: 1},
		"clicks":   {Clicks: 1},
		"revenue":  {RevenueCents: 1},
	} {
		if d.Empty() {
			t.Errorf("%s: should not be empty", name)
		}
	}
}

func TestSubject(t *testing.T) {
	d := sampleData()
	if got := Subject(d); got != "LeadEcho weekly digest: 12 buyers, $1,234.50 attributed" {
		t.Fatalf("weekly subject %q", got)
	}
	d.Kind = KindDaily
	if got := Subject(d); got != "LeadEcho daily digest: 12 buyers, 42 mentions" {
		t.Fatalf("daily subject %q", got)
	}
}

func TestFacts(t *testing.T) {
	f := Facts(sampleData())
	for _, want := range []string{"buyers: 12", "competitor complaints: 4", "noisy keywords: crm, ai", "attributed revenue: $1,234.50", "reddit: 30", "Looking for a"} {
		if !strings.Contains(f, want) {
			t.Errorf("facts missing %q:\n%s", want, f)
		}
	}
}
