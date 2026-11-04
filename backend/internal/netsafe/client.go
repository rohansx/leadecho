// Package netsafe provides an HTTP client for user-supplied URLs that refuses
// to reach internal addresses (SSRF guard).
package netsafe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// DisallowedIP blocks loopback/private/link-local/unspecified and CGNAT ranges.
func DisallowedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// CGNAT 100.64.0.0/10
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
}

// Client resolves the target itself and refuses to connect to any
// internal address (re-checked at dial time to defeat DNS rebinding), and
// disallows redirects.
func Client(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("redirects are not allowed")
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
				if err != nil || len(ips) == 0 {
					return nil, fmt.Errorf("dns lookup failed for %s", host)
				}
				for _, ip := range ips {
					if DisallowedIP(ip.IP) {
						return nil, fmt.Errorf("blocked internal address: %s", ip.IP)
					}
				}
				// Dial the already-validated IP to avoid a TOCTOU re-resolution.
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
			},
		},
	}
}
