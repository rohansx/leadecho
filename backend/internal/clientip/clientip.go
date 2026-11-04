// Package clientip resolves the caller's IP without trusting spoofable headers.
package clientip

import (
	"net"
	"net/http"
	"strings"
)

// From returns the client IP for r. trustedHops is how many reverse proxies we
// control sit in front of the app (0 = none, 1 = behind Traefik/nginx). Each
// proxy appends the address it saw to X-Forwarded-For, so the entry trustedHops
// from the right is the last one a proxy we trust wrote; anything left of it
// is client-controlled. With 0 hops, or a header too short, RemoteAddr is used.
func From(r *http.Request, trustedHops int) string {
	if trustedHops > 0 {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if i := len(parts) - trustedHops; i >= 0 {
				if ip := strings.TrimSpace(parts[i]); net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
