package monitor

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"leadecho/internal/enrich"
)

// The identity gate. A lead joins three identities that come from three
// different lookups: the company on the job posting, the employer on the
// contact's profile, and the domain of the email found for the contact. Each
// lookup answers about a namesake often enough (a Chilean fintech's job, the
// CTO of a Singapore staffing firm of the same name) that a lead is only
// confirmed when all three are the same organisation.

// companyPageSlug is the company's LinkedIn page name
// ("https://www.linkedin.com/company/Acme-Inc/" -> "acme-inc"), "" for
// anything else. A page given by its number (/company/1234567) is "" too: it
// is the same page as its name, and the two can't be compared.
func companyPageSlug(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.HasSuffix(strings.ToLower(u.Hostname()), "linkedin.com") {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "company" {
		return ""
	}
	if strings.Trim(parts[1], "0123456789") == "" {
		return ""
	}
	return strings.ToLower(parts[1])
}

// jobIdentity adds what the posting itself says about its company to what
// the lookup by name returned. A website the posting names wins over the
// lookup's: when they differ the lookup answered about another company, and
// its size is dropped with it.
func jobIdentity(job jobPost, co hiringCompany) hiringCompany {
	co.byName = co.domain != ""
	if d := jobSiteDomain(job); d != "" {
		if co.domain != "" && !enrich.SameDomain(co.domain, d) {
			co = hiringCompany{}
		}
		co.domain, co.byName = d, false
	}
	co.page, co.country = companyPageSlug(job.CompanyURL), jobCountry(job)
	return co
}

// jobCountry is the ISO code of the country the job is in, "" when unknown.
func jobCountry(job jobPost) string {
	if c := strings.ToUpper(strings.TrimSpace(job.Country)); len(c) == 2 {
		return c
	}
	// LinkedIn writes US jobs as "Raleigh, NC" and never uses country codes;
	// a Hacker News post is free text, where "Berlin, DE" is not Delaware.
	return locationCountry(job.Location, job.Source != sourceHN)
}

var (
	// ponytail: the countries customers sell into so far, by the names
	// LinkedIn uses. An unlisted place is "unknown", which never fails a
	// check; use a geocoder if the country starts to decide more.
	countryByName = map[string]string{
		"united states": "US", "usa": "US", "us": "US", "united states of america": "US",
		"united kingdom": "GB", "uk": "GB", "england": "GB", "scotland": "GB", "wales": "GB",
		"canada": "CA", "india": "IN", "germany": "DE", "france": "FR", "netherlands": "NL", "spain": "ES", "italy": "IT",
		"ireland": "IE", "portugal": "PT", "poland": "PL", "sweden": "SE", "switzerland": "CH", "belgium": "BE",
		"australia": "AU", "new zealand": "NZ", "singapore": "SG", "japan": "JP", "china": "CN", "hong kong": "HK",
		"brazil": "BR", "brasil": "BR", "mexico": "MX", "chile": "CL", "argentina": "AR", "colombia": "CO",
		"israel": "IL", "united arab emirates": "AE", "south africa": "ZA", "nigeria": "NG", "indonesia": "ID",
		"philippines": "PH", "vietnam": "VN", "turkey": "TR", "ukraine": "UA", "romania": "RO", "denmark": "DK",
		"norway": "NO", "finland": "FI", "austria": "AT", "czechia": "CZ", "pakistan": "PK", "egypt": "EG",
		"san francisco bay area": "US", "new york city metropolitan area": "US", "greater boston": "US",
		"greater seattle area": "US", "greater london": "GB",
	}
	// "georgia" is left out: it is also a country.
	usStateNames = wordSet("alabama|alaska|arizona|arkansas|california|colorado|connecticut|delaware|florida|hawaii|idaho|illinois|indiana|iowa|kansas|kentucky|louisiana|maine|maryland|massachusetts|michigan|minnesota|mississippi|missouri|montana|nebraska|nevada|new hampshire|new jersey|new mexico|new york|north carolina|north dakota|ohio|oklahoma|oregon|pennsylvania|rhode island|south carolina|south dakota|tennessee|texas|utah|vermont|virginia|washington|west virginia|wisconsin|wyoming|district of columbia")
	usStateCodes = wordSet("al|ak|az|ar|ca|co|ct|de|fl|ga|hi|id|il|in|ia|ks|ky|la|me|md|ma|mi|mn|ms|mo|mt|ne|nv|nh|nj|nm|ny|nc|nd|oh|ok|or|pa|ri|sc|sd|tn|tx|ut|vt|va|wa|wv|wi|wy|dc")
)

func wordSet(list string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Split(list, "|") {
		out[w] = true
	}
	return out
}

// locationCountry reads the country from "City, Region, Country" as an ISO
// code, "" when it can't tell. stateCodes says a two-letter part is a US
// state ("Raleigh, NC"): only true where the text never holds country codes.
func locationCountry(loc string, stateCodes bool) string {
	parts := strings.Split(loc, ",")
	for i := len(parts) - 1; i >= 0; i-- { // the country comes last
		p := strings.ToLower(strings.TrimSpace(parts[i]))
		if c, ok := countryByName[p]; ok {
			return c
		}
		if usStateNames[p] || (stateCodes && usStateCodes[p]) {
			return "US"
		}
	}
	return ""
}

// describeCompany is a company as a mismatch note names it: "Toku" (toku.co).
func describeCompany(name, domain string) string {
	if domain == "" {
		return fmt.Sprintf("%q", name)
	}
	return fmt.Sprintf("%q (%s)", name, domain)
}

// sameEmployer checks the contact's employer against the hiring company.
// A shared name never confirms it (two companies can both be "Prophetic"):
// the LinkedIn pages must be the same, or the websites. A website that is
// only known from a lookup by name can itself be a namesake's, and so can one
// that only matches under another suffix (acme.com, acme.io), so there the
// contact must also not be in another country than the job.
// When it fails, reason says why in plain words, and mismatch (when the two
// sides positively disagree) names both for the lead's metadata.
func sameEmployer(company string, co hiringCompany, name string, c *enrich.Result) (ok bool, reason, mismatch string) {
	cd := enrich.CleanDomain(c.CompanyDomain)
	sides := fmt.Sprintf("employer %s vs job company %s", describeCompany(c.Company, cd), describeCompany(company, co.domain))
	named := func() (bool, string, string) { // the existing wording, by what the two names say
		switch {
		case c.Company == "":
			return false, fmt.Sprintf("%s's profile lists no current employer", name), ""
		case sameCompany(company, c.Company):
			// The names agree, but nothing else confirms it: it may be a namesake.
			return false, fmt.Sprintf("could not verify that %s's employer is this %s and not another company of the same name", name, company), ""
		}
		return false, fmt.Sprintf("%s's profile lists %s, not %s", name, c.Company, company), sides
	}
	if c.IdentityMismatch != "" {
		return false, fmt.Sprintf("%s's email and the employer on their profile disagree, so who they work for is unknown", name), c.IdentityMismatch
	}
	if jobPage, page := co.page, companyPageSlug(c.CompanyLinkedInURL); jobPage != "" && page != "" {
		if jobPage == page {
			return true, "", "" // the very page that posted the job
		}
		_, reason, _ = named()
		return false, reason, sides + fmt.Sprintf("; LinkedIn page %q vs %q", page, jobPage)
	}
	if cd == "" || co.domain == "" {
		return named()
	}
	sameSite := enrich.SameDomain(cd, co.domain)
	if !sameSite && !(enrich.SameDomainLabel(cd, co.domain) && sameCompany(company, c.Company)) {
		_, reason, _ = named()
		return false, reason, sides
	}
	if cc := locationCountry(c.Location, false); (co.byName || !sameSite) && cc != "" && co.country != "" && cc != co.country {
		return false, fmt.Sprintf("%s is in %s but the job is in %s, and the company was only matched by name: it may be another %s", name, cc, co.country, company),
			sides + fmt.Sprintf("; contact in %s, job in %s", cc, co.country)
	}
	return true, "", ""
}

// emailDisagrees checks the email found for the contact against the company:
// an address at another organisation, or a personal mailbox, does not reach
// the person at this job's company. Empty reason: it agrees.
func emailDisagrees(company string, co hiringCompany, name string, c *enrich.Result) (reason, mismatch string) {
	ed := enrich.EmailDomain(c.Email)
	switch {
	case enrich.FreeMail(ed):
		return fmt.Sprintf("only a personal email address (%s) was found for %s", ed, name),
			fmt.Sprintf("email domain %q is a personal mailbox, not job company %s", ed, describeCompany(company, co.domain))
	case !enrich.EmailAtCompany(c.Email, co.domain, company) && !enrich.EmailAtCompany(c.Email, c.CompanyDomain, c.Company):
		return fmt.Sprintf("%s's email is at %s, which is not %s", name, ed, company),
			fmt.Sprintf("email domain %q vs job company %s", ed, describeCompany(company, co.domain))
	}
	return "", ""
}

// emailLabel is how the lead states its email: only a verified address reads
// as verified.
func emailLabel(status string) string {
	switch status {
	case "verified":
		return "verified"
	case "catchall":
		return "catch_all"
	}
	return "unverified"
}

// emailCaveat is the sentence a lead's text carries about an email that was
// not verified ("" for a verified one).
func emailCaveat(label string) string {
	switch label {
	case "catch_all":
		return " Their email could not be verified (the company's mail server accepts any address), so it may bounce."
	case "unverified":
		return " Their email has not been verified, so it may bounce."
	}
	return ""
}

// ---- who to write to --------------------------------------------------------

var (
	engTitleRe     = regexp.MustCompile(`(?i)\b(engineer\w*|technology|technical|software|cto|r&d|developer\w*|development|platform|architect\w*)\b`)
	talentTitleRe  = regexp.MustCompile(`(?i)\b(recruit\w*|talent|people|hiring|human resources|hr)\b`)
	founderTitleRe = regexp.MustCompile(`(?i)\b(founder|owner|ceo|chief executive)\b`)
	execTitleRe    = regexp.MustCompile(`(?i)\b(chief|c[a-z]o|[se]?vp|vice president|president|founder|owner)\b`)
	juniorRoleRe   = regexp.MustCompile(`(?i)\b(junior|jr|entry[- ]level|graduate|associate|intern\w*)\b`)
	buyerTitleSplitRe   = regexp.MustCompile(`(?i),|\bor\b|/`)
)

const (
	// founderLedSize: up to here a founder does the hiring.
	founderLedSize = 50
	maxContactFit  = 3
)

// contactFit says how well a title fits the person who would buy for this
// opening, 0 (a decision-maker in another function) to 3. Engineering
// leadership, talent and recruiting, and the plan's own buyer titles fit; a
// founder fits at a small company. From hiringTeamContactSize up the people
// running this hire fit better than a company-wide executive, most of all
// for a junior role.
// ponytail: word lists, English only, like decisionTitleRe.
func contactFit(title, buyerTitles string, companySize int, jobTitle string) int {
	eng, talent := engTitleRe.MatchString(title), talentTitleRe.MatchString(title)
	founder, exec := founderTitleRe.MatchString(title), execTitleRe.MatchString(title)
	planned := titleNamesBuyer(title, buyerTitles)
	if companySize >= hiringTeamContactSize {
		switch {
		case talent, eng && !exec:
			return 3
		case (eng || planned) && juniorRoleRe.MatchString(jobTitle):
			return 1
		case eng, planned:
			return 2
		}
		return 0
	}
	switch {
	case eng, talent, planned:
		return 3
	case founder && companySize > 0 && companySize <= founderLedSize:
		return 3
	case founder && companySize == 0:
		return 2
	case founder:
		return 1
	}
	return 0
}

// titleNamesBuyer reports whether a title is one of the plan's buyer titles
// ("CTO, VP Engineering or Head of Engineering"): every word of one of them
// appears in it ("VP of Engineering" names "VP Engineering").
func titleNamesBuyer(title, buyerTitles string) bool {
	have := map[string]bool{}
	for _, w := range strings.Split(companyKey(title), "-") {
		have[w] = true
	}
next:
	for _, want := range buyerTitleSplitRe.Split(buyerTitles, -1) {
		words := strings.Split(companyKey(want), "-")
		if words[0] == "" {
			continue
		}
		for _, w := range words {
			if !have[w] {
				continue next
			}
		}
		return true
	}
	return false
}

// countryMatch is +1 when both countries are known and equal, -1 when both
// are known and differ, 0 otherwise.
func countryMatch(a, b string) int {
	switch {
	case a == "" || b == "":
		return 0
	case a == b:
		return 1
	}
	return -1
}

// rankBuyers orders the candidates a people search returned by how well they
// fit this opening, then by being in the job's country: search order is the
// provider's, and its first result was a London VP for a junior role in
// Raleigh. Ties keep search order.
func rankBuyers(people []foundPerson, buyerTitles string, job jobPost, companySize int) []foundPerson {
	out := append([]foundPerson(nil), people...)
	country := jobCountry(job)
	rank := func(p foundPerson) int {
		return 2*contactFit(p.Title, buyerTitles, companySize, job.Title) + countryMatch(country, locationCountry(p.Location, false))
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) > rank(out[j]) })
	return out
}

// ---- score ------------------------------------------------------------------

// leadFit is what a confirmed lead is scored on.
type leadFit struct {
	contact  int       // contactFit, 0..maxContactFit
	country  int       // countryMatch of the contact and the job
	sizeFits bool      // the company's size is known and inside the plan's range
	posted   time.Time // when the job was posted or the round announced; zero = unknown
	email    string    // emailLabel
}

// hiringLeadScore scores a confirmed lead on fit: is this the right person
// (up to +1.2), in the job's country (+-0.4), at a company of the right size
// (+0.3), and is the signal fresh (+0.3). Whether the email could be verified
// only nudges it (+0.2). A confirmed lead never scores under the lead
// threshold: it was confirmed, and the score only orders the inbox.
func hiringLeadScore(f leadFit, now time.Time) float64 {
	s := leadScoreThreshold + 0.4*float64(min(max(f.contact, 0), maxContactFit)) + 0.4*float64(f.country)
	if f.sizeFits {
		s += 0.3
	}
	if !f.posted.IsZero() {
		switch age := now.Sub(f.posted); {
		case age <= 48*time.Hour:
			s += 0.3
		case age <= maxMentionAge:
			s += 0.15
		}
	}
	if f.email == "verified" {
		s += 0.2
	}
	return math.Round(max(s, leadScoreThreshold)*100) / 100
}
