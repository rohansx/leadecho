package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Input validation happens before any DB access, so a nil service is fine here.
func TestDigestHandlerValidation(t *testing.T) {
	h := NewDigestHandler(nil)
	tests := []struct {
		name    string
		method  string
		target  string
		body    string
		handler http.HandlerFunc
		want    int
		wantMsg string
	}{
		{"preview bad kind", http.MethodPost, "/api/v1/settings/digests/preview?kind=monthly", "", h.Preview, 400, "kind"},
		{"put bad json", http.MethodPut, "/api/v1/settings/digests", "{", h.Put, 400, "invalid request body"},
		{"put bad time", http.MethodPut, "/api/v1/settings/digests", `{"daily_time":"7pm","weekly_day":"monday"}`, h.Put, 400, "daily_time"},
		{"put bad channel", http.MethodPut, "/api/v1/settings/digests", `{"daily_time":"09:30","weekly_day":"monday","channels":["sms"]}`, h.Put, 400, "channel"},
		{"put oversized body", http.MethodPut, "/api/v1/settings/digests", `{"timezone":"` + strings.Repeat("x", 70000) + `"}`, h.Put, 400, "invalid request body"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.handler(rec, httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body)))
			if rec.Code != tc.want || !strings.Contains(rec.Body.String(), tc.wantMsg) {
				t.Fatalf("got %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
