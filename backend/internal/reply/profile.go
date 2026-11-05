package reply

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// ErrResponseProfileNotFound is returned when an explicit response_profile_id
// does not exist in the workspace.
var ErrResponseProfileNotFound = errors.New("response profile not found")

// EnsureDefaultResponseProfile returns the workspace's preferred response
// profile, creating the derived default (drive_conversion, brand plug low,
// tone from the brand profile) when the workspace has none. For onboarding.
func EnsureDefaultResponseProfile(ctx context.Context, q *database.Queries, wsID string) (database.ResponseProfile, error) {
	p, err := q.GetPreferredResponseProfile(ctx, wsID)
	if err == nil || !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	_, desc := brandOf(ctx, q, wsID)
	v := DerivedVoice(desc)
	return q.CreateResponseProfile(ctx, database.CreateResponseProfileParams{
		WorkspaceID: wsID, Name: "Default", Goal: v.Goal, Tone: int32(v.Tone), Length: v.Length,
		Assertiveness: int32(v.Assertiveness), BrandPlug: v.BrandPlug, Examples: []byte("{}"),
	})
}

// brandOf returns the first monitoring profile's name and description.
func brandOf(ctx context.Context, q *database.Queries, wsID string) (name, desc string) {
	profiles, err := q.ListMonitoringProfiles(ctx, wsID)
	if err != nil || len(profiles) == 0 {
		return "", ""
	}
	return profiles[0].Name, profiles[0].Description
}

// resolveVoice picks the explicit profile, else the workspace default, else a
// derived voice. Only an explicit unknown id is an error — drafting is never gated.
func (d *Drafter) resolveVoice(ctx context.Context, wsID, profileID, brandDesc string) (Voice, error) {
	var (
		p   database.ResponseProfile
		err error
	)
	if profileID != "" {
		p, err = d.q.GetResponseProfile(ctx, database.GetResponseProfileParams{ID: profileID, WorkspaceID: wsID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
				return Voice{}, ErrResponseProfileNotFound
			}
			return Voice{}, fmt.Errorf("load response profile: %w", err)
		}
		return VoiceFromProfile(p), nil
	}
	p, err = d.q.GetPreferredResponseProfile(ctx, wsID)
	if err != nil {
		return DerivedVoice(brandDesc), nil
	}
	return VoiceFromProfile(p), nil
}

func isInvalidUUID(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "22P02"
}

// destinationFor is the profile's CTA URL, else the URL the workspace onboarded with.
func (d *Drafter) destinationFor(ctx context.Context, wsID string, v Voice) string {
	if v.DestinationURL != "" {
		return v.DestinationURL
	}
	if a, err := d.q.GetLatestOnboardingAnalysis(ctx, wsID); err == nil {
		return strings.TrimSpace(a.SourceUrl)
	}
	return ""
}

const shortCodeChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func shortCode() string {
	b := make([]byte, 8)
	for i := range b {
		b[i] = shortCodeChars[rand.IntN(len(shortCodeChars))]
	}
	return string(b)
}

// attachTrackingLink creates the reply's UTM link and puts its short URL in the
// draft. A failure keeps the unlinked draft — the link is a bonus, not a gate.
func (d *Drafter) attachTrackingLink(ctx context.Context, r database.Reply, v Voice) database.Reply {
	if v.BrandPlug == "none" {
		return r
	}
	dest := d.destinationFor(ctx, r.WorkspaceID, v)
	if dest == "" {
		return r
	}
	for attempt := 0; attempt < 5; attempt++ {
		link, err := d.q.CreateUTMLink(ctx, database.CreateUTMLinkParams{
			WorkspaceID: r.WorkspaceID, Code: shortCode(), DestinationUrl: dest,
			UtmSource: "leadecho", UtmMedium: "social_reply",
			UtmCampaign: pgtype.Text{String: r.MentionID, Valid: true},
			UtmContent:  pgtype.Text{String: r.ID, Valid: true},
		})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		if err != nil {
			return r
		}
		var id pgtype.UUID
		if id.Scan(link.ID) != nil {
			return r
		}
		linked, err := d.q.SetReplyTrackingLink(ctx, database.SetReplyTrackingLinkParams{
			Content: LinkedReply(r.Content, dest, d.shortBase+"/r/"+link.Code), UtmLinkID: id,
			ID: r.ID, WorkspaceID: r.WorkspaceID,
		})
		if err != nil {
			return r
		}
		return linked
	}
	return r
}
