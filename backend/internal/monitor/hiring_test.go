package monitor

import (
	"testing"

	"leadecho/internal/ai"
)

// Shapes below are trimmed from live treg responses (2026-11-11).
const jobsBody = `{"output":{"data":{"items":[
 {"company":"Alaffia Health","companyUrl":"https://www.linkedin.com/company/alaffia-health","createdUtc":100,"id":"1","location":"New York, NY","title":"Senior Software Engineer (Backend)","url":"https://www.linkedin.com/jobs/view/1"},
 {"company":"Ivo","companyUrl":"https://www.linkedin.com/company/ivo","createdUtc":300,"id":"2","location":"San Francisco, CA","title":"Senior Software Engineer, Backend","url":"https://www.linkedin.com/jobs/view/2"},
 {"company":"Alaffia  Health","createdUtc":200,"id":"3","title":"Staff Engineer","url":"https://www.linkedin.com/jobs/view/3"},
 {"company":"","title":"no company","url":"https://x"},
 {"company":"NoURL Inc","title":"Engineer"}
],"nextCursor":"abc"}}}`

func TestParseJobsAndOnePerCompany(t *testing.T) {
	jobs := onePerCompany(parseJobs([]byte(jobsBody)))
	if len(jobs) != 2 {
		t.Fatalf("want 2 companies, got %d: %+v", len(jobs), jobs)
	}
	if jobs[0].Company != "Ivo" { // newest first
		t.Fatalf("newest company first, got %q", jobs[0].Company)
	}
	if jobs[1].ID != "3" { // the company's newest job wins
		t.Fatalf("want Alaffia's newest job (id 3), got %q", jobs[1].ID)
	}
	if got := parseJobs([]byte(`{"data":{"items":[{"company":"A","url":"u"}]}}`)); len(got) != 1 {
		t.Fatalf("unwrapped body: got %d", len(got))
	}
	if parseJobs([]byte(`not json`)) != nil {
		t.Fatal("bad json should give no jobs")
	}
}

func TestPickBuyer(t *testing.T) {
	people := parsePeople([]byte(`{"output":{"people":[
	 {"name":"Kai A","title":"Co-Founder & CTO","company":"Anon","profile_url":"https://linkedin.com/in/kai"},
	 {"name":"No Profile","title":"CTO","company":"Alaffia Health","profile_url":""},
	 {"name":"Chien Kuo","title":"VP of Engineering","company":"Alaffia Health","profile_url":"https://linkedin.com/in/chienkuo"},
	 {"name":"Edward R","title":"Senior Engineering Manager","company":"Alaffia Health","profile_url":"https://linkedin.com/in/edward"}
	]}}`))
	got, ok := pickBuyer(people, "Alaffia Health")
	if !ok || got.Name != "Chien Kuo" {
		t.Fatalf("want the first current employee with a profile, got %+v ok=%v", got, ok)
	}
	// "LVT (LiveView Technologies)" in a job post vs "LVT" on a profile.
	if _, ok := pickBuyer([]foundPerson{{Name: "A", Title: "CTO", Company: "LVT", ProfileURL: "https://linkedin.com/in/a"}}, "LVT (LiveView Technologies)"); !ok {
		t.Fatal("company name with a suffix should still match")
	}
	// An individual contributor is not a buyer, even at the right company.
	if _, ok := pickBuyer([]foundPerson{{Name: "Alex", Title: "Frontend Web Developer", Company: "Two Barrels LLC", ProfileURL: "https://linkedin.com/in/alex"}}, "Two Barrels LLC"); ok {
		t.Fatal("a developer must not be picked as the buyer")
	}
	for _, title := range []string{"Senior Technical Recruiter", "Co-Founder & CTO", "Head of Talent", "Engineering Manager", "Chief of Staff"} {
		if _, ok := pickBuyer([]foundPerson{{Name: "A", Title: title, Company: "Acme", ProfileURL: "https://linkedin.com/in/a"}}, "Acme"); !ok {
			t.Fatalf("%q should count as a buyer title", title)
		}
	}
	if _, ok := pickBuyer(people, "Ivo"); ok {
		t.Fatal("nobody works at Ivo in this list")
	}
	if _, ok := pickBuyer(nil, "Ivo"); ok {
		t.Fatal("empty result must not match")
	}
}

func TestURLChecks(t *testing.T) {
	for _, bad := range []string{"", "javascript:alert(1)//linkedin.com/in/a", "https://evil.test/?linkedin.com/in/a", "http://linkedin.com/in/a", "https://linkedin.com/company/acme"} {
		if linkedInProfileURL(bad) {
			t.Errorf("%q must not pass as a LinkedIn profile", bad)
		}
	}
	for _, good := range []string{"https://linkedin.com/in/chienkuo", "https://www.linkedin.com/in/jane-doe"} {
		if !linkedInProfileURL(good) {
			t.Errorf("%q should pass", good)
		}
	}
	if validHTTPURL("javascript:alert(1)") || validHTTPURL("/jobs/1") || !validHTTPURL("https://www.linkedin.com/jobs/view/1") {
		t.Error("validHTTPURL accepts only absolute http(s) URLs")
	}
}

func TestParseCompanySize(t *testing.T) {
	cases := map[string]int{
		`{"output":{"name":"Acme","employees":"51-200"}}`: 51,
		`{"output":{"employees":10210}}`:                  10210,
		`{"employees":"10k+"}`:                            10000,
		`{"output":{"name":"Unknown Co"}}`:                0,
		`not json`:                                        0,
	}
	for body, want := range cases {
		if got := parseCompanySize([]byte(body)); got != want {
			t.Errorf("%s: got %d, want %d", body, got, want)
		}
	}
}

func TestHiringPlatformID(t *testing.T) {
	a := hiringPlatformID("Alaffia Health", "2026-11")
	if a != "hiring-alaffia-health-2026-11" {
		t.Fatalf("got %q", a)
	}
	if hiringPlatformID("ALAFFIA  health!", "2026-11") != a {
		t.Fatal("same company must give the same id")
	}
	if hiringPlatformID("Alaffia Health", "2026-12") == a {
		t.Fatal("a new month is a new signal")
	}
}

func TestNormalizeHiringPlan(t *testing.T) {
	on := ai.NormalizeHiringPlan(ai.HiringPlan{Applies: true, Roles: []string{" software engineer ", "", "a", "b", "c"}, BuyerTitles: "CTO or VP Engineering"})
	if !on.Applies || len(on.Roles) != 3 || on.Roles[0] != "software engineer" {
		t.Fatalf("got %+v", on)
	}
	for _, bad := range []ai.HiringPlan{
		{Applies: true, Roles: nil, BuyerTitles: "CTO"},
		{Applies: true, Roles: []string{"engineer"}, BuyerTitles: ""},
		{Applies: false, Roles: []string{"engineer"}, BuyerTitles: "CTO"},
	} {
		if got := ai.NormalizeHiringPlan(bad); got.Applies || len(got.Roles) != 0 || got.BuyerTitles != "" {
			t.Fatalf("unusable plan must be off and empty: %+v", got)
		}
	}
}
