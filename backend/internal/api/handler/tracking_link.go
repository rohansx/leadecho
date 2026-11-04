package handler

import (
	"net/url"
	"regexp"
	"strings"
)

// withTrackingLink puts the tracked short link into a reply only where the
// product belongs: it replaces a URL pointing at the destination's host, or
// appends the link when the reply names the product. A reply that never
// mentions the product is left alone — a bare link there reads as spam.
func withTrackingLink(text, destinationURL, short string) (string, bool) {
	u, err := url.Parse(destinationURL)
	if err != nil || u.Hostname() == "" {
		return text, false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")

	hostRE := regexp.MustCompile(`(?i)\b(?:https?://)?(?:www\.)?` + regexp.QuoteMeta(host) + `\b(?:/[^\s)\]>]*[^\s)\]>.,!?;:])?/?`)
	if hostRE.MatchString(text) {
		return hostRE.ReplaceAllLiteralString(text, short), true
	}

	// ponytail: brand = first host label ("cloakpipe" for cloakpipe.co). Misses
	// products whose name differs from their domain; add a product-name setting then.
	brand := strings.SplitN(host, ".", 2)[0]
	if len(brand) >= 3 && strings.Contains(strings.ToLower(text), brand) {
		return strings.TrimRight(text, "\n") + "\n\n" + short, true
	}
	return text, false
}
