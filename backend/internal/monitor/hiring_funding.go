package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"leadecho/internal/enrich"
	"leadecho/internal/treg"
)

// Funding signals: a company that just raised a round hires engineers within
// weeks. Once a day, alongside the job sources, the scan asks for the newest
// rounds and puts each funded company through the same pipeline as a
// posting's company (screen, size, contact, identity gate, research, draft).
// A company that both raised and is hiring is one lead carrying both facts.

const (
	tregFundingTool = "predictleads.financing.discover"
	sourceFunding   = "funding"

	// Records asked for per day, each billed ($0.04) whether or not it is
	// usable; workspaces.settings.hiring_signals.funding_limit overrides it
	// (0 turns the signal off).
	defaultFundingLimit    = 5
	maxFundingLimit        = 25
	fundingRecordCostMicro = 40_000
	// fundingInterval is under a day: the scan itself is claimed every 24h,
	// and a retry or an operator's manual scan must not buy the list again.
	fundingInterval = 20 * time.Hour
	// The provider orders rounds by when it found them, not by their date, so
	// old rounds (and dates a few days ahead) come back among the new ones.
	fundingMaxAge   = 14 * 24 * time.Hour
	fundingMaxAhead = 3 * 24 * time.Hour

	fundingTypes = "seed,series_a,series_b"
	// ponytail: US rounds only, and skipped for a workspace whose countries
	// leave the US out. Map the workspace's countries to the provider's
	// location names when a customer sells elsewhere.
	fundingLocation = "United States"
	maxFundingText  = 40 // runes kept of a provider's amount or round type
)

// fundingRound is the round as the lead stores it, under these keys of
// platform_metadata. Everything shown or written about the round comes from
// these four fields.
type fundingRound struct {
	Type   string `json:"funding_type"`             // "seed", "series_a", ...
	Amount string `json:"funding_amount,omitempty"` // as announced ("$4.4 million")
	Date   string `json:"funding_date"`             // 2026-11-13
	URL    string `json:"funding_url,omitempty"`    // where it was announced
	domain string // the funded company's website, as the provider gives it
}

// label is the round type in words: "series_a" -> "Series A".
func (f fundingRound) label() string {
	words := strings.Fields(strings.ReplaceAll(f.Type, "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// headline is "a Seed round of $4.4 million" (no amount: "a Seed round").
func (f fundingRound) headline() string {
	if f.Amount == "" {
		return fmt.Sprintf("a %s round", f.label())
	}
	return fmt.Sprintf("a %s round of %s", f.label(), f.Amount)
}

// fact is the round as the draft and the screen are told it.
func (f *fundingRound) fact() string {
	if f == nil {
		return ""
	}
	return f.headline() + ", announced " + f.Date
}

func (f fundingRound) when() time.Time {
	t, _ := time.Parse(time.DateOnly, f.Date)
	return t
}

// fundingLimit is how many records to ask for: the workspace's setting or
// the default, bounded.
func fundingLimit(st hiringState) int {
	n := defaultFundingLimit
	if st.FundingLimit != nil {
		n = *st.FundingLimit
	}
	return min(max(n, 0), maxFundingLimit)
}

// fundedCompanies fetches the newest rounds, at most once per fundingInterval,
// as entries for the scan's pipeline. Nothing is returned (and nothing is
// retried sooner than the next run) when the call fails.
func (m *Monitor) fundedCompanies(ctx context.Context, wsID string, st *hiringState, now time.Time, rep *hiringReport) []jobPost {
	limit := fundingLimit(*st)
	if limit == 0 || now.Sub(st.LastFunding) < fundingInterval {
		return nil
	}
	if cc := countryCodes(st.Countries); len(cc) > 0 && !slices.Contains(cc, "US") {
		return nil
	}
	sr := rep.source(sourceFunding)
	sr.Calls++
	resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tregFundingTool, Method: http.MethodGet,
		Query: url.Values{"financing_types_normalized": {fundingTypes}, "company_location": {fundingLocation}, "limit": {strconv.Itoa(limit)}},
		// Billed per record, and five records cost more than the ceiling set
		// for single lookups: the call states its own, the price of the
		// records it asks for.
		MaxCostMicro: int64(limit) * fundingRecordCostMicro})
	if err != nil {
		sr.FailedCalls++
		if !treg.IsOperatorError(err) {
			m.logger.Debug().Err(err).Str("workspace_id", wsID).Msg("hiring: funding search failed")
		}
		return nil
	}
	st.LastFunding = now
	rep.FundingSpendMicro = resp.CostMicro
	rounds, returned := parseFunding(resp.Body, now)
	sr.Fetched = returned
	rep.skip("funding_old_or_incomplete", returned-len(rounds))
	return rounds
}

// parseFunding reads the JSON:API answer {data:[{attributes, relationships.
// company}], included:[{id, attributes{domain, company_name}}]} (or the same
// under "output"). A round is kept when its date is inside the window and
// its company has a website: the website is what a contact's employer is
// checked against. returned counts every record, since each is billed.
func parseFunding(body []byte, now time.Time) (rounds []jobPost, returned int) {
	type page struct {
		Data []struct {
			Attributes struct {
				EffectiveDate string   `json:"effective_date"`
				FoundAt       string   `json:"found_at"`
				Type          string   `json:"financing_type_normalized"`
				Amount        string   `json:"amount"`
				SourceURLs    []string `json:"source_urls"`
			} `json:"attributes"`
			Relationships struct {
				Company struct {
					Data struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"company"`
			} `json:"relationships"`
		} `json:"data"`
		Included []struct {
			ID         string `json:"id"`
			Attributes struct {
				Domain string `json:"domain"`
				Name   string `json:"company_name"`
			} `json:"attributes"`
		} `json:"included"`
	}
	var v struct {
		Output *page `json:"output"`
		page
	}
	if json.Unmarshal(body, &v) != nil {
		return nil, 0
	}
	got := v.page
	if v.Output != nil {
		got = *v.Output
	}
	type company struct{ name, domain string }
	companies := map[string]company{} // investors are listed here too: only the round's own company is looked up
	for _, inc := range got.Included {
		companies[inc.ID] = company{trimLegalForm(inc.Attributes.Name), companyDomain(inc.Attributes.Domain)}
	}
	for _, d := range got.Data {
		a, co := d.Attributes, companies[d.Relationships.Company.Data.ID]
		date, err := time.Parse(time.DateOnly, a.EffectiveDate)
		kind := companyKey(a.Type)
		if err != nil || date.Before(now.Add(-fundingMaxAge)) || date.After(now.Add(fundingMaxAhead)) || co.domain == "" || co.name == "" || kind == "" {
			continue
		}
		round := &fundingRound{Type: clipRunes(strings.ReplaceAll(kind, "-", "_"), maxFundingText), Amount: clipRunes(strings.TrimSpace(a.Amount), maxFundingText), Date: a.EffectiveDate, domain: co.domain}
		site := "https://" + co.domain
		job := jobPost{Company: truncateRunes(co.name, maxProspectName), CompanyURL: site, URL: site, Source: sourceFunding, Funding: round}
		for _, u := range a.SourceURLs {
			if validHTTPURL(u) {
				round.URL, job.URL = u, u
				break
			}
		}
		// The lead is dated by when the round was found: that is when it became news.
		if found, err := time.Parse(time.RFC3339, a.FoundAt); err == nil && !found.After(now) {
			job.CreatedUTC = found.Unix()
		}
		rounds = append(rounds, job)
	}
	return rounds, len(got.Data)
}

// trimLegalForm drops a trailing legal form from a company's display name
// ("Tiberius Aerospace Inc." -> "Tiberius Aerospace"), so the round and a job
// posting by the same company share one lead.
// ponytail: names are matched as written otherwise ("Acme" and "Acme AI" are
// two companies); match on the website if that splits real leads.
func trimLegalForm(name string) string {
	f := strings.Fields(name)
	for len(f) > 1 && slices.Equal(enrich.CompanyWords(strings.Join(f, " ")), enrich.CompanyWords(strings.Join(f[:len(f)-1], " "))) {
		f = f[:len(f)-1]
	}
	return strings.TrimRight(strings.Join(f, " "), ", ")
}

// mergeFunded puts the day's funded companies in front of its postings (they
// are few, and the day's people searches run in order). A funded company that
// also has a posting in hand is one entry: the posting, carrying the round.
func mergeFunded(fresh, funded []jobPost, skipReason func(jobPost) string, rep *hiringReport) []jobPost {
	if len(funded) == 0 {
		return fresh
	}
	at := map[string]int{}
	for i, j := range fresh {
		at[companyKey(j.Company)] = i
	}
	sr, seen := rep.source(sourceFunding), map[string]bool{}
	var first []jobPost
	for _, f := range funded {
		key := companyKey(f.Company)
		if i, ok := at[key]; ok {
			fresh[i].Funding = f.Funding
			continue
		}
		reason := "duplicate_or_incomplete" // two rounds of one company
		if !seen[key] {
			reason = skipReason(f)
		}
		seen[key] = true
		if reason != "" {
			rep.skip(reason, 1)
			continue
		}
		sr.New++
		first = append(first, f)
	}
	return append(first, fresh...)
}

// fundingFact is the round as platform_metadata keys; nil without a round.
func fundingFact(f *fundingRound) map[string]any {
	if f == nil {
		return nil
	}
	return map[string]any{"funding_type": f.Type, "funding_amount": f.Amount, "funding_date": f.Date, "funding_url": f.URL}
}

// signalFact is what a second signal adds to a company's lead that is
// already stored this month: the round for a lead found by its posting, the
// posting for a lead found by its round. marker is a key the lead has once
// the fact is there. A round is only added to a lead stored with the funded
// company's website (domain): the two were matched by name, and a namesake's
// round must not be stated as this company's.
// ponytail: a posting is added on the name alone (most postings carry no
// website); its link is stored beside it so the reader can check.
func signalFact(job jobPost) (patch map[string]any, marker, domain string) {
	if job.Title == "" && job.Funding != nil {
		return fundingFact(job.Funding), "funding_type", job.Funding.domain
	}
	return map[string]any{"job_title": job.Title, "job_url": job.URL, "job_location": job.Location}, "job_title", ""
}

// addSignalFact records the second signal on the stored lead. Best effort:
// the lead stands without it.
func (m *Monitor) addSignalFact(ctx context.Context, wsID, platform, pid string, job jobPost) {
	patch, marker, domain := signalFact(job)
	if err := m.q.AddMentionMetadata(ctx, wsID, platform, pid, marker, domain, jsonBytes(patch)); err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Str("company", job.Company).Msg("hiring: add second signal to the lead")
	}
}

// signalText is the lead's title and opening sentence: the posting, the
// round, or both.
func signalText(job jobPost) (title, opening string) {
	if job.Title == "" && job.Funding != nil {
		return fmt.Sprintf("%s raised %s", job.Company, job.Funding.headline()),
			fmt.Sprintf("%s raised %s, announced %s.", job.Company, job.Funding.headline(), job.Funding.Date)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s posted a job for %q", job.Company, job.Title)
	if job.Location != "" {
		fmt.Fprintf(&b, " (%s)", job.Location)
	}
	b.WriteString(postedOn(job) + ".")
	if job.Funding != nil {
		fmt.Fprintf(&b, " It also raised %s, announced %s.", job.Funding.headline(), job.Funding.Date)
	}
	return fmt.Sprintf("%s is hiring: %s", job.Company, job.Title), b.String()
}

// postedOn is " on 12 November 2026" when the posting's date is known, and
// nothing otherwise: the stored text is read for weeks, so it never says
// "this week".
func postedOn(job jobPost) string {
	if job.CreatedUTC <= 0 {
		return ""
	}
	return " on " + time.Unix(job.CreatedUTC, 0).UTC().Format("2 January 2006")
}

// signalKind names the signal behind an entry: a round with no posting is a
// funding signal, anything else a hiring signal.
func signalKind(job jobPost) string {
	if job.Title == "" && job.Funding != nil {
		return "funding"
	}
	return "hiring"
}

// signalDate is when the signal happened, for the freshness part of the score.
func signalDate(job jobPost) time.Time {
	switch {
	case job.CreatedUTC > 0:
		return time.Unix(job.CreatedUTC, 0)
	case job.Funding != nil:
		return job.Funding.when()
	}
	return time.Time{}
}
