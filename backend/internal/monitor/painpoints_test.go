package monitor

import (
	"reflect"
	"testing"
)

func TestBrandPainPhrases(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"brand profile rows (real shape)", `[{"problem": "prompt injection tricking agents", "explanation": ""}, {"problem": "  over-permissioned agents ", "explanation": "x"}]`,
			[]string{"prompt injection tricking agents", "over-permissioned agents"}},
		{"legacy plain strings", `["slow onboarding", ""]`, []string{"slow onboarding"}},
		{"empty", `[]`, nil},
		{"null", ``, nil},
		{"garbage", `{"nope":1}`, nil},
	}
	for _, c := range cases {
		if got := brandPainPhrases([]byte(c.raw)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %#v, want %#v", c.name, got, c.want)
		}
	}
}
