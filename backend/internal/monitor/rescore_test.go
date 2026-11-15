package monitor

import (
	"testing"

	"leadecho/internal/database"
)

func TestBatchMoved(t *testing.T) {
	ids := func(xs ...string) []database.RescoreMention {
		out := make([]database.RescoreMention, len(xs))
		for i, x := range xs {
			out[i] = database.RescoreMention{ID: x}
		}
		return out
	}
	cases := []struct {
		name        string
		batch, left []database.RescoreMention
		want        bool
	}{
		{"nothing moved", ids("a", "b", "c"), ids("a", "b", "c"), false},
		// a post that keeps failing stays first, the rest moved and new ones fill in
		{"stuck post on top, rest moved", ids("stuck", "b", "c"), ids("stuck", "d", "e"), true},
		{"all done", ids("a", "b"), nil, true},
	}
	for _, c := range cases {
		if got := batchMoved(c.batch, c.left); got != c.want {
			t.Errorf("%s: batchMoved = %v, want %v", c.name, got, c.want)
		}
	}
}
