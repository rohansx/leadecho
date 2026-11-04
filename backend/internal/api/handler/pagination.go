package handler

import (
	"net/http"
	"strconv"
)

// maxPageSize caps list endpoints.
const maxPageSize = 100

// parseLimit reads ?limit=. Values above max are clamped to max (not ignored:
// silently falling back to the default made "limit=1000" return 20 rows).
// Missing, non-numeric or non-positive values keep def.
func parseLimit(r *http.Request, def, max int32) int32 {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 {
		return def
	}
	if n > int(max) {
		return max
	}
	return int32(n)
}
