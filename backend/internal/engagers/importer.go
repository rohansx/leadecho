package engagers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/treg"
)

// Scorer is the slice of *researcher.Service used for imported leads.
type Scorer interface {
	ScoreLead(ctx context.Context, leadID, workspaceID string) error // Person360 + ICP, no paid calls
	EnrichLeadAsync(leadID, workspaceID string)                      // + treg enrichment
}

// Importer creates one mention + lead per engager (source=engager).
type Importer struct {
	q      *database.Queries
	treg   *treg.Client
	scorer Scorer
	logger zerolog.Logger
}

func NewImporter(q *database.Queries, c *treg.Client, scorer Scorer, logger zerolog.Logger) *Importer {
	return &Importer{q: q, treg: c, scorer: scorer, logger: logger}
}

func (i *Importer) Enabled() bool { return i != nil && i.treg.Enabled() }

// Row is one engager as shown in the dashboard table.
type Row struct {
	Engager
	LeadID      string   `json:"lead_id"`
	MentionID   string   `json:"mention_id"`
	ICPFitScore *float32 `json:"icp_fit_score"`
}

// Request is a validated POST /linkedin/engagers body.
type Request struct {
	PostURL string
	Ref     PostRef
	Kinds   []string
	Enrich  bool
}

func (i *Importer) Pull(ctx context.Context, ws string, req Request) ([]Row, error) {
	if !i.Enabled() {
		return nil, ErrDisabled
	}
	list, err := Fetch(ctx, i.treg, ws, req.Ref, req.Kinds)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(list))
	for _, e := range list {
		row, err := i.importOne(ctx, ws, req, e)
		if err != nil {
			i.logger.Warn().Err(err).Str("workspace_id", ws).Str("profile", e.ProfileURL).Msg("engagers: import failed")
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// engagerHeuristic: a comment is a stronger signal than a reaction.
// ponytail: fixed numbers; the comment text could go through the LLM scorer.
func engagerHeuristic(e Engager) (relevance, conversion float32) {
	if e.Comment != "" {
		return 6, 0.3
	}
	return 4, 0.15
}

func (i *Importer) importOne(ctx context.Context, ws string, req Request, e Engager) (Row, error) {
	content := e.Comment
	if content == "" {
		content = fmt.Sprintf("Reacted (%s) to %s", strings.ToLower(firstNonEmpty(e.Reaction, "like")), req.PostURL)
	}
	rel, conv := engagerHeuristic(e)
	meta, _ := json.Marshal(map[string]any{"source": "engager", "kind": e.Kind, "post_url": req.PostURL, "post_urn": req.Ref.URN, "headline": e.Headline, "reaction": e.Reaction})
	mentionID, err := i.q.UpsertEngagerMention(ctx, database.UpsertEngagerMentionParams{
		WorkspaceID:           ws,
		PlatformID:            engagerPlatformID(req.Ref, e),
		Url:                   req.PostURL,
		Content:               content,
		AuthorUsername:        txt(firstNonEmpty(e.PublicID, e.Name)),
		AuthorProfileUrl:      txt(e.ProfileURL),
		PlatformMetadata:      meta,
		RelevanceScore:        pgtype.Float4{Float32: rel, Valid: true},
		ConversionProbability: pgtype.Float4{Float32: conv, Valid: true},
	})
	if err != nil {
		return Row{}, fmt.Errorf("upsert mention: %w", err)
	}
	lead, err := i.ensureLead(ctx, ws, mentionID, req, e)
	if err != nil {
		return Row{}, err
	}
	row := Row{Engager: e, LeadID: lead.ID, MentionID: mentionID}
	if i.scorer != nil {
		if err := i.scorer.ScoreLead(ctx, lead.ID, ws); err != nil {
			i.logger.Debug().Err(err).Str("lead_id", lead.ID).Msg("engagers: score")
		}
		if req.Enrich {
			i.scorer.EnrichLeadAsync(lead.ID, ws)
		}
	}
	if p, err := i.q.GetPersonByLead(ctx, database.GetPersonByLeadParams{LeadID: lead.ID, WorkspaceID: ws}); err == nil && p.IcpFitScore.Valid {
		row.ICPFitScore = &p.IcpFitScore.Float32
	}
	return row, nil
}

func (i *Importer) ensureLead(ctx context.Context, ws, mentionID string, req Request, e Engager) (database.Lead, error) {
	mid := pgUUID(mentionID)
	lead, err := i.q.GetLeadByMention(ctx, database.GetLeadByMentionParams{MentionID: mid, WorkspaceID: ws})
	if err == nil {
		return lead, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return database.Lead{}, fmt.Errorf("get lead: %w", err)
	}
	meta, _ := json.Marshal(map[string]any{"source": "engager", "kind": e.Kind, "post_url": req.PostURL, "headline": e.Headline, "title": e.Title})
	lead, err = i.q.CreateLead(ctx, database.CreateLeadParams{
		WorkspaceID: ws,
		MentionID:   mid,
		Stage:       database.LeadStageProspect,
		ContactName: txt(e.Name),
		Company:     txt(e.Company),
		Username:    txt(firstNonEmpty(e.PublicID, e.Name)),
		Platform:    database.NullPlatformType{PlatformType: database.PlatformTypeLinkedin, Valid: true},
		ProfileUrl:  txt(e.ProfileURL),
		Tags:        []string{"engager", e.Kind},
		Metadata:    meta,
	})
	if err != nil {
		return database.Lead{}, fmt.Errorf("create lead: %w", err)
	}
	return lead, nil
}

// engagerPlatformID is stable per (post, person) so re-pulling a post updates
// rather than duplicates.
func engagerPlatformID(ref PostRef, e Engager) string {
	sum := sha256.Sum256([]byte(ref.URN + "|" + e.key()))
	return "linkedin_engager_" + hex.EncodeToString(sum[:12])
}

func txt(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func pgUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(s)
	return u
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
