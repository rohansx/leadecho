package handler

import (
	"net/http/httptest"
	"testing"
)

func TestParseLimit(t *testing.T) {
	for q, want := range map[string]int32{"": 20, "abc": 20, "-5": 20, "0": 20, "50": 50, "100": 100, "1000": 100} {
		r := httptest.NewRequest("GET", "/x?limit="+q, nil)
		if got := parseLimit(r, 20, 100); got != want {
			t.Errorf("limit=%q: got %d want %d", q, got, want)
		}
	}
}
