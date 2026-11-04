package handler

import "testing"

func TestWithTrackingLink(t *testing.T) {
	const dest, short = "https://www.cloakpipe.co/pricing", "https://le.test/r/abc"
	cases := []struct {
		name, in, want string
		ok             bool
	}{
		{"no mention", "Use scoped tokens and an approval queue.", "Use scoped tokens and an approval queue.", false},
		{"names product", "CloakPipe does this.", "CloakPipe does this.\n\n" + short, true},
		{"replaces url", "See https://cloakpipe.co/docs.", "See " + short + ".", true},
		{"replaces bare domain", "Try cloakpipe.co today", "Try " + short + " today", true},
		{"other domain untouched", "see cloakpipe.com", "see cloakpipe.com\n\n" + short, true},
	}
	for _, c := range cases {
		got, ok := withTrackingLink(c.in, dest, short)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}
