package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"leadecho/internal/api/middleware"
	"leadecho/internal/auth"
	"leadecho/internal/database"
)

type fakeSearcher struct {
	ws, q string
	calls int
}

func (f *fakeSearcher) SearchPeople(_ context.Context, ws, q string, limit int32) ([]database.SearchPerson, error) {
	f.ws, f.q = ws, q
	f.calls++
	return []database.SearchPerson{{PersonID: "p1", Name: "Jane Doe", MentionID: "m1"}}, nil
}

func (f *fakeSearcher) SearchPosts(_ context.Context, ws, q string, limit int32) ([]database.SearchPost, error) {
	f.calls++
	return []database.SearchPost{{MentionID: "m2", Title: "CI is slow"}}, nil
}

func searchRequest(q string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/search?q="+url.QueryEscape(q), nil)
	return r.WithContext(middleware.WithClaims(r.Context(), &auth.Claims{UserID: "u1", WorkspaceID: "ws-1"}))
}

func TestSearchScopesToWorkspaceAndGroupsResults(t *testing.T) {
	f := &fakeSearcher{}
	rec := httptest.NewRecorder()
	NewSearchHandler(f).Search(rec, searchRequest("  jane  "))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if f.ws != "ws-1" || f.q != "jane" {
		t.Errorf("searched workspace %q for %q, want ws-1 / jane", f.ws, f.q)
	}
	var got SearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.People) != 1 || got.People[0].MentionID != "m1" || len(got.Posts) != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestSearchRejectsTooShortOrTooLong(t *testing.T) {
	long := make([]byte, 101)
	for i := range long {
		long[i] = 'a'
	}
	for _, q := range []string{"", "a", " a ", string(long)} {
		f := &fakeSearcher{}
		rec := httptest.NewRecorder()
		NewSearchHandler(f).Search(rec, searchRequest(q))
		if rec.Code != http.StatusBadRequest || f.calls != 0 {
			t.Errorf("q=%q: status %d, %d queries; want 400 and none", q, rec.Code, f.calls)
		}
	}
}
