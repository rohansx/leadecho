package tools

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

const (
	defaultLimit   = 20
	maxLimit       = 100
	maxContentView = 2000 // chars of mention content returned to agents
)

// Enum mirrors: values outside these sets would fail the Postgres enum cast.
var (
	mentionStatuses = set("new", "reviewed", "replied", "archived", "spam")
	mentionIntents  = set("buy_signal", "complaint", "recommendation_ask", "comparison", "general")
	leadStages      = set("prospect", "qualified", "engaged", "converted", "lost")
	platforms       = set("reddit", "hackernews", "devto", "lobsters", "indiehackers", "twitter",
		"linkedin", "quora", "exa", "googlenews")
	defaultPlatforms = []string{"hackernews", "reddit", "twitter", "linkedin"}
)

func set(vals ...string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

func pageArgs(limit, offset int32) (int32, int32, error) {
	if offset < 0 {
		return 0, 0, userErr("offset must be >= 0")
	}
	switch {
	case limit <= 0:
		limit = defaultLimit
	case limit > maxLimit:
		limit = maxLimit
	}
	return limit, offset, nil
}

func requireUUID(field, v string) error {
	var u pgtype.UUID
	if u.Scan(v) != nil || !u.Valid {
		return userErr("%s must be a UUID", field)
	}
	return nil
}

func checkLen(field, v string, max int) error {
	if utf8.RuneCountInString(v) > max {
		return userErr("%s must be at most %d characters", field, max)
	}
	return nil
}

// notFound maps pgx.ErrNoRows to a caller-facing error and passes others through.
func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return userErr("%s not found", what)
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func uuidStr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	v, _ := u.Value()
	s, _ := v.(string)
	return &s
}

type MentionView struct {
	ID        string    `json:"id"`
	Platform  string    `json:"platform"`
	URL       string    `json:"url"`
	Title     *string   `json:"title,omitempty"`
	Content   string    `json:"content"`
	Author    *string   `json:"author,omitempty"`
	Score     *float32  `json:"relevance_score,omitempty"`
	Intent    *string   `json:"intent,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func mentionView(m database.Mention) MentionView {
	v := MentionView{
		ID: m.ID, Platform: m.Platform, URL: m.Url, Title: textPtr(m.Title),
		Content: truncate(m.Content, maxContentView), Author: textPtr(m.AuthorUsername),
		Status: string(m.Status), CreatedAt: m.CreatedAt,
	}
	if m.RelevanceScore.Valid {
		v.Score = &m.RelevanceScore.Float32
	}
	if m.Intent.Valid {
		s := string(m.Intent.IntentType)
		v.Intent = &s
	}
	return v
}

func mentionViews(ms []database.Mention) []MentionView {
	out := make([]MentionView, len(ms))
	for i, m := range ms {
		out[i] = mentionView(m)
	}
	return out
}

type LeadView struct {
	ID             string    `json:"id"`
	Stage          string    `json:"stage"`
	MentionID      *string   `json:"mention_id,omitempty"`
	ContactName    *string   `json:"contact_name,omitempty"`
	ContactEmail   *string   `json:"contact_email,omitempty"`
	Company        *string   `json:"company,omitempty"`
	Username       *string   `json:"username,omitempty"`
	Platform       *string   `json:"platform,omitempty"`
	ProfileURL     *string   `json:"profile_url,omitempty"`
	EstimatedValue *int32    `json:"estimated_value,omitempty"`
	Notes          *string   `json:"notes,omitempty"`
	Tags           []string  `json:"tags"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func leadView(l database.Lead) LeadView {
	v := LeadView{
		ID: l.ID, Stage: string(l.Stage), MentionID: uuidStr(l.MentionID),
		ContactName: textPtr(l.ContactName), ContactEmail: textPtr(l.ContactEmail),
		Company: textPtr(l.Company), Username: textPtr(l.Username), ProfileURL: textPtr(l.ProfileUrl),
		Notes: textPtr(l.Notes), Tags: l.Tags, CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	if l.Platform.Valid {
		s := string(l.Platform.PlatformType)
		v.Platform = &s
	}
	if l.EstimatedValue.Valid {
		v.EstimatedValue = &l.EstimatedValue.Int32
	}
	return v
}

type ReplyView struct {
	ID        string `json:"id"`
	MentionID string `json:"mention_id"`
	Content   string `json:"content"` // edited content when present
	Status    string `json:"status"`
}

func replyView(r database.Reply) ReplyView {
	c := r.Content
	if r.EditedContent.Valid && r.EditedContent.String != "" {
		c = r.EditedContent.String
	}
	return ReplyView{ID: r.ID, MentionID: r.MentionID, Content: c, Status: string(r.Status)}
}

type ListenerView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	LooksFor    string `json:"looks_for"`
	Skips       string `json:"skips"`
	IsActive    bool   `json:"is_active"`
}

func listenerView(p database.MonitoringProfile) ListenerView {
	return ListenerView{ID: p.ID, Name: p.Name, Description: p.Description, LooksFor: p.LooksFor, Skips: p.Skips, IsActive: p.IsActive}
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}

// cleanTerms trims, drops blanks and case-insensitive duplicates.
func cleanTerms(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range in {
		t = strings.TrimSpace(t)
		k := strings.ToLower(t)
		if t == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out
}
