package monitor

import "testing"

// The settings page asks for just the cookie *value*; a bare value must
// become the platform's login cookie, not be dropped (Test sent 0 cookies).
func TestParseCookieString(t *testing.T) {
	cases := []struct {
		in, domain, wantName, wantValue string
		n                               int
	}{
		{"eyJhbGciOi.payload.sig", ".reddit.com", "reddit_session", "eyJhbGciOi.payload.sig", 1},
		{"  abc123  ", ".linkedin.com", "li_at", "abc123", 1},
		{"reddit_session=v1; token_v2=v2", ".reddit.com", "reddit_session", "v1", 2},
		{"", ".reddit.com", "", "", 0},
	}
	for _, c := range cases {
		got := ParseCookieString(c.in, c.domain)
		if len(got) != c.n {
			t.Fatalf("ParseCookieString(%q) = %d cookies, want %d", c.in, len(got), c.n)
		}
		if c.n > 0 && (got[0].Name != c.wantName || got[0].Value != c.wantValue || got[0].Domain != c.domain) {
			t.Errorf("ParseCookieString(%q)[0] = %+v", c.in, got[0])
		}
	}
}
