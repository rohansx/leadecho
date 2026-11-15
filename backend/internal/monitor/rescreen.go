package monitor

import (
	"context"
	"errors"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

// Re-screening: company leads stored before the posting screen (or under an
// older one) stay in the inbox with their old verdict. An operator can ask
// for a workspace's recent company leads to be checked again. It reads only
// what is already stored and asks the model once per batch: no data-provider
// call is made, and the customer is not charged.

// rescreenReviewBatch: when the screen keeps none of a batch this large, the
// likelier cause is an empty answer from the model than that many bad leads.
// Nothing in the batch is archived; it is reported as needing review.
const rescreenReviewBatch = 5

const rescreenPostingReason = "the job posting does not pass the current screen (not a role you look for, an employer that hires for its clients, or a company outside your market)"

// rescreenReasons returns why each failing lead fails, by lead id. A lead
// fails when its stored research ruled the company out, or (hiring signals)
// when screen no longer keeps its posting. screen is given at most
// maxHiringScreened postings at a time; if it fails, so does the whole run,
// so a model outage archives nobody. review counts the leads of batches the
// screen dropped whole (see rescreenReviewBatch): they are left as they are.
func rescreenReasons(leads []database.CompanyLead, screen func([]ai.HiringJob) ([]ai.ProspectFit, error)) (reasons map[string]string, review int, err error) {
	reasons = map[string]string{}
	var postings []jobPost
	for _, l := range leads {
		switch {
		case l.NotABuyer != "":
			// The same fixed sentence a new lead gets, not the model's wording.
			reasons[l.ID] = afterResearch(contactCheck{confirmed: true}, &ai.ResearchBrief{NotABuyer: l.NotABuyer}).reason
		case l.Signal == "hiring" && l.JobTitle != "":
			postings = append(postings, jobPost{ID: l.ID, Company: l.Company, Title: l.JobTitle})
		}
	}
	for len(postings) > 0 {
		batch := postings[:min(len(postings), maxHiringScreened)]
		postings = postings[len(batch):]
		fits, err := screen(screenInput(batch))
		if err != nil {
			return nil, 0, err
		}
		_, dropped := screenedJobs(batch, fits)
		if len(batch) >= rescreenReviewBatch && len(dropped) == len(batch) {
			review += len(batch)
			continue
		}
		for _, j := range dropped {
			reasons[j.ID] = rescreenPostingReason
		}
	}
	return reasons, review, nil
}

// rescreen checks the workspace's company leads of the last 30 days again and
// archives the ones that fail, recording why. Who asked is in the ops audit
// log, not on the lead: the customer can read a lead's metadata. It returns
// how many leads it checked, how many it archived and how many it left for a
// person to review (a whole batch the screen rejected). Leads the customer acted on
// are left alone, and so is everything when the run cannot finish; a second
// run finds only the leads that passed.
func (m *Monitor) rescreen(ctx context.Context, ws string) (checked, archived, review int, err error) {
	bp, err := m.q.GetBrandProfile(ctx, ws)
	if err != nil || bp.Product == "" {
		return 0, 0, 0, errors.New("the workspace has not finished onboarding")
	}
	st, ok := m.loadHiringState(ctx, ws)
	if !ok {
		return 0, 0, 0, errors.New("the workspace's hiring plan could not be read")
	}
	leads, err := m.q.ListCompanyLeadsToRescreen(ctx, ws, time.Now().Add(-rescoreWindow))
	if err != nil {
		m.logger.Error().Err(err).Str("workspace_id", ws).Msg("rescreen: list leads")
		return 0, 0, 0, errors.New("the workspace's leads could not be read")
	}
	reasons, review, err := rescreenReasons(leads, func(jobs []ai.HiringJob) ([]ai.ProspectFit, error) {
		// A plan that does not apply has no roles: every posting would fail.
		if st.Plan == nil || !st.Plan.Applies {
			return nil, errors.New("no hiring plan")
		}
		return m.hiringPlanner.ScreenHiringJobs(ctx, ws, ai.HiringScreen{
			Product: bp.Product, Overview: bp.Overview, Audience: bp.TargetAudience,
			Roles: st.Plan.Roles, MaxCompanySize: st.Plan.MaxCompanySize, MinCompanySize: st.Plan.MinSize(), Jobs: jobs,
		})
	})
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", ws).Msg("rescreen: screen failed")
		return len(leads), 0, 0, errors.New("the posting screen failed, so nothing was changed")
	}
	at := time.Now().UTC().Format(time.RFC3339)
	for _, l := range leads {
		reason, failed := reasons[l.ID]
		if !failed {
			continue
		}
		done, err := m.q.ArchiveRescreenedLead(ctx, ws, l.ID, jsonBytes(map[string]any{"reason": reason, "at": at}))
		if err != nil {
			m.logger.Error().Err(err).Str("mention_id", l.ID).Msg("rescreen: archive lead")
			return len(leads), archived, review, errors.New("a lead could not be archived; ask again to carry on")
		}
		if done {
			archived++
			m.logger.Info().Str("workspace_id", ws).Str("company", l.Company).Str("why", reason).Msg("rescreen: lead archived")
		}
	}
	return len(leads), archived, review, nil
}
