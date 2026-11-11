package digest

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

func pgUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(s)
	return u
}

// Only the best few leads get a drafted reply in the digest: each new draft
// costs credits, and a short list is what gets acted on.
const (
	maxDraftedLeads   = 3
	minDraftLeadScore = 6.0 // the scorer's lead threshold
)

// DraftFunc writes (and saves) a reply draft for a mention and returns its
// text; "" means the drafter decided the post isn't worth a reply.
type DraftFunc func(ctx context.Context, wsID, mentionID string) (string, error)

// SetDrafter lets the daily digest include a ready-to-post reply per top lead.
func (s *Service) SetDrafter(f DraftFunc) { s.draft = f }

// withActionables fills in who each top lead is and a drafted reply, so the
// digest can be acted on without opening the app. Failures leave the field
// empty: the digest still goes out.
func (s *Service) withActionables(ctx context.Context, wsID string, d Data) Data {
	if d.Kind != KindDaily {
		return d
	}
	leads := append([]Lead(nil), d.TopLeads...)
	for i := range leads {
		if i >= maxDraftedLeads || leads[i].Score < minDraftLeadScore {
			break // sorted by score, so the rest are lower
		}
		l := &leads[i]
		if lead, err := s.q.GetLeadByMention(ctx, database.GetLeadByMentionParams{MentionID: pgUUID(l.MentionID), WorkspaceID: wsID}); err == nil {
			l.Who = whoLine(lead.Metadata)
		}
		l.Draft = s.draftFor(ctx, wsID, l.MentionID)
	}
	d.TopLeads = leads
	return d
}

// draftFor reuses the newest saved draft and only writes a new one when
// there is none.
func (s *Service) draftFor(ctx context.Context, wsID, mentionID string) string {
	replies, err := s.q.ListRepliesByMention(ctx, database.ListRepliesByMentionParams{MentionID: mentionID, WorkspaceID: wsID})
	if err == nil && len(replies) > 0 {
		r := replies[0] // newest first
		if r.EditedContent.Valid && r.EditedContent.String != "" {
			return r.EditedContent.String
		}
		return r.Content
	}
	if s.draft == nil {
		return ""
	}
	text, err := s.draft(ctx, wsID, mentionID)
	if err != nil {
		s.logger.Warn().Err(err).Str("mention_id", mentionID).Msg("digest: reply draft failed")
		return ""
	}
	return text
}

// whoLine turns a lead's stored enrichment into "Jane Doe · VP Engineering ·
// Acme (51-200)". Without a title it falls back to the profile bio.
func whoLine(leadMeta []byte) string {
	var m struct {
		Enrichment struct {
			FullName  string `json:"full_name"`
			Title     string `json:"title"`
			Bio       string `json:"bio"`
			Company   string `json:"company"`
			Employees string `json:"employees"`
		} `json:"enrichment"`
	}
	if json.Unmarshal(leadMeta, &m) != nil {
		return ""
	}
	e := m.Enrichment
	company := e.Company
	if company != "" && e.Employees != "" {
		company += " (" + e.Employees + ")"
	}
	role := e.Title
	if role == "" {
		role = truncate(strings.Join(strings.Fields(e.Bio), " "), 100)
	}
	var parts []string
	for _, p := range []string{e.FullName, role, company} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}
