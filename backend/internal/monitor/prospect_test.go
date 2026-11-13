package monitor

import (
	"reflect"
	"testing"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/database"
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
	q := segmentQueries(ai.CustomerSegment{Name: "SME accountancy firms", Criteria: []string{"5-30 person practice", "Partner-led"}})
	want := []string{
		"SME accountancy firms: 5-30 person practice; Partner-led",
		"SME accountancy firms: 5-30 person practice",
		"SME accountancy firms: Partner-led",
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
