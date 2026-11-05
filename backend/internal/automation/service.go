package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/auth"
	"leadecho/internal/database"
)

// RunTimeout bounds one playbook run.
const RunTimeout = 2 * time.Minute

// Querier is the data access the service needs (*database.Queries).
type Querier interface {
	Store
	ListEnabledAutomations(ctx context.Context) ([]database.Automation, error)
	ClaimScheduledAutomationRun(ctx context.Context, arg database.ClaimScheduledAutomationRunParams) (database.AutomationRun, error)
	CreateManualAutomationRun(ctx context.Context, arg database.CreateManualAutomationRunParams) (database.AutomationRun, error)
	FinishAutomationRun(ctx context.Context, arg database.FinishAutomationRunParams) (database.AutomationRun, error)
	SetAutomationLastRun(ctx context.Context, arg database.SetAutomationLastRunParams) error
	CreateNotification(ctx context.Context, arg database.CreateNotificationParams) (database.Notification, error)
	GetWorkspaceOwnerEmail(ctx context.Context, workspaceID string) (string, error)
}

// Service schedules, runs and delivers automations.
type Service struct {
	q           Querier
	runner      Runner
	mail        auth.Resend
	frontendURL string
	logger      zerolog.Logger
	now         func() time.Time // overridable in tests
}

func NewService(q Querier, tc ToolCaller, resendAPIKey, fromEmail, frontendURL string, logger zerolog.Logger) *Service {
	return &Service{
		q: q, runner: Runner{Tools: tc, Q: q},
		mail:        auth.Resend{APIKey: resendAPIKey, From: fromEmail},
		frontendURL: strings.TrimRight(frontendURL, "/"),
		logger:      logger.With().Str("component", "automations").Logger(),
		now:         time.Now,
	}
}

// Run checks for due automations every interval until ctx is done.
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

// RunDue runs every enabled automation whose scheduled occurrence is due.
// The (automation, occurrence) slot is claimed in automation_runs first, so
// repeated ticks and concurrent processes run each period once. A failed
// scheduled run is not retried; "Run now" is the retry.
// ponytail: sequential; fan out if a tick takes longer than the interval.
func (s *Service) RunDue(ctx context.Context) {
	rows, err := s.q.ListEnabledAutomations(ctx)
	if err != nil {
		s.logger.Error().Err(err).Msg("automations: list failed")
		return
	}
	for _, a := range rows {
		slot, due := ScheduleOf(a).Due(s.now())
		if !due {
			continue
		}
		run, err := s.q.ClaimScheduledAutomationRun(ctx, database.ClaimScheduledAutomationRunParams{
			WorkspaceID: a.WorkspaceID, AutomationID: a.ID, PeriodStart: slot,
			MinGapSeconds: MinGap(ScheduleOf(a)).Seconds(),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue // this period already ran (or is running)
		}
		if err != nil {
			s.logger.Error().Err(err).Str("automation_id", a.ID).Msg("automations: claim failed")
			continue
		}
		s.execute(ctx, a, run)
	}
}

// RunNow runs an automation immediately, outside its schedule.
func (s *Service) RunNow(ctx context.Context, a database.Automation) (database.AutomationRun, error) {
	run, err := s.q.CreateManualAutomationRun(ctx, database.CreateManualAutomationRunParams{WorkspaceID: a.WorkspaceID, AutomationID: a.ID})
	if err != nil {
		return database.AutomationRun{}, fmt.Errorf("create run: %w", err)
	}
	return s.execute(ctx, a, run), nil
}

// ScheduleOf reads the schedule columns of an automation row.
func ScheduleOf(a database.Automation) Schedule {
	return Schedule{Frequency: a.Frequency, Time: a.RunTime, Weekday: a.Weekday, Timezone: a.Timezone}
}

// ConfigOf parses the stored config and fills defaults for its kind.
func ConfigOf(a database.Automation) (Config, error) {
	var c Config
	if len(a.Config) > 0 {
		if err := json.Unmarshal(a.Config, &c); err != nil {
			return Config{}, fmt.Errorf("config: %w", err)
		}
	}
	return c.normalize(a.Kind)
}

func (s *Service) execute(ctx context.Context, a database.Automation, run database.AutomationRun) database.AutomationRun {
	started := s.now()
	out, err := s.runPlaybook(ctx, a, started)
	status, errMsg := "succeeded", ""
	if err != nil {
		s.logger.Error().Err(err).Str("automation_id", a.ID).Str("workspace_id", a.WorkspaceID).Msg("automations: run failed")
		status, errMsg = "failed", "run failed: "+publicMessage(err)
		out = Output{Summary: errMsg}
	}
	out.Notes = append(out.Notes, s.deliver(ctx, a, out, status)...)
	raw, _ := json.Marshal(out)
	done, ferr := s.q.FinishAutomationRun(ctx, database.FinishAutomationRunParams{
		ID: run.ID, WorkspaceID: a.WorkspaceID, Status: status, Output: raw, Error: errMsg,
	})
	if ferr != nil {
		s.logger.Error().Err(ferr).Str("run_id", run.ID).Msg("automations: finish failed")
		run.Status, run.Output, run.Error = status, raw, errMsg
		done = run
	}
	// last_run_at is the next scheduled run's "since": only a successful
	// scheduled run may advance it, or failed/manual runs would hide mentions.
	if status == "succeeded" && run.Trigger != "manual" {
		if err := s.q.SetAutomationLastRun(ctx, database.SetAutomationLastRunParams{
			ID: a.ID, WorkspaceID: a.WorkspaceID, LastRunAt: pgtype.Timestamptz{Time: started, Valid: true},
		}); err != nil {
			s.logger.Warn().Err(err).Str("automation_id", a.ID).Msg("automations: last_run update failed")
		}
	}
	s.logger.Info().Str("automation_id", a.ID).Str("kind", a.Kind).Str("status", status).Msg("automation run")
	return done
}

func (s *Service) runPlaybook(ctx context.Context, a database.Automation, started time.Time) (Output, error) {
	cfg, err := ConfigOf(a)
	if err != nil {
		return Output{}, err
	}
	since := started.Add(-ScheduleOf(a).Lookback())
	if a.LastRunAt.Valid {
		since = a.LastRunAt.Time
	}
	ctx, cancel := context.WithTimeout(ctx, RunTimeout)
	defer cancel()
	return s.runner.Run(ctx, a.WorkspaceID, a.Kind, cfg, since)
}
