package billing

import "testing"

func TestProspectDaily(t *testing.T) {
	cases := []struct {
		plan     string
		enforced bool
		stored   []byte
		want     bool
	}{
		{"pro", true, nil, true},
		{"pro", true, []byte("true"), true},
		{"pro", true, []byte("false"), false},
		{"starter", true, nil, false},
		{"starter", true, []byte("true"), false},
		{"free", true, nil, false},
		{"free", false, nil, true}, // self-host: credits not enforced
		{"free", false, []byte("false"), false},
	}
	for _, c := range cases {
		if got := ProspectDaily(c.plan, c.enforced, c.stored); got != c.want {
			t.Errorf("ProspectDaily(%q, %v, %q) = %v, want %v", c.plan, c.enforced, c.stored, got, c.want)
		}
	}
}
