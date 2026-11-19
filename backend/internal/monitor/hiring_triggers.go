package monitor

import (
	"fmt"
	"strings"
)

// Buying triggers. For some products nearly every company "hires for the
// role": every software company has a software engineer job open, so an
// open role says nothing about pain, budget or timing, and an engineering
// assessment tool's hiring leads were mostly mature companies with a hiring
// process in place. With hiringState.RequireTrigger a company is kept only
// with a reason to buy now:
//   - it just raised a round (budget, and a hiring push to come);
//   - it has several matching openings at once (interviews eat the team);
//   - it is hiring a recruiter or talent lead (someone is setting up the
//     hiring process, and buying tools for it);
//   - it posted in HN's "Who is hiring" (small startups, where engineers run
//     the interviews themselves).

// minOpeningsTrigger is how many distinct matching postings count as hiring
// at volume.
const minOpeningsTrigger = 2

var talentTitles = []string{"recruiter", "talent acquisition", "head of talent", "talent partner", "recruiting"}

// buyingTrigger says why a company is worth contacting now ("" for none).
func buyingTrigger(j jobPost) string {
	switch {
	case j.Funding != nil:
		return "it just raised " + j.Funding.headline()
	case containsAnyFold(j.Title, talentTitles):
		return fmt.Sprintf("it is hiring a %s, so it is setting up its hiring process", strings.TrimSpace(j.Title))
	case j.Openings >= minOpeningsTrigger:
		return fmt.Sprintf("it has %d matching openings at once", j.Openings)
	case j.Source == sourceHN:
		return "a startup hiring through HN's Who is hiring, where engineers run the interviews"
	}
	return ""
}

// withTriggers keeps the companies with a buying trigger, recording it on
// each, and reports how many were dropped.
func withTriggers(jobs []jobPost) ([]jobPost, int) {
	out := jobs[:0:0]
	for _, j := range jobs {
		if t := buyingTrigger(j); t != "" {
			j.Trigger = t
			out = append(out, j)
		}
	}
	return out, len(jobs) - len(out)
}
