package researcher

import "testing"

func TestExtractGitHubHandle(t *testing.T) {
	tests := []struct {
		profileURL string
		content    string
		author     string
		want       string
	}{
		{"https://github.com/octocat", "", "", "octocat"},
		{"", "my repo is at github.com/devuser/cool", "", "devuser"},
		{"", "", "hn_user", "hn_user"},
		{"https://reddit.com/u/foo", "", "foo", "foo"},
	}
	for _, tt := range tests {
		if got := extractGitHubHandle(tt.profileURL, tt.content, tt.author); got != tt.want {
			t.Errorf("extractGitHubHandle(%q, %q, %q) = %q, want %q",
				tt.profileURL, tt.content, tt.author, got, tt.want)
		}
	}
}
