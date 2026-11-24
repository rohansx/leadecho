package reply

import "testing"

func TestWantsTrackedLink(t *testing.T) {
	const dest = "https://www.cloakpipe.co/pricing"
	tests := []struct {
		name    string
		text    string
		dest    string
		linksOn bool
		want    bool
	}{
		{"links on, any draft", "Try a proxy that strips PII.", dest, true, true},
		{"links on, no destination", "Try cloakpipe.co", "", true, false},
		{"links off, plain draft", "Try a proxy that strips PII.", dest, false, false},
		{"links off, names product only", "Cloakpipe does this.", dest, false, false},
		{"links off, links the site", "We built https://cloakpipe.co/docs for this.", dest, false, true},
		{"links off, bare host", "see cloakpipe.co", dest, false, true},
		{"links off, other site", "see https://example.com", dest, false, false},
		{"links off, lookalike host", "see notcloakpipe.co.evil.io", dest, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wantsTrackedLink(tt.text, tt.dest, tt.linksOn); got != tt.want {
				t.Fatalf("wantsTrackedLink = %v, want %v", got, tt.want)
			}
		})
	}
}
