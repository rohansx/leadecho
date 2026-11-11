package demand

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

const (
	// SettingsKey is where a workspace's themes live (workspaces.settings).
	SettingsKey = "demand"
	// askWindow is how far back asks are picked up for grouping.
	askWindow = 30 * 24 * time.Hour
	maxAsks   = 400
)

// DBStore is the Store backed by the LeadEcho database.
type DBStore struct{ Q *database.Queries }

func (d DBStore) LoadState(ctx context.Context, ws string) (State, error) {
	var s State
	raw, err := d.Q.GetWorkspaceSettingsKey(ctx, ws, SettingsKey)
	if err != nil {
		return s, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &s); err != nil {
			return State{}, err
		}
	}
	return s, nil
}

func (d DBStore) SaveState(ctx context.Context, ws string, s State) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return d.Q.SetWorkspaceSettingsKey(ctx, ws, SettingsKey, raw)
}

func (d DBStore) RecentAsks(ctx context.Context, ws string) ([]Ask, error) {
	rows, err := d.Q.ListMentionAsks(ctx, ws, time.Now().Add(-askWindow), maxAsks)
	if err != nil {
		return nil, err
	}
	out := make([]Ask, len(rows))
	for i, r := range rows {
		out[i] = Ask{MentionID: r.MentionID, Quote: r.Ask, Buyer: r.AuthorRole == "buyer", Platform: r.Platform, URL: r.URL, Author: r.Author, At: r.At}
	}
	return out, nil
}

func (d DBStore) Product(ctx context.Context, ws string) (Product, error) {
	bp, err := d.Q.GetBrandProfile(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, nil // not onboarded: themes are still grouped, coverage defaults to gap
	}
	if err != nil {
		return Product{}, err
	}
	return Product{Name: bp.Product, Overview: bp.Overview, Features: bp.Features}, nil
}

func (d DBStore) CreateDraft(ctx context.Context, ws, mentionID, text string) error {
	_, err := d.Q.CreateReply(ctx, database.CreateReplyParams{
		MentionID: mentionID, WorkspaceID: ws, Content: text, Status: database.ReplyStatusDraft,
		TemplateStyle: pgtype.Text{String: "follow_up", Valid: true},
	})
	return err
}
