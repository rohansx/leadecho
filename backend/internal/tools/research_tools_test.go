package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"leadecho/internal/treg"
)

type fakeResearcher struct {
	body string
	err  error
	got  treg.Request
}

func (f *fakeResearcher) Enabled() bool { return true }
func (f *fakeResearcher) Call(_ context.Context, req treg.Request) (*treg.Response, error) {
	f.got = req
	if f.err != nil {
		return nil, f.err
	}
	return &treg.Response{Status: 200, Body: []byte(f.body)}, nil
}

// Shapes captured from live calls (2026-11-11).
const webBody = `{"output":{"results":[
 {"snippet":"Utkrusht has assessed over 11,000 candidates  from customers across India, Europe, and USA.","source":"gg","title":"Candidates are faking their way through Technical Hiring","url":"https://www.siliconindia.com/news/x.html"},
 {"snippet":"bad link","title":"script","url":"javascript:alert(1)"},
 {"snippet":"","title":"No snippet","url":"http://example.com/a"}
]}}`

const jobsBodyResearch = `{"output":{"data":{"items":[
 {"company":"University of the Incarnate Word","title":"Systems Analyst/Programmer","createdUtc":1000,"url":"https://www.linkedin.com/jobs/view/1"},
 {"company":"Vista Higher Learning","title":"NetSuite Systems Administrator","location":"Boston, MA","createdUtc":1000,"url":"https://www.linkedin.com/jobs/view/2"},
 {"company":"Vista Higher Learning, Inc.","title":"Sr. Financial Analyst","createdUtc":1000,"url":"ftp://nope"},
 {"company":"Vista Higher Learning Foundation","title":"Not the same employer","createdUtc":1000,"url":"https://x"}
]}}}`

func TestWebSearchTool(t *testing.T) {
	f := &fakeResearcher{body: webBody}
	r := New(Deps{}).WithResearch(Deps{Research: f})
	res, err := r.Call(context.Background(), "ws", []string{"read"}, "web_search", json.RawMessage(`{"query":" Utkrusht customers "}`))
	if err != nil {
		t.Fatal(err)
	}
	got := res.(map[string]any)["results"].([]WebResult)
	if len(got) != 2 || got[0].URL != "https://www.siliconindia.com/news/x.html" || got[1].URL != "http://example.com/a" {
		t.Fatalf("want the two http(s) results, got %+v", got)
	}
	if f.got.Tool != webSearchTool || f.got.WorkspaceID != "ws" {
		t.Fatalf("call = %+v", f.got)
	}
	if body := f.got.Body.(map[string]any); body["q"] != "Utkrusht customers" {
		t.Fatalf("query not trimmed: %+v", body)
	}
}

func TestCompanyOpenJobsKeepsOnlyThatCompany(t *testing.T) {
	now := time.Unix(1000+3*86400, 0)
	jobs := parseCompanyJobs([]byte(jobsBodyResearch), "Vista Higher Learning", now)
	if len(jobs) != 2 {
		t.Fatalf("want the 2 postings by this employer, got %+v", jobs)
	}
	if jobs[0].Title != "NetSuite Systems Administrator" || jobs[0].PostedDaysAgo != 3 || jobs[0].Location != "Boston, MA" {
		t.Fatalf("job not mapped: %+v", jobs[0])
	}
	if jobs[1].URL != "" {
		t.Fatalf("a non-https link must be dropped, got %q", jobs[1].URL)
	}
	if got := parseCompanyJobs([]byte(`not json`), "x", now); len(got) != 0 {
		t.Fatalf("bad json: %+v", got)
	}
}

func TestResearchToolsUnavailableOrFailing(t *testing.T) {
	// The public registry does not carry them at all.
	if _, err := New(Deps{}).Call(context.Background(), "ws", []string{"read"}, "web_search", json.RawMessage(`{"query":"x y"}`)); !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("public registry must not expose web_search, got %v", err)
	}
	// No provider configured: a clear, safe message.
	r := New(Deps{}).WithResearch(Deps{})
	_, err := r.Call(context.Background(), "ws", []string{"read"}, "company_open_jobs", json.RawMessage(`{"company":"Acme"}`))
	var ue *Error
	if !errors.As(err, &ue) {
		t.Fatalf("want a user-facing error, got %v", err)
	}
	// A provider failure never leaks provider detail.
	f := &fakeResearcher{err: errors.New("treg: 429 secret-internal-detail")}
	_, err = New(Deps{}).WithResearch(Deps{Research: f}).Call(context.Background(), "ws", []string{"read"}, "web_search", json.RawMessage(`{"query":"acme"}`))
	if !errors.As(err, &ue) || ue.Error() == "" || containsStr(ue.Error(), "secret-internal-detail") {
		t.Fatalf("provider error leaked or not user-facing: %v", err)
	}
}

func TestCompanyNameKey(t *testing.T) {
	same := [][2]string{{"Vista Higher Learning, Inc.", "vista higher learning"}, {"LVT (LiveView Technologies)", "LVT"}, {"Two Barrels LLC", "Two Barrels"}}
	for _, p := range same {
		if companyNameKey(p[0]) != companyNameKey(p[1]) {
			t.Errorf("%q and %q should be the same employer", p[0], p[1])
		}
	}
	if companyNameKey("Acme") == companyNameKey("Acme Health") || companyNameKey("") != "" {
		t.Error("different names must not match; empty stays empty")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
