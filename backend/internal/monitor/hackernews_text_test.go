package monitor

import "testing"

func TestCleanHNText(t *testing.T) {
	in := `what did everyone migrate to?<p>I don&#x27;t care about <i>MCP</i>, see <a href="https://x.io">x.io</a>`
	want := "what did everyone migrate to?\n\nI don't care about MCP, see x.io"
	if got := cleanHNText(in); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}
