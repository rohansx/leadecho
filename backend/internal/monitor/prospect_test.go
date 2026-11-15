package monitor

import (
	"reflect"
	"testing"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/database"
	"leadecho/internal/enrich"
)

func brandProfileWithSegments(raw string) database.BrandProfile {
	return database.BrandProfile{Product: "P", CustomerSegments: []byte(raw)}
}

// Rows come back in each provider's own shape: the parser must take the
// fields from whichever key that provider uses.
const companiesBody = `{"output":{"companies":[
 {"name":"Racz Group","domain":"raczgroup.co.uk","description":"Franchise developer","employees":60,"country":"GB"},
 {"company_name":"JRK Group","website":"https://www.jrk.group/about","short_description":"Wendy's franchisee","employee_count":"11-50"},
 {"name":"Dupe","website_url":"http://eu.jrk.group"},
 {"name":"Only LinkedIn","website":"https://www.linkedin.com/company/x"},
 {"name":"No site"},
 {"domain":"noname.com"}
]}}`

func TestParseCompanies(t *testing.T) {
	got := parseCompanies([]byte(companiesBody))
	want := []ai.ProspectCandidate{
		{Name: "Racz Group", Domain: "raczgroup.co.uk", Description: "Franchise developer", Employees: 60, Country: "GB"},
		{Name: "JRK Group", Domain: "jrk.group", Description: "Wendy's franchisee", Employees: 11},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCompanies:\n got %+v\nwant %+v", got, want)
	}
	if parseCompanies([]byte(`{"companies":[{"name":"A","domain":"a.io"}]}`))[0].Domain != "a.io" {
		t.Fatal("unwrapped body not read")
	}
	if parseCompanies([]byte(`not json`)) != nil {
		t.Fatal("bad body should give nothing")
	}
	// Hunter, as the router served it live on 2026-11-15: the name is "organization".
	if got := parseCompanies([]byte(`{"output":{"companies":[{"domain":"polymarket.com","organization":"Polymarket","emails_count":{"total":202}}]}}`)); len(got) != 1 || got[0].Name != "Polymarket" {
		t.Fatalf("hunter row: %+v", got)
	}
}

// Exa's shape, trimmed from a live exa.companies.search answer (2026-11-15).
func TestParseCompaniesExa(t *testing.T) {
	body := `{"requestId":"x","results":[
	 {"id":"e1","title":"Swipe (YC S21)","url":"https://getswipe.in/","entities":[{"type":"company","properties":{
	   "name":"Swipe","description":"GST billing app for Indian SMBs.","workforce":{"total":85},
	   "headquarters":{"city":"Hyderabad","country":"India"}}}]},
	 {"id":"e2","title":"Bare","url":"https://bare.io/"}
	]}`
	got := parseCompanies([]byte(body))
	want := []ai.ProspectCandidate{
		{Name: "Swipe", Domain: "getswipe.in", Description: "GST billing app for Indian SMBs.", Employees: 85, Country: "India"},
		{Name: "Bare", Domain: "bare.io"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exa:\n got %+v\nwant %+v", got, want)
	}
	wrapped := parseCompanies([]byte(`{"output":{"results":[{"url":"https://a.io","title":"A"}]}}`))
	if len(wrapped) != 1 || wrapped[0].Domain != "a.io" {
		t.Fatalf("wrapped exa: %+v", wrapped)
	}
	if countryName("in") != "India" || countryName("ZZ") != "ZZ" {
		t.Fatal("countryName")
	}
}

func TestCompanyDomain(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.eu.acme.co.uk/about": "acme.co.uk",
		"ACME.io":                         "acme.io",
		"crunchbase.com/organization/x":   "",
		"localhost":                       "",
		"":                                "",
	} {
		if got := companyDomain(in); got != want {
			t.Errorf("companyDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewProspects(t *testing.T) {
	in := []ai.ProspectCandidate{
		{Name: "Seen Co", Domain: "seen.com"},
		{Name: "PaperHQ", Domain: "paperhq.ai"},
		{Name: "Dext Ltd", Domain: "dext.com"},
		{Name: "Ledgers", Domain: "ledgers.work"},
		{Name: "Fits Co", Domain: "fits.co.uk"},
	}
	got := newProspects(in, map[string]bool{"seen.com": true}, "https://www.paperhq.ai/", []string{"Dext", "ledgers.work"})
	if len(got) != 1 || got[0].Domain != "fits.co.uk" {
		t.Fatalf("newProspects = %+v, want only fits.co.uk", got)
	}
}

func TestSegmentQueriesRotate(t *testing.T) {
	q := segmentQueries(ai.CustomerSegment{Name: "Technical recruiters", Criteria: []string{"accountancy practice with 5-30 staff", "Partner-led"},
		ExampleCompanies: []string{"Ryan & Co"}})
	want := []string{ // never the segment name: it names the buyer, not the company
		"accountancy practice with 5-30 staff; Partner-led",
		"accountancy practice with 5-30 staff; Partner-led; similar to Ryan & Co",
	}
	if !reflect.DeepEqual(q, want) {
		t.Fatalf("segmentQueries = %q", q)
	}
}

func TestProspectableSegments(t *testing.T) {
	bp := brandProfileWithSegments(`[
	 {"name":"Old style","why_they_buy":"x"},
	 {"name":"Full","criteria":["a"],"buyer_titles":"Partner"},
	 {"name":"No titles","criteria":["a"]}
	]`)
	got := prospectableSegments(bp)
	if len(got) != 1 || got[0].Name != "Full" {
		t.Fatalf("prospectableSegments = %+v", got)
	}
	if prospectableSegments(brandProfileWithSegments(`oops`)) != nil {
		t.Fatal("unreadable segments should give none")
	}
}

func TestNewManualScanAcceptsProspects(t *testing.T) {
	if _, err := NewManualScan(ScanProspects, "op", time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestRankByTitles(t *testing.T) {
	people := []foundPerson{
		{Name: "R", Title: "Senior Talent Partner"},
		{Name: "X", Title: "Office Manager"},
		{Name: "E", Title: "Vice President of Engineering"},
	}
	if r := rankByTitles([]foundPerson{{Name: "D", Title: "Director of Marketing"}, {Name: "C", Title: "CTO."}}, "CTO or VP Engineering"); r[0].Name != "C" {
		t.Fatalf("cto must not match director: %+v", r)
	}
	got := rankByTitles(people, "VP Engineering or Director of Talent Acquisition, Head of Talent")
	if got[0].Name != "E" || got[1].Name != "R" || got[2].Name != "X" || people[0].Name != "R" {
		t.Fatalf("rankByTitles = %+v (input must not change)", got)
	}
}

func TestConfirmProspectContactRejectsCatchAll(t *testing.T) {
	co := hiringCompany{domain: "credgenics.com", size: 900}
	p := foundPerson{Name: "S R"}
	res := &enrich.Result{FullName: "S R", Company: "Credgenics", CompanyDomain: "credgenics.com", Email: "s@credgenics.com", EmailStatus: "catchall"}
	if ck := confirmProspectContact("Credgenics", co, p, res); ck.confirmed {
		t.Fatal("a catch-all email must not confirm a prospect")
	}
	ok := *res
	ok.EmailStatus = "verified"
	if ck := confirmProspectContact("Credgenics", co, p, &ok); !ck.confirmed {
		t.Fatalf("verified email should confirm: %+v", ck)
	}
}
