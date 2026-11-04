package netsafe

import (
	"context"
	"fmt"
	"net"
	"net/url"
)

// ValidatePublicURL rejects non-http(s) URLs and hosts that resolve to any
// internal address. Use it before handing a user URL to something that does
// its own networking (e.g. a headless-browser sidecar) and so bypasses Client.
// ponytail: check-then-use, so DNS rebinding between this check and the
// sidecar's fetch is still possible; network-isolate the sidecar for a hard guarantee.
func ValidatePublicURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("netsafe: invalid url")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if DisallowedIP(ip) {
			return fmt.Errorf("netsafe: internal address not allowed")
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("netsafe: resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if DisallowedIP(ip.IP) {
			return fmt.Errorf("netsafe: internal address not allowed")
		}
	}
	return nil
}
