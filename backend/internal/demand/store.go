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
	maxURLLen = 500
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

// ErrBusy: the stored themes kept changing while an update was applied.
var ErrBusy = errors.New("demand: themes are being updated, try again")

// UpdateState reads the stored state, applies fn and writes the result only
// if nothing changed in between; otherwise it starts over on the new state.
func (d DBStore) UpdateState(ctx context.Context, ws string, fn func(State) (State, error)) (State, error) {
	for range 5 {
		raw, err := d.Q.GetWorkspaceSettingsKey(ctx, ws, SettingsKey)
		if err != nil {
			return State{}, err
		}
		var cur State
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &cur); err != nil {
				return State{}, err
			}
		}
		next, err := fn(cur)
		if err != nil {
			return State{}, err
		}
		out, err := json.Marshal(next)
		if err != nil {
			return State{}, err
		}
		ok, err := d.Q.SwapWorkspaceSettingsKey(ctx, ws, SettingsKey, raw, out)
		if err != nil {
			return State{}, err
		}
		if ok {
			return next, nil
		}
	}
	return State{}, ErrBusy
}

func (d DBStore) RecentAsks(ctx context.Context, ws string) ([]Ask, error) {
	rows, err := d.Q.ListMentionAsks(ctx, ws, time.Now().Add(-askWindow), MinRelevance, maxAsks)
	if err != nil {
		return nil, err
	}
	out := make([]Ask, len(rows))
	for i, r := range rows {
		url := r.URL
		if len(url) > maxURLLen { // a clipped link is a broken link
			url = ""
		}
		out[i] = Ask{MentionID: r.MentionID, Quote: clip(r.Ask, 240), Buyer: r.AuthorRole == "buyer", Platform: r.Platform, URL: url, Author: clip(r.Author, 80), At: r.At}
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
