package billing

import (
	"context"
	"reflect"
	"testing"
)

func TestLockedFor(t *testing.T) {
	all := map[string]bool{"twitter": true, "linkedin": true, "reddit": true}
	cases := []struct {
		plan     string
		paidData bool
		want     map[string]bool
	}{
		{"free", false, all},
		{"solo", true, map[string]bool{"twitter": true, "linkedin": true}},
		{"solo", false, all}, // ops switched paid data off
		{"starter", true, map[string]bool{}},
		{"pro", true, map[string]bool{}},
		{"starter", false, all},
	}
	for _, c := range cases {
		if got := LockedFor(c.plan, c.paidData); !reflect.DeepEqual(got, c.want) {
			t.Errorf("LockedFor(%s, %v) = %v, want %v", c.plan, c.paidData, got, c.want)
		}
	}
	// The returned map is a copy: changing it must not unlock anything.
	LockedFor("free", false)["reddit"] = false
	if !PaidPlatforms["reddit"] {
		t.Fatal("LockedFor leaked PaidPlatforms")
	}
}

func TestLockedPlatformsSelfHost(t *testing.T) {
	if got := New(nil, false).LockedPlatforms(context.Background(), "ws"); got != nil {
		t.Fatalf("self-host locks nothing, got %v", got)
	}
	var c *Credits
	if got := c.LockedPlatforms(context.Background(), "ws"); got != nil {
		t.Fatalf("nil credits lock nothing, got %v", got)
	}
}
