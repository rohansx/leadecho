package clientip

import (
	"net/http/httptest"
	"testing"
)

func TestFrom(t *testing.T) {
	cases := []struct {
		name string
		xff  string
		hops int
		want string
	}{
		{"no proxy ignores spoofed XFF", "1.2.3.4", 0, "10.0.0.9"},
		{"one proxy takes rightmost", "6.6.6.6, 203.0.113.7", 1, "203.0.113.7"},
		{"one proxy ignores client-prepended entries", "1.1.1.1, 2.2.2.2, 203.0.113.7", 1, "203.0.113.7"},
		{"two proxies", "203.0.113.7, 172.16.0.2", 2, "203.0.113.7"},
		{"header shorter than hops falls back", "203.0.113.7", 2, "10.0.0.9"},
		{"garbage entry falls back", "not-an-ip", 1, "10.0.0.9"},
		{"no header falls back", "", 1, "10.0.0.9"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "10.0.0.9:5555"
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		r.Header.Set("X-Real-IP", "9.9.9.9") // never trusted
		if got := From(r, c.hops); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}
