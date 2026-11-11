package digest

import (
	"strings"
	"testing"
)

// Numbers from the first production scan: 5 "leads" were 3 hiring signals
// and 2 posts. Hiring signals carry a buying intent, so they used to be
// counted as buyers.
func TestHiringSignalsCountedSeparately(t *testing.T) {
	d := Data{
		Kind:            KindDaily,
		Intents:         []Count{{"buy_signal", 9}, {"recommendation_ask", 1}, {"general", 40}},
		HiringConfirmed: 3,
		HiringHeld:      5,
	}
	if got := d.Buyers(); got != 2 {
		t.Fatalf("buyers from posts = %d, want 2 (10 buying-intent mentions minus 8 hiring signals)", got)
	}
	if s := Subject(d); !strings.Contains(s, "2 buyers") || !strings.Contains(s, "3 hiring signals") {
		t.Fatalf("subject = %q", s)
	}
	html, err := RenderHTML(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"buyers from posts", "hiring signals", "(+5 held back)"} {
		if !strings.Contains(html, want) {
			t.Errorf("email is missing %q", want)
		}
	}
	if slack := RenderSlack(d); !strings.Contains(slack, "2 buyers from posts · 3 hiring signals") {
		t.Fatalf("slack = %q", slack)
	}

	// No hiring signals: nothing changes, and the count never goes negative.
	plain := Data{Kind: KindDaily, Intents: []Count{{"buy_signal", 2}}}
	if plain.Buyers() != 2 || strings.Contains(Subject(plain), "hiring") {
		t.Fatalf("plain: buyers=%d subject=%q", plain.Buyers(), Subject(plain))
	}
	if (Data{HiringConfirmed: 4}).Buyers() != 0 {
		t.Fatal("buyers must not go below zero")
	}
}
