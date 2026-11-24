package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

type fakeMarker struct {
	marked []string
	err    error
}

func (f *fakeMarker) MarkFeatureUsed(_ context.Context, ws, feature string) error {
	f.marked = append(f.marked, ws+"/"+feature)
	return f.err
}

func TestFeatureUseMarksAndServes(t *testing.T) {
	for _, markErr := range []error{nil, errors.New("db down")} {
		m := &fakeMarker{err: markErr}
		served := false
		h := FeatureUse(m, "demand", zerolog.Nop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			served = true
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/demand", nil))
		if len(m.marked) != 1 || m.marked[0] != WorkspaceID(context.Background())+"/demand" {
			t.Fatalf("marked %v", m.marked)
		}
		if !served {
			t.Fatalf("request not served when mark returned %v", markErr)
		}
	}
}
