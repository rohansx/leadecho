package attribution

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

// Service records conversion outcomes and ties them back to replies/leads.
type Service struct {
	q      *database.Queries
	logger zerolog.Logger
}

func NewService(q *database.Queries, logger zerolog.Logger) *Service {
	return &Service{
		q:      q,
		logger: logger.With().Str("component", "attribution").Logger(),
	}
}

type ConversionRequest struct {
	UTMCode      string
	EventType    string // signup | purchase | click
	RevenueCents int32
	Metadata     map[string]any
}

// RecordConversion attaches a signup/purchase to a UTM link and advances linked leads.
func (s *Service) RecordConversion(ctx context.Context, workspaceID string, req ConversionRequest) (database.UtmLink, error) {
	if req.EventType == "" {
		req.EventType = "signup"
	}
	if req.EventType != "signup" && req.EventType != "purchase" && req.EventType != "click" {
		return database.UtmLink{}, fmt.Errorf("invalid event_type")
	}

	link, err := s.q.GetUTMLinkByCode(ctx, req.UTMCode)
	if err != nil {
		return database.UtmLink{}, fmt.Errorf("utm link not found")
	}
	if link.WorkspaceID != workspaceID {
		return database.UtmLink{}, fmt.Errorf("utm link not found")
	}

	var updated database.UtmLink
	if req.EventType == "click" {
		if err := s.q.IncrementUTMClicks(ctx, req.UTMCode); err != nil {
			return database.UtmLink{}, err
		}
		updated = link
	} else {
		updated, err = s.q.RecordUTMConversion(ctx, database.RecordUTMConversionParams{
			Code:         req.UTMCode,
			RevenueCents: req.RevenueCents,
		})
		if err != nil {
			return database.UtmLink{}, err
		}
	}

	meta, _ := json.Marshal(req.Metadata)
	if meta == nil {
		meta = []byte("{}")
	}
	_, _ = s.q.CreateUTMEvent(ctx, database.CreateUTMEventParams{
		UtmLinkID:    link.ID,
		EventType:    req.EventType,
		Referrer:     pgtype.Text{},
		UserAgent:    pgtype.Text{},
		IpHash:       pgtype.Text{},
		RevenueCents: pgtype.Int4{Int32: req.RevenueCents, Valid: req.RevenueCents > 0},
		Metadata:     meta,
	})

	s.advanceLeadFromUTM(ctx, workspaceID, link, req.EventType)
	return updated, nil
}

func (s *Service) advanceLeadFromUTM(ctx context.Context, workspaceID string, link database.UtmLink, eventType string) {
	if !link.UtmContent.Valid {
		return
	}
	reply, err := s.q.GetReply(ctx, database.GetReplyParams{
		ID:          link.UtmContent.String,
		WorkspaceID: workspaceID,
	})
	if err != nil {
		return
	}

	// Attach conversion outcome on reply metadata for analytics feedback loops.
	meta := map[string]any{}
	if len(reply.Metadata) > 0 {
		_ = json.Unmarshal(reply.Metadata, &meta)
	}
	meta["last_conversion"] = map[string]any{
		"event_type": eventType,
		"utm_code":   link.Code,
	}
	if b, err := json.Marshal(meta); err == nil {
		_, _ = s.q.UpdateReplyMetadata(ctx, database.UpdateReplyMetadataParams{
			ID:          reply.ID,
			WorkspaceID: workspaceID,
			Metadata:    b,
		})
	}

	lead, err := s.q.GetLeadByMention(ctx, database.GetLeadByMentionParams{
		MentionID:   parseUUID(reply.MentionID),
		WorkspaceID: workspaceID,
	})
	if err != nil {
		return
	}
	if lead.Stage == database.LeadStageConverted || lead.Stage == database.LeadStageLost {
		return
	}
	prev := lead.Stage
	updatedLead, err := s.q.UpdateLeadStage(ctx, database.UpdateLeadStageParams{
		Stage:       database.LeadStageConverted,
		ID:          lead.ID,
		WorkspaceID: workspaceID,
	})
	if err != nil {
		return
	}
	_, _ = s.q.CreateLeadEvent(ctx, database.CreateLeadEventParams{
		LeadID:        updatedLead.ID,
		PreviousStage: database.NullLeadStage{LeadStage: prev, Valid: true},
		NewStage:      database.LeadStageConverted,
		Notes:         pgtype.Text{String: "utm_" + eventType, Valid: true},
	})
}

func parseUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if len(s) != 36 {
		return u
	}
	hexVal := func(c byte) (byte, bool) {
		switch {
		case '0' <= c && c <= '9':
			return c - '0', true
		case 'a' <= c && c <= 'f':
			return c - 'a' + 10, true
		case 'A' <= c && c <= 'F':
			return c - 'A' + 10, true
		}
		return 0, false
	}
	dst := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			continue
		}
		hi, ok1 := hexVal(s[i])
		lo, ok2 := hexVal(s[i+1])
		if !ok1 || !ok2 {
			return pgtype.UUID{}
		}
		u.Bytes[dst] = hi<<4 | lo
		dst++
		i++
	}
	u.Valid = true
	return u
}
