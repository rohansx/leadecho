package monitor

import (
	"net/url"
	"strings"
	"time"

	"leadecho/internal/database"
)

// Carry-over: a run screens more companies than its paid lookups can reach
// (the first Hacker News run of a month collects about 120 and reaches 20),
// and the next run asks its sources only for newer postings. The companies
// that passed the screen and were not reached are kept in the workspace's
// hiring state and go first on the next run: they were already searched for
// and screened, so neither is paid for again.

const (
	maxHiringCarried = 60
	hiringCarryAge   = 10 * 24 * time.Hour
)

// carriedJob is a screened posting (or funded company) waiting for a run with
// budget left, as stored under workspaces.settings.hiring_signals.carry.
type carriedJob struct {
	Company    string `json:"company"`
	CompanyURL string `json:"company_url,omitempty"`
	Title      string `json:"title,omitempty"`
	Location   string `json:"location,omitempty"`
	URL        string `json:"url"`
	CreatedUTC int64  `json:"created_utc,omitempty"`
	Source     string `json:"source,omitempty"`
	SizeFits   bool   `json:"size_fits,omitempty"`
	Country    string `json:"country,omitempty"`
	// The round a funded company raised, and that company's website.
	Funding       *fundingRound `json:"funding,omitempty"`
	FundingDomain string        `json:"funding_domain,omitempty"`
	Seen          time.Time     `json:"seen"` // when it first passed the screen
}

func (c carriedJob) job() jobPost {
	j := jobPost{Company: c.Company, CompanyURL: c.CompanyURL, Title: c.Title, Location: c.Location, URL: c.URL,
		CreatedUTC: c.CreatedUTC, Source: c.Source, SizeFits: c.SizeFits, Country: c.Country}
	if c.Funding != nil {
		f := *c.Funding
		f.domain = c.FundingDomain
		j.Funding = &f
	}
	return j
}

// carriedJobs returns the stored companies that are still worth a lookup, in
// the order they were stored, and when each was first seen. skipReason is the
// scan's own check (stored since, on the skip list, too old to insert).
func carriedJobs(carry []carriedJob, now time.Time, skipReason func(jobPost) string, rep *hiringReport) (jobs []jobPost, seen map[string]time.Time) {
	seen = map[string]time.Time{}
	for _, c := range carry {
		job, key := c.job(), companyKey(c.Company)
		if _, dup := seen[key]; dup || key == "" {
			continue
		}
		if now.Sub(c.Seen) > hiringCarryAge {
			rep.skip("carry_expired", 1)
			continue
		}
		if reason := skipReason(job); reason != "" {
			rep.skip(reason, 1)
			continue
		}
		seen[key] = c.Seen
		jobs = append(jobs, job)
	}
	return jobs, seen
}

// carryOver is what to store for the next run: the companies this run did not
// reach, oldest first, at most maxHiringCarried. A company carried before
// keeps the date it was first seen, so it cannot be carried for ever.
func carryOver(unreached []jobPost, seen map[string]time.Time, now time.Time) (carry []carriedJob, dropped int) {
	for _, j := range unreached {
		if len(carry) == maxHiringCarried {
			dropped++
			continue
		}
		c := carriedJob{Company: j.Company, CompanyURL: j.CompanyURL, Title: j.Title, Location: j.Location, URL: j.URL,
			CreatedUTC: j.CreatedUTC, Source: j.Source, SizeFits: j.SizeFits, Country: j.Country, Seen: now}
		if j.Funding != nil {
			f := *j.Funding
			c.Funding, c.FundingDomain = &f, f.domain
		}
		if at, ok := seen[companyKey(j.Company)]; ok {
			c.Seen = at
		}
		carry = append(carry, c)
	}
	return carry, dropped
}

// limitFunded keeps funded companies (a round, no posting) to a share of the
// run: they sit at the front of the queue, and on a small plan they would use
// every people search of the day. The first max stay where they are; the rest
// are returned as over, to wait for a later run.
func limitFunded(jobs []jobPost, max int) (queue, over []jobPost) {
	funded := 0
	for _, j := range jobs {
		if signalKind(j) != "funding" {
			queue = append(queue, j)
			continue
		}
		if funded++; funded > max {
			over = append(over, j)
			continue
		}
		queue = append(queue, j)
	}
	return queue, over
}

// fundedShare is how many of a run's people searches may go to funded
// companies: a third (1 of Starter's 3, 6 of Pro's 20).
func fundedShare(peopleSearches int) int { return peopleSearches / 3 }

// signalPlatforms are the platforms a company signal may be stored under; a
// company is one lead a month across all of them.
var signalPlatforms = []string{string(database.PlatformTypeLinkedin), string(database.PlatformTypeHackernews)}

// signalPlatform is the platform a signal is stored under: where its link
// leads. A Hacker News posting links to the post; everything else (LinkedIn
// postings, funding rounds, whose contact is a LinkedIn profile) is LinkedIn.
func signalPlatform(job jobPost) string {
	if u, err := url.Parse(job.URL); err == nil && job.Source == sourceHN && strings.EqualFold(u.Hostname(), "news.ycombinator.com") {
		return string(database.PlatformTypeHackernews)
	}
	return string(database.PlatformTypeLinkedin)
}
