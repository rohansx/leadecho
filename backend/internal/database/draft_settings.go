package database

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Draft settings: what the customer told us about how their drafts are
// written, under workspaces.settings.drafts. Hand-written (no sqlc codegen).

const (
	DraftSettingsKey      = "drafts"
	autoDraftCountKey     = "auto_drafts"
	DefaultAutoDraftDaily = 5
	MaxAutoDraftDaily     = 20
	maxProofPointLen      = 400
	maxSignOffLen         = 200
)

// DraftSettings holds the only proof point and sign-off a draft may use: a
// draft never makes one up, so empty means "none".
type DraftSettings struct {
	// ProofPoint is one result or customer the product can stand behind, in
	// the customer's words ("Acme cut interview hours by a third").
	ProofPoint string `json:"proof_point"`
	// SignOff closes an outreach email ("Rohan\nFounder, Utkrusht").
	SignOff string `json:"sign_off"`
	// PublicReplyLinks puts the tracked short link in public replies. Off
	// unless the workspace switched it on.
	PublicReplyLinks bool `json:"public_reply_links"`
	// AutoDraftDaily is how many replies a day are drafted without being
	// asked for (free). nil = DefaultAutoDraftDaily; 0 = off.
	AutoDraftDaily *int `json:"auto_draft_daily,omitempty"`
}

// AutoDraftLimit is the day's number of unasked-for drafts.
func (s DraftSettings) AutoDraftLimit() int {
	if s.AutoDraftDaily == nil {
		return DefaultAutoDraftDaily
	}
	return min(max(*s.AutoDraftDaily, 0), MaxAutoDraftDaily)
}

// Clean trims the texts and rejects values out of range.
func (s DraftSettings) Clean() (DraftSettings, error) {
	s.ProofPoint = strings.Join(strings.Fields(s.ProofPoint), " ")
	s.SignOff = strings.TrimSpace(s.SignOff)
	if utf8.RuneCountInString(s.ProofPoint) > maxProofPointLen {
		return s, errors.New("proof_point must be 400 characters or fewer")
	}
	if utf8.RuneCountInString(s.SignOff) > maxSignOffLen {
		return s, errors.New("sign_off must be 200 characters or fewer")
	}
	if s.AutoDraftDaily != nil && (*s.AutoDraftDaily < 0 || *s.AutoDraftDaily > MaxAutoDraftDaily) {
		return s, errors.New("auto_draft_daily must be between 0 and 20")
	}
	return s, nil
}

// GetDraftSettings returns the workspace's draft settings; the zero value
// (no proof point, no sign-off, no links, default auto-drafts) when none are
// stored or they cannot be read.
func (q *Queries) GetDraftSettings(ctx context.Context, workspaceID string) DraftSettings {
	var s DraftSettings
	if raw, err := q.GetWorkspaceSettingsKey(ctx, workspaceID, DraftSettingsKey); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &s)
	}
	return s
}

// SetDraftSettings stores the workspace's draft settings.
func (q *Queries) SetDraftSettings(ctx context.Context, workspaceID string, s DraftSettings) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return q.SetWorkspaceSettingsKey(ctx, workspaceID, DraftSettingsKey, raw)
}

type autoDraftCount struct {
	Day string `json:"day"`
	N   int    `json:"n"`
}

// nextAutoDraftCount is the day's counter after taking (delta 1) or giving
// back (delta -1) one auto-draft; ok is false when the day's limit is used up.
func nextAutoDraftCount(c autoDraftCount, today string, delta, limit int) (autoDraftCount, bool) {
	if c.Day != today {
		c = autoDraftCount{Day: today}
	}
	if delta > 0 && c.N >= limit {
		return c, false
	}
	c.N = max(c.N+delta, 0)
	return c, true
}

// TakeAutoDraft claims one of the workspace's auto-drafts for today (UTC)
// and reports whether one was left. Safe under concurrent scoring: the
// counter is only replaced if nobody changed it in between.
func (q *Queries) TakeAutoDraft(ctx context.Context, workspaceID string, limit int) (bool, error) {
	return q.moveAutoDraftCount(ctx, workspaceID, 1, limit)
}

// ReturnAutoDraft gives back a claim whose draft was not written.
func (q *Queries) ReturnAutoDraft(ctx context.Context, workspaceID string) {
	_, _ = q.moveAutoDraftCount(ctx, workspaceID, -1, 0)
}

func (q *Queries) moveAutoDraftCount(ctx context.Context, workspaceID string, delta, limit int) (bool, error) {
	today := time.Now().UTC().Format(time.DateOnly)
	for attempt := 0; attempt < 5; attempt++ {
		old, err := q.GetWorkspaceSettingsKey(ctx, workspaceID, autoDraftCountKey)
		if err != nil {
			return false, err
		}
		var c autoDraftCount
		if len(old) > 0 {
			_ = json.Unmarshal(old, &c)
		}
		next, ok := nextAutoDraftCount(c, today, delta, limit)
		if !ok {
			return false, nil
		}
		raw, _ := json.Marshal(next)
		if swapped, err := q.SwapWorkspaceSettingsKey(ctx, workspaceID, autoDraftCountKey, old, raw); err != nil {
			return false, err
		} else if swapped {
			return true, nil
		}
	}
	return false, nil // contended five times: skip this one rather than overshoot
}
