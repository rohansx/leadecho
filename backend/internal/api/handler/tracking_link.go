package handler

import "leadecho/internal/reply"

// withTrackingLink — see reply.WithTrackingLink (moved there so the drafter
// can link replies at draft time).
func withTrackingLink(text, destinationURL, short string) (string, bool) {
	return reply.WithTrackingLink(text, destinationURL, short)
}
