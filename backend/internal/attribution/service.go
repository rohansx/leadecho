package attribution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/crmsync"
	"leadecho/internal/database"
)

// Service records conversion outcomes and ties them back to replies/leads.
type Service struct {
	q      *database.Queries
	logger zerolog.Logger
	crm    *crmsync.Dispatcher
}

func NewService(q *database.Queries, logger zerolog.Logger, crm *crmsync.Dispatcher) *Service {
	return &Service{
		crm:    crm,
		q:      q,
		logger: logger.With().Str("component", "attribution").Logger(),
	}
}

// ErrDuplicateConversion: this event_id was already recorded on the link.
var ErrDuplicateConversion = errors.New("conversion already recorded")

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

	meta := []byte("{}")
	if len(req.Metadata) > 0 {
		if meta, err = json.Marshal(req.Metadata); err != nil {
			return database.UtmLink{}, fmt.Errorf("invalid metadata")
		}
	}

	// The event row and the counter move together: a duplicate event_id (unique
	// index utm_events_link_event_id_unique) rolls the counter back too.
	updated := link
	err = s.q.InTx(ctx, func(q *database.Queries) error {
		if _, err := q.CreateUTMEvent(ctx, database.CreateUTMEventParams{
			UtmLinkID:    link.ID,
			EventType:    req.EventType,
			Referrer:     pgtype.Text{},
			UserAgent:    pgtype.Text{},
			IpHash:       pgtype.Text{},
			RevenueCents: pgtype.Int4{Int32: req.RevenueCents, Valid: req.RevenueCents > 0},
			Metadata:     meta,
		}); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return ErrDuplicateConversion
			}
			return fmt.Errorf("record utm event: %w", err)
		}
		if req.EventType == "click" {
			return q.IncrementUTMClicks(ctx, req.UTMCode)
		}
		var err error
		updated, err = q.RecordUTMConversion(ctx, database.RecordUTMConversionParams{
			Code:         req.UTMCode,
			RevenueCents: req.RevenueCents,
		})
		return err
	})
	if err != nil {
		if !errors.Is(err, ErrDuplicateConversion) {
			s.logger.Error().Err(err).Str("utm_code", req.UTMCode).Msg("record conversion")
		}
		return database.UtmLink{}, err
	}

	s.advanceLeadFromUTM(ctx, workspaceID, link, req.EventType, req.RevenueCents)
	return updated, nil
}

func (s *Service) advanceLeadFromUTM(ctx context.Context, workspaceID string, link database.UtmLink, eventType string, revenueCents int32) {
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
	if eventType != "click" {
		defer s.crm.Emit(workspaceID, crmsync.Event{
			Type: crmsync.EventConversion, LeadID: lead.ID, PrevStage: string(lead.Stage),
			ConvType: eventType, RevenueCents: revenueCents,
		})
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
