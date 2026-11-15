package billing

import "testing"

func TestProspectDaily(t *testing.T) {
	cases := []struct {
		plan   string
		stored []byte
		want   bool
	}{
		{"pro", nil, true},
		{"pro", []byte("true"), true},
		{"pro", []byte("false"), false},
		{"starter", nil, false},
		{"starter", []byte("true"), false},
		{"free", nil, false},
	}
	for _, c := range cases {
		if got := ProspectDaily(c.plan, c.stored); got != c.want {
			t.Errorf("ProspectDaily(%q, %q) = %v, want %v", c.plan, c.stored, got, c.want)
		}
	}
}
