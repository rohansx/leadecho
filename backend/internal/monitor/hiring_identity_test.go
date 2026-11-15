package monitor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"leadecho/internal/enrich"
)

// The shapes independent reviewers found in the production inbox, all stored
// as confirmed at the time. People's names are invented.
func TestIdentityGate(t *testing.T) {
	person := foundPerson{Name: "Wei Lin Tan", Title: "CTO", Company: "Toku", ProfileURL: "https://www.linkedin.com/in/weilintan"}
	// A job at a Chilean fintech; the lookup by name answered with a
	// 13-person Singapore staffing firm of the same name, and so did people
	// search. Every field of the contact agrees with the lookup.
	tokuJob := jobPost{Company: "Toku", Title: "Senior Backend Engineer", Location: "Santiago, Santiago Metropolitan Region, Chile",
		CompanyURL: "https://www.linkedin.com/company/trytoku", Source: sourceLinkedIn}
	namesake := hiringCompany{size: 13, domain: "toku.example"}
	tokuContact := func(mut func(*enrich.Result)) *enrich.Result {
		r := &enrich.Result{FullName: "Wei Lin Tan", Title: "CTO", Company: "Toku Pte. Ltd.", CompanyDomain: "toku.example", CompanySize: 13,
			CompanyLinkedInURL: "https://www.linkedin.com/company/toku-staffing/", Location: "Singapore", Email: "weilin@toku.example", EmailStatus: "verified"}
		if mut != nil {
			mut(r)
		}
		return r
	}
	// "Nubank" shown as 4 employees in Bellevue; the job is in Rio.
	nubankJob := jobPost{Company: "Nubank", Title: "Software Engineer", Location: "Rio de Janeiro, Rio de Janeiro, Brazil",
		CompanyURL: "https://www.linkedin.com/company/nubank", Source: sourceLinkedIn}
	nubankContact := &enrich.Result{FullName: "Priya Raman", Title: "Founder", Company: "Nubank", CompanyDomain: "nubank.example", CompanySize: 4,
		Location: "Bellevue, Washington, United States", Email: "priya@nubank.example", EmailStatus: "verified"}
	// A company whose identity is not in doubt, for the email cases.
	acmeJob := jobPost{Company: "Acme Robotics", Title: "Software Engineer", Location: "Austin, TX", CompanyURL: "https://www.linkedin.com/company/acme-robotics", Source: sourceLinkedIn}
	acme := hiringCompany{size: 120, domain: "acmerobotics.example"}
	acmeContact := func(email, status string) *enrich.Result {
		return &enrich.Result{FullName: "Sam Okoye", Title: "VP Engineering", Company: "Acme Robotics, Inc.", CompanyDomain: "acmerobotics.example",
			Location: "Austin, Texas, United States", Email: email, EmailStatus: status}
	}

	cases := []struct {
		name        string
		job         jobPost
		lookup      hiringCompany // what the company lookup by name returned
		contact     *enrich.Result
		confirmed   bool
		reasonHas   string
		mismatchHas []string
		email       string
	}{
		{"toku: the contact's employer is another LinkedIn page", tokuJob, namesake, tokuContact(nil), false, "another company of the same name",
			[]string{`employer "Toku Pte. Ltd." (toku.example) vs job company "Toku"`, `"toku-staffing" vs "trytoku"`}, ""},
		{"toku: no page on the profile, but the contact is in another country than the job", tokuJob, namesake,
			tokuContact(func(r *enrich.Result) { r.CompanyLinkedInURL = "" }), false, "is in SG but the job is in CL", []string{"contact in SG, job in CL"}, ""},
		{"toku: the posting names its own website", jobPost{Company: "Toku", Title: "Senior Backend Engineer", Location: "Santiago, Chile", CompanyURL: "https://trytoku.example", Source: sourceHN},
			namesake, tokuContact(func(r *enrich.Result) { r.CompanyLinkedInURL = "" }), false, "another company of the same name",
			[]string{`employer "Toku Pte. Ltd." (toku.example) vs job company "Toku" (trytoku.example)`}, ""},
		{"nubank: a 4-person namesake in Bellevue for a job in Rio", nubankJob, hiringCompany{size: 4, domain: "nubank.example"}, nubankContact, false,
			"is in US but the job is in BR", []string{`employer "Nubank" (nubank.example) vs job company "Nubank" (nubank.example)`, "contact in US, job in BR"}, ""},
		{"the same page posted the job: confirmed whatever the lookup by name said", tokuJob, hiringCompany{size: 300},
			&enrich.Result{FullName: "Wei Lin Tan", Company: "Toku", CompanyDomain: "trytoku.example", CompanyLinkedInURL: "https://linkedin.com/company/TryToku",
				Location: "Santiago, Chile", Email: "w@trytoku.example", EmailStatus: "verified"}, true, "", nil, "verified"},
		{"same country, matched by name: confirmed", acmeJob, acme, acmeContact("sam@acmerobotics.example", "verified"), true, "", nil, "verified"},
		{"country unknown on one side never fails the check", jobPost{Company: "Acme Robotics", Title: "Engineer", Location: "Remote", Source: sourceLinkedIn}, acme,
			acmeContact("sam@acmerobotics.example", "verified"), true, "", nil, "verified"},
		{"the company's site under another country suffix", acmeJob, hiringCompany{size: 120, domain: "acmerobotics.co.uk"}, acmeContact("sam@acmerobotics.example", "verified"), true, "", nil, "verified"},
		{"email at an unrelated domain", acmeJob, acme, acmeContact("sam@ugbazaar.example", "verified"), false, "email is at ugbazaar.example",
			[]string{`email domain "ugbazaar.example" vs job company "Acme Robotics" (acmerobotics.example)`}, ""},
		{"a free mailbox is never the company's", acmeJob, acme, acmeContact("sam.okoye@gmail.com", "verified"), false, "only a personal email address",
			[]string{`email domain "gmail.com" is a personal mailbox`}, ""},
		{"catch-all: still a lead, labelled", acmeJob, acme, acmeContact("sam@acmerobotics.example", "catchall"), true, "", nil, "catch_all"},
		{"never checked: still a lead, not called verified", acmeJob, acme, acmeContact("sam@acmerobotics.example", ""), true, "", nil, "unverified"},
		{"risky: not called verified", acmeJob, acme, acmeContact("sam@acmerobotics.example", "risky"), true, "", nil, "unverified"},
	}
	for _, c := range cases {
		got := confirmContact(c.job.Company, jobIdentity(c.job, c.lookup), 5000, person, c.contact)
		if got.confirmed != c.confirmed || got.tooBig {
			t.Errorf("%s: got %+v", c.name, got)
			continue
		}
		if !strings.Contains(got.reason, c.reasonHas) {
			t.Errorf("%s: reason %q should mention %q", c.name, got.reason, c.reasonHas)
		}
		for _, want := range c.mismatchHas {
			if !strings.Contains(got.mismatch, want) {
				t.Errorf("%s: mismatch %q should name %q", c.name, got.mismatch, want)
			}
		}
		if got.email != c.email {
			t.Errorf("%s: email label %q, want %q", c.name, got.email, c.email)
		}
		if got.confirmed && (got.reason != "" || got.mismatch != "") {
			t.Errorf("%s: a confirmed contact has no reason or mismatch, got %+v", c.name, got)
		}
		// The stored signal says what disagreed, is archived, and never
		// calls a catch-all address verified.
		var meta map[string]any
		if err := json.Unmarshal(hiringMetadata(c.job, "CTO", got, nil), &meta); err != nil {
			t.Fatal(err)
		}
		if mm, _ := meta["identity_mismatch"].(string); mm != got.mismatch || meta["confirmed"] != c.confirmed {
			t.Errorf("%s: metadata %v", c.name, meta)
		}
		if es, _ := meta["email_status"].(string); es != c.email {
			t.Errorf("%s: metadata email_status %q, want %q", c.name, es, c.email)
		}
		if status := hiringSignalStatus(got); c.confirmed != (status == "new") {
			t.Errorf("%s: stored as %q", c.name, status)
		}
	}
}

// A size the lookup by name gave is dropped with its website when the
// posting names another website: it was a namesake's.
func TestJobIdentity(t *testing.T) {
	hn := jobPost{Company: "Toku", CompanyURL: "https://www.trytoku.example/careers", Location: "Santiago, Chile | Remote", Source: sourceHN}
	if got := jobIdentity(hn, hiringCompany{size: 13, domain: "toku.example"}); got != (hiringCompany{domain: "trytoku.example"}) {
		t.Errorf("another company's lookup must be dropped, got %+v", got)
	}
	if got := jobIdentity(hn, hiringCompany{size: 40, domain: "eng.trytoku.example"}); got.size != 40 || got.domain != "trytoku.example" || got.byName {
		t.Errorf("the posting's own site confirms the lookup, got %+v", got)
	}
	li := jobPost{Company: "Acme", CompanyURL: "https://www.linkedin.com/company/Acme-Inc/", Location: "Raleigh, NC", Source: sourceLinkedIn}
	if got := jobIdentity(li, hiringCompany{size: 50, domain: "acme.example"}); got != (hiringCompany{size: 50, domain: "acme.example", page: "acme-inc", country: "US", byName: true}) {
		t.Errorf("got %+v", got)
	}
	if got := jobIdentity(li, hiringCompany{}); got.byName || got.domain != "" {
		t.Errorf("nothing looked up: %+v", got)
	}
	for in, want := range map[string]string{"https://www.linkedin.com/company/acme": "acme", "https://uk.linkedin.com/company/Acme/about": "acme",
		"https://www.linkedin.com/in/someone": "", "https://acme.example/company/acme": "", "": ""} {
		if got := companyPageSlug(in); got != want {
			t.Errorf("companyPageSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJobCountry(t *testing.T) {
	cases := []struct {
		job  jobPost
		want string
	}{
		{jobPost{Location: "Raleigh, NC", Source: sourceLinkedIn}, "US"},
		{jobPost{Location: "Rio de Janeiro, Rio de Janeiro, Brazil", Source: sourceLinkedIn}, "BR"},
		{jobPost{Location: "London, England, United Kingdom", Source: sourceLinkedIn}, "GB"},
		{jobPost{Location: "Toronto, CA", Country: "ca", Source: sourceBlitz}, "CA"}, // the source's code wins over "CA" the state
		{jobPost{Location: "Berlin, DE", Source: sourceHN}, ""},                      // not Delaware
		{jobPost{Location: "NYC, Remote (US)", Source: sourceHN}, ""},
		{jobPost{Location: "San Francisco, California", Source: sourceHN}, "US"},
		{jobPost{Location: "Remote", Source: sourceLinkedIn}, ""},
		{jobPost{}, ""},
	}
	for _, c := range cases {
		if got := jobCountry(c.job); got != c.want {
			t.Errorf("jobCountry(%+v) = %q, want %q", c.job, got, c.want)
		}
	}
	// A contact's location is free text: a two-letter part is not read as a state.
	if got := locationCountry("Bellevue, WA", false); got != "" {
		t.Errorf("got %q", got)
	}
}

// The provider's first result was a London VP for a junior role in Raleigh.
func TestRankBuyers(t *testing.T) {
	titles := "CTO, VP Engineering or Head of Engineering"
	junior := jobPost{Company: "Acme", Title: "Junior Software Engineer", Location: "Raleigh, NC", Source: sourceLinkedIn}
	people := []foundPerson{
		{Name: "London VP", Title: "VP Engineering", Company: "Acme", Location: "London, England, United Kingdom", ProfileURL: "https://www.linkedin.com/in/a"},
		{Name: "Sales", Title: "Director of Sales", Company: "Acme", Location: "Raleigh, North Carolina, United States", ProfileURL: "https://www.linkedin.com/in/b"},
		{Name: "Raleigh EM", Title: "Engineering Manager", Company: "Acme", Location: "Raleigh, North Carolina, United States", ProfileURL: "https://www.linkedin.com/in/c"},
		{Name: "Recruiter", Title: "Senior Technical Recruiter", Company: "Acme", Location: "Austin, Texas, United States", ProfileURL: "https://www.linkedin.com/in/d"},
	}
	names := func(ps []foundPerson) string {
		var out []string
		for _, p := range ps {
			out = append(out, p.Name)
		}
		return strings.Join(out, ", ")
	}
	// A large company: whoever runs this hire, in the job's country, first.
	if got := names(pickBuyers(rankBuyers(people, titles, junior, 2400), "Acme", maxContactTries)); got != "Raleigh EM, Recruiter" {
		t.Errorf("large company, junior role: got %s", got)
	}
	// A small company: the plan's buyer fits, but the one in the job's
	// country is tried before the one abroad; another function comes last.
	if got := names(rankBuyers(people, titles, junior, 80)); got != "Raleigh EM, Recruiter, London VP, Sales" {
		t.Errorf("small company: got %s", got)
	}
	// Nothing to tell them apart: the search's order is kept.
	same := []foundPerson{{Name: "One", Title: "CTO"}, {Name: "Two", Title: "CTO"}}
	if got := names(rankBuyers(same, titles, jobPost{Title: "Engineer"}, 0)); got != "One, Two" {
		t.Errorf("ties: got %s", got)
	}
	if people[0].Name != "London VP" {
		t.Error("the input must not be reordered")
	}
}

func TestContactFit(t *testing.T) {
	titles := "CTO, VP Engineering or Head of Engineering"
	cases := []struct {
		title string
		size  int
		job   string
		want  int
	}{
		{"VP of Engineering", 200, "Senior Backend Engineer", 3},
		{"Head of Talent Acquisition", 200, "Senior Backend Engineer", 3},
		{"Co-Founder & CEO", 30, "Software Engineer", 3},  // a founder hires at a small company
		{"Co-Founder & CEO", 0, "Software Engineer", 2},   // size unknown
		{"Co-Founder & CEO", 900, "Software Engineer", 0}, // not at a large one
		{"Founder", 300, "Software Engineer", 1},
		{"Director of Sales", 200, "Software Engineer", 0},
		{"Chief Marketing Officer", 30, "Software Engineer", 0},
		{"Engineering Manager", 2400, "Junior Software Engineer", 3},
		{"Technical Recruiter", 2400, "Junior Software Engineer", 3},
		{"VP Engineering", 2400, "Senior Staff Engineer", 2},
		{"VP Engineering", 2400, "Junior Software Engineer", 1}, // never sees a junior requisition
	}
	for _, c := range cases {
		if got := contactFit(c.title, titles, c.size, c.job); got != c.want {
			t.Errorf("contactFit(%q, size %d, %q) = %d, want %d", c.title, c.size, c.job, got, c.want)
		}
	}
}

// The score used to be 7.5 for a verified email and 6.5 for a catch-all one,
// whoever the contact was.
func TestHiringLeadScoreIsOnFit(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	best := leadFit{contact: 3, country: 1, sizeFits: true, posted: now.Add(-6 * time.Hour), email: "verified"}
	score := func(mut func(*leadFit)) float64 {
		f := best
		if mut != nil {
			mut(&f)
		}
		return hiringLeadScore(f, now)
	}
	top := score(nil)
	if top > 10 || top < 8 {
		t.Fatalf("best fit scores %.2f", top)
	}
	wrongPerson := score(func(f *leadFit) { f.contact = 0 })
	catchAll := score(func(f *leadFit) { f.email = "catch_all" })
	abroad := score(func(f *leadFit) { f.country = -1 })
	// A right contact with a catch-all email outranks a wrong contact with a verified one.
	if !(top > catchAll && catchAll > abroad && abroad > wrongPerson) {
		t.Errorf("order: top %.2f, catch-all %.2f, abroad %.2f, wrong person %.2f", top, catchAll, abroad, wrongPerson)
	}
	if d := top - catchAll; d > 0.25 {
		t.Errorf("email status moves the score by %.2f: it must be a small modifier", d)
	}
	for name, mut := range map[string]func(*leadFit){
		"company size unknown or outside the plan": func(f *leadFit) { f.sizeFits = false },
		"posted days ago":                          func(f *leadFit) { f.posted = now.Add(-5 * 24 * time.Hour) },
		"date unknown":                             func(f *leadFit) { f.posted = time.Time{} },
		"country unknown":                          func(f *leadFit) { f.country = 0 },
	} {
		if got := score(mut); got >= top {
			t.Errorf("%s: %.2f should rank under %.2f", name, got, top)
		}
	}
	// Whatever the fit, a confirmed lead stays a lead.
	if worst := hiringLeadScore(leadFit{country: -1, email: "unverified"}, now); worst < leadScoreThreshold {
		t.Errorf("worst confirmed lead scores %.2f, under the lead threshold", worst)
	}
}
