package ai

import (
	"reflect"
	"testing"
)

// LinkedIn is opt-in: the priciest paid source and, measured on CloakPipe,
// 0 useful leads in 154 posts. Suggestions never switch it on by default.
func TestDefaultPlatforms(t *testing.T) {
	got := defaultPlatforms([]string{"reddit", "linkedin", "twitter", "LinkedIn", "hackernews"})
	if want := []string{"reddit", "twitter", "hackernews"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := defaultPlatforms([]string{"linkedin"}); len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}
