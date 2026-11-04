package handler

import (
	"context"
	"fmt"
	"strings"

	"leadecho/internal/database"
)

// scoring: editable "Looks for" / "Skips" per listener (monitoring profile).

const maxListenerIntentLen = 2000

// mergeListenerIntent applies optional request values over the stored ones.
// nil = unchanged, "" = clear.
func mergeListenerIntent(p database.MonitoringProfile, looksFor, skips *string) (string, string, bool, error) {
	outLooks, outSkips, changed := p.LooksFor, p.Skips, false
	for _, f := range []struct {
		in   *string
		out  *string
		name string
	}{{looksFor, &outLooks, "looks_for"}, {skips, &outSkips, "skips"}} {
		if f.in == nil {
			continue
		}
		v := strings.TrimSpace(*f.in)
		if len(v) > maxListenerIntentLen {
			return "", "", false, fmt.Errorf("%s must be <= %d characters", f.name, maxListenerIntentLen)
		}
		*f.out, changed = v, true
	}
	return outLooks, outSkips, changed, nil
}

// saveListenerIntent persists looks_for / skips when the request set them.
func (h *ProfileHandler) saveListenerIntent(ctx context.Context, p database.MonitoringProfile, looksFor, skips *string) (database.MonitoringProfile, error) {
	looks, sk, changed, err := mergeListenerIntent(p, looksFor, skips)
	if err != nil || !changed {
		return p, err
	}
	return h.q.UpdateMonitoringProfileIntent(ctx, database.UpdateMonitoringProfileIntentParams{
		LooksFor: looks, Skips: sk, ID: p.ID, WorkspaceID: p.WorkspaceID,
	})
}
