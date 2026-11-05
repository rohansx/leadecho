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
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 0: // 0.0.0.0/8 ("this network", reaches localhost on Linux)
			return true
		case v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127: // CGNAT 100.64.0.0/10
			return true
		case v4[0] == 198 && (v4[1] == 18 || v4[1] == 19): // benchmarking 198.18.0.0/15
			return true
		}
		return false
	}
	// NAT64 64:ff9b::/96 embeds an IPv4 address: judge the embedded one.
	if len(ip) == net.IPv6len && ip[0] == 0x00 && ip[1] == 0x64 && ip[2] == 0xff && ip[3] == 0x9b &&
		string(ip[4:12]) == string(make([]byte, 8)) {
		return DisallowedIP(net.IPv4(ip[12], ip[13], ip[14], ip[15]))
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
