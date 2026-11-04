package digest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/auth"
	"leadecho/internal/database"
	"leadecho/internal/netsafe"
)

// Summarizer writes the weekly AI paragraph (llm.Router). Optional: without
// one, or when it fails (e.g. no provider key in dev), TemplateSummary is used.
type Summarizer interface {
	SummarizeDigest(ctx context.Context, workspaceID, facts string) (string, error)
}

type Service struct {
	q           *database.Queries
	llm         Summarizer
	mail        auth.Resend
	frontendURL string
	logger      zerolog.Logger

	// overridable in tests
	now         func() time.Time
	slackClient *http.Client
	slackOK     func(string) bool
}

func NewService(q *database.Queries, llm Summarizer, resendAPIKey, fromEmail, frontendURL string, logger zerolog.Logger) *Service {
	return &Service{
		q: q, llm: llm, mail: auth.Resend{APIKey: resendAPIKey, From: fromEmail}, frontendURL: strings.TrimRight(frontendURL, "/"),
		logger:      logger.With().Str("component", "digest").Logger(),
		now:         time.Now,
		slackClient: netsafe.Client(8 * time.Second),
		slackOK:     slackURLAllowed,
	}
}

// Settings returns the workspace schedule (defaults when never saved); an
// empty recipient list is shown as the workspace owner's email.
func (s *Service) Settings(ctx context.Context, wsID string) (Schedule, error) {
	sched := DefaultSchedule()
	row, err := s.q.GetDigestSettings(ctx, wsID)
	switch {
	case err == nil:
		sched = fromRow(row.DailyEnabled, row.DailyTime, row.WeeklyEnabled, row.WeeklyDay, row.Timezone, row.Recipients, row.Channels)
	case !errors.Is(err, pgx.ErrNoRows):
		return Schedule{}, fmt.Errorf("load digest settings: %w", err)
	}
	if len(sched.Recipients) == 0 {
		if owner, err := s.q.GetWorkspaceOwnerEmail(ctx, wsID); err == nil {
			sched.Recipients = []string{owner}
		}
	}
	return sched, nil
}

// SaveSettings validates and stores the schedule.
func (s *Service) SaveSettings(ctx context.Context, wsID string, in Schedule) (Schedule, error) {
	sched, err := in.Normalize()
	if err != nil {
		return Schedule{}, err
	}
	row, err := s.q.UpsertDigestSettings(ctx, database.UpsertDigestSettingsParams{
		WorkspaceID: wsID, DailyEnabled: sched.DailyEnabled, DailyTime: sched.DailyTime,
		WeeklyEnabled: sched.WeeklyEnabled, WeeklyDay: sched.WeeklyDay, Timezone: sched.Timezone,
		Recipients: sched.Recipients, Channels: sched.Channels,
	})
	if err != nil {
		return Schedule{}, fmt.Errorf("save digest settings: %w", err)
	}
	return fromRow(row.DailyEnabled, row.DailyTime, row.WeeklyEnabled, row.WeeklyDay, row.Timezone, row.Recipients, row.Channels), nil
}

func fromRow(dailyOn bool, dailyTime string, weeklyOn bool, weeklyDay, tz string, recipients, channels []string) Schedule {
	return Schedule{
		DailyEnabled: dailyOn, DailyTime: dailyTime, WeeklyEnabled: weeklyOn, WeeklyDay: weeklyDay,
		Timezone: tz, Recipients: recipients, Channels: channels,
	}
}

// Preview renders the digest for the trailing day/week without sending it.
func (s *Service) Preview(ctx context.Context, wsID string, kind Kind) (string, error) {
	d, err := s.Build(ctx, wsID, kind, TrailingPeriod(kind, s.now()))
	if err != nil {
		return "", err
	}
	return RenderHTML(s.withSummary(ctx, wsID, d))
}

// withSummary adds the weekly paragraph: LLM first, deterministic template fallback.
func (s *Service) withSummary(ctx context.Context, wsID string, d Data) Data {
	if d.Kind != KindWeekly {
		return d
	}
	d.Summary = TemplateSummary(d)
	if s.llm == nil {
		return d
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if text, err := s.llm.SummarizeDigest(ctx, wsID, Facts(d)); err == nil && strings.TrimSpace(text) != "" {
		d.Summary = strings.TrimSpace(text)
	} else if err != nil {
		s.logger.Debug().Err(err).Str("workspace_id", wsID).Msg("digest: llm summary unavailable, using template")
	}
	return d
}

// Run checks for due digests every interval until ctx is done.
func (s *Service) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		s.RunDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunDue sends every digest that is due now. Each (workspace, kind, period)
// is claimed in digest_runs first, so concurrent processes and repeated ticks
// send it once.
// ponytail: sequential over all workspaces; fan out if tick time grows past the interval.
func (s *Service) RunDue(ctx context.Context) {
	rows, err := s.q.ListDigestSchedules(ctx)
	if err != nil {
		s.logger.Error().Err(err).Msg("digest: list schedules failed")
		return
	}
	now := s.now()
	for _, r := range rows {
		sched := fromRow(r.DailyEnabled, r.DailyTime, r.WeeklyEnabled, r.WeeklyDay, r.Timezone, r.Recipients, r.Channels)
		if len(sched.Channels) == 0 {
			continue
		}
		for _, kind := range []Kind{KindDaily, KindWeekly} {
			if p, due := sched.Due(kind, now); due {
				s.runOne(ctx, r.WorkspaceID, sched, kind, p)
			}
		}
	}
}

func (s *Service) runOne(ctx context.Context, wsID string, sched Schedule, kind Kind, p Period) {
	runID, err := s.q.ClaimDigestRun(ctx, database.ClaimDigestRunParams{
		WorkspaceID: wsID, Kind: string(kind), PeriodStart: p.Start, PeriodEnd: p.End,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return // already sent (or being sent) by someone else
	}
	if err != nil {
		s.logger.Error().Err(err).Str("workspace_id", wsID).Msg("digest: claim failed")
		return
	}
	status, msg := s.buildAndDeliver(ctx, wsID, sched, kind, p)
	if err := s.q.FinishDigestRun(ctx, database.FinishDigestRunParams{
		ID: runID, WorkspaceID: wsID, Status: status, Error: msg,
	}); err != nil {
		s.logger.Error().Err(err).Str("run_id", runID).Msg("digest: finish failed")
	}
	s.logger.Info().Str("workspace_id", wsID).Str("kind", string(kind)).Str("status", status).Str("note", msg).Msg("digest run")
}

func (s *Service) buildAndDeliver(ctx context.Context, wsID string, sched Schedule, kind Kind, p Period) (status, msg string) {
	d, err := s.Build(ctx, wsID, kind, p)
	if err != nil {
		return "failed", err.Error()
	}
	if d.Empty() {
		return "skipped", "nothing happened in this period"
	}
	return s.deliver(ctx, wsID, sched, s.withSummary(ctx, wsID, d))
}

// deliver sends on each channel the user chose. A run is "sent" when any
// channel delivered; "failed" (and retried) only when none did and one errored.
func (s *Service) deliver(ctx context.Context, wsID string, sched Schedule, d Data) (status, msg string) {
	var delivered int
	var notes []string
	var failed bool
	if sched.HasChannel("email") {
		ok, note, err := s.sendEmail(ctx, wsID, sched.Recipients, d)
		delivered, failed = delivered+ok, failed || err != nil
		notes = appendNote(notes, note, err)
	}
	if sched.HasChannel("slack") {
		ok, note, err := s.sendSlack(ctx, wsID, d)
		delivered, failed = delivered+ok, failed || err != nil
		notes = appendNote(notes, note, err)
	}
	msg = strings.Join(notes, "; ")
	switch {
	case delivered > 0:
		return "sent", msg
	case failed:
		return "failed", msg
	default:
		return "skipped", msg
	}
}

func appendNote(notes []string, note string, err error) []string {
	if err != nil {
		return append(notes, err.Error())
	}
	if note != "" {
		return append(notes, note)
	}
	return notes
}

func (s *Service) sendEmail(ctx context.Context, wsID string, recipients []string, d Data) (int, string, error) {
	if !s.mail.Enabled() {
		return 0, "email: RESEND_API_KEY not set", nil
	}
	if len(recipients) == 0 {
		owner, err := s.q.GetWorkspaceOwnerEmail(ctx, wsID)
		if err != nil {
			return 0, "email: no recipient", nil
		}
		recipients = []string{owner}
	}
	body, err := RenderHTML(d)
	if err != nil {
		return 0, "", err
	}
	subject := Subject(d)
	sendErr := s.mail.Send(ctx, recipients, subject, body)
	s.logNotification(ctx, wsID, database.NotificationChannelEmail, strings.Join(recipients, ","), subject, body, sendErr == nil)
	if sendErr != nil {
		return 0, "", fmt.Errorf("email: %w", sendErr)
	}
	return 1, "", nil
}

func (s *Service) sendSlack(ctx context.Context, wsID string, d Data) (int, string, error) {
	raw, err := s.q.GetWorkspaceSettings(ctx, wsID)
	if err != nil {
		return 0, "", fmt.Errorf("slack: load settings: %w", err)
	}
	url := slackURLFromSettings(raw)
	if url == "" || !s.slackOK(url) {
		return 0, "slack: no valid webhook on the Alerts page", nil
	}
	text := RenderSlack(d)
	sendErr := postSlack(ctx, s.slackClient, url, text)
	s.logNotification(ctx, wsID, database.NotificationChannelSlack, url, Subject(d), text, sendErr == nil)
	if sendErr != nil {
		return 0, "", fmt.Errorf("slack: %w", sendErr)
	}
	return 1, "", nil
}

func (s *Service) logNotification(ctx context.Context, wsID string, ch database.NotificationChannel, recipient, subject, body string, sent bool) {
	sentAt := pgtype.Timestamptz{}
	if sent {
		sentAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
	}
	if _, err := s.q.CreateNotification(ctx, database.CreateNotificationParams{
		WorkspaceID: wsID, Channel: ch, Recipient: recipient,
		Subject: pgtype.Text{String: subject, Valid: true}, Body: body, Metadata: []byte(`{"kind":"digest"}`), SentAt: sentAt,
	}); err != nil {
		s.logger.Warn().Err(err).Msg("digest: notification log failed")
	}
}
