package keywordjudge

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"leadecho/internal/database"
)

// The judge acts on its own review: a keyword that has had a full week of
// posts is paused or moved when the judge says so, and the keywords it
// proposes are added (each gets its own week before it can be judged). The
// customer sees every change on the Keywords page and can undo it there.

const (
	// MinAge: a keyword is judged only after a week of posts.
	MinAge = 7 * 24 * time.Hour
	// MinActive: the judge never pauses a workspace below this many keywords.
	MinActive = 5
	// DecisionAuto marks a suggestion the judge applied itself.
	DecisionAuto = "auto"
)

type action struct {
	suggestionID string
	kind         string // pause, platforms, add
	kw           *database.ListKeywordsRow
	term         string
	platforms    []string
}

// plan picks the suggestions that may be applied now. leads is the lead
// count per keyword ID and platform in the review window; allowed are the
// platforms the workspace may use; room is how many keywords may still be
// added. st.Decisions must hold every decision already made (the customer's
// and the judge's), so a dismissed or undone change is never made again.
func plan(st Stored, kws []database.ListKeywordsRow, leads map[string]map[string]int, allowed []string, room int, now time.Time) []action {
	byTerm := map[string]*database.ListKeywordsRow{}
	active := 0
	for i := range kws {
		byTerm[strings.ToLower(strings.TrimSpace(kws[i].Term))] = &kws[i]
		if kws[i].IsActive {
			active++
		}
	}
	var out []action
	for _, s := range st.Suggestions {
		if st.Decisions[s.ID] != "" {
			continue
		}
		kw := byTerm[strings.ToLower(strings.TrimSpace(s.Term))]
		judged := kw != nil && kw.IsActive && now.Sub(kw.CreatedAt) >= MinAge
		switch s.Kind {
		case "pause":
			// Never a keyword that found a lead in the window, whatever the judge says.
			if judged && total(leads[kw.ID]) == 0 && active > MinActive {
				active--
				out = append(out, action{suggestionID: s.ID, kind: "pause", kw: kw})
			}
		case "platforms":
			ps := onlyAllowed(s.Platforms, allowed)
			// Never drop a platform that found a lead.
			if judged && len(ps) > 0 && !sameSet(ps, kw.Platforms) && keepsLeads(ps, leads[kw.ID]) {
				out = append(out, action{suggestionID: s.ID, kind: "platforms", kw: kw, platforms: ps})
			}
		case "add":
			ps := onlyAllowed(s.Platforms, allowed)
			if kw == nil && len(ps) > 0 && room > 0 && strings.TrimSpace(s.Term) != "" {
				room--
				byTerm[strings.ToLower(strings.TrimSpace(s.Term))] = &database.ListKeywordsRow{Term: s.Term} // no duplicate adds
				out = append(out, action{suggestionID: s.ID, kind: "add", term: strings.TrimSpace(s.Term), platforms: ps})
			}
		}
	}
	return out
}

func total(byPlatform map[string]int) int {
	n := 0
	for _, v := range byPlatform {
		n += v
	}
	return n
}

func keepsLeads(ps []string, byPlatform map[string]int) bool {
	for p, n := range byPlatform {
		if n > 0 && !slices.Contains(ps, p) {
			return false
		}
	}
	return true
}

func onlyAllowed(ps, allowed []string) []string {
	out := []string{}
	for _, p := range ps {
		if slices.Contains(allowed, p) && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !slices.Contains(b, x) {
			return false
		}
	}
	return true
}

// Apply carries out the review's suggestions that plan allows and records
// each as decided by the judge. keywordLimit is the plan's keyword cap (0 or
// less: no keywords are added). It returns a note for the ops console.
func Apply(ctx context.Context, q *database.Queries, wsID string, st *Stored, keywordLimit int, locked map[string]bool, now time.Time) (string, error) {
	kws, err := q.ListKeywords(ctx, wsID)
	if err != nil {
		return "", fmt.Errorf("list keywords: %w", err)
	}
	yield, err := q.ListKeywordYield(ctx, wsID, now.Add(-Window))
	if err != nil {
		return "", fmt.Errorf("keyword yield: %w", err)
	}
	leads := map[string]map[string]int{}
	for _, y := range yield {
		if leads[y.KeywordID] == nil {
			leads[y.KeywordID] = map[string]int{}
		}
		leads[y.KeywordID][y.Platform] += y.Leads
	}
	// The decisions as stored: the review just saved kept every earlier one.
	cur := *st
	if raw, err := q.GetKeywordReview(ctx, wsID); err == nil && len(raw) > 0 {
		var saved Stored
		if json.Unmarshal(raw, &saved) == nil && saved.Decisions != nil {
			cur.Decisions = saved.Decisions
		}
	}
	active := 0
	for _, k := range kws {
		if k.IsActive {
			active++
		}
	}
	profileID := ""
	if ps, err := q.ListActiveMonitoringProfiles(ctx, wsID); err == nil && len(ps) > 0 {
		profileID = ps[0].ID
	}
	room := 0
	if profileID != "" {
		room = keywordLimit - active
	}
	var paused, moved, added int
	var firstErr error
	for _, a := range plan(cur, kws, leads, Platforms(locked), room, now) {
		if err := applyOne(ctx, q, wsID, profileID, a); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s %q: %w", a.kind, a.term+termOf(a.kw), err)
			}
			continue // the others still go
		}
		if _, err := q.SetKeywordReviewDecision(ctx, wsID, a.suggestionID, DecisionAuto); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("record decision: %w", err)
		}
		switch a.kind {
		case "pause":
			paused++
		case "platforms":
			moved++
		case "add":
			added++
		}
	}
	return fmt.Sprintf("judge applied: %d paused, %d moved, %d added", paused, moved, added), firstErr
}

func termOf(k *database.ListKeywordsRow) string {
	if k == nil {
		return ""
	}
	return k.Term
}

func applyOne(ctx context.Context, q *database.Queries, wsID, profileID string, a action) error {
	if a.kind == "add" {
		_, err := q.CreateKeyword(ctx, database.CreateKeywordParams{
			WorkspaceID: wsID, ProfileID: profileID, Term: a.term, Platforms: a.platforms,
			IsActive: true, MatchType: "contains", NegativeTerms: []string{}, Subreddits: []string{},
		})
		return err
	}
	k := a.kw
	upd := database.UpdateKeywordParams{
		Term: k.Term, Platforms: k.Platforms, IsActive: k.IsActive, MatchType: k.MatchType,
		NegativeTerms: k.NegativeTerms, Subreddits: k.Subreddits, ID: k.ID, WorkspaceID: wsID,
	}
	if a.kind == "pause" {
		upd.IsActive = false
	} else {
		upd.Platforms = a.platforms
	}
	_, err := q.UpdateKeyword(ctx, upd)
	return err
}
