package handler

// automations (P2 #17): CRUD, run now, run history, and the prompt library.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"leadecho/internal/api/middleware"
	"leadecho/internal/automation"
	"leadecho/internal/database"
	"leadecho/internal/skills"
)

const maxAutomationsPerWorkspace = 20

// AutomationRunner runs an automation now (*automation.Service).
type AutomationRunner interface {
	RunNow(ctx context.Context, a database.Automation) (database.AutomationRun, error)
}

type AutomationHandler struct {
	q      database.Querier
	runner AutomationRunner
}

func NewAutomationHandler(q database.Querier, runner AutomationRunner) *AutomationHandler {
	return &AutomationHandler{q: q, runner: runner}
}

type AutomationView struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Kind      string              `json:"kind"`
	Enabled   bool                `json:"enabled"`
	Schedule  automation.Schedule `json:"schedule"`
	Config    automation.Config   `json:"config"`
	LastRunAt *time.Time          `json:"last_run_at"`
	CreatedAt time.Time           `json:"created_at"`
}

type AutomationRunView struct {
	ID         string          `json:"id"`
	Trigger    string          `json:"trigger"`
	Status     string          `json:"status"`
	Output     json.RawMessage `json:"output"`
	Error      string          `json:"error"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at"`
}

func automationView(a database.Automation) AutomationView {
	cfg, _ := automation.ConfigOf(a) // stored rows were validated on write
	v := AutomationView{ID: a.ID, Name: a.Name, Kind: a.Kind, Enabled: a.Enabled,
		Schedule: automation.ScheduleOf(a), Config: cfg, CreatedAt: a.CreatedAt}
	if a.LastRunAt.Valid {
		v.LastRunAt = &a.LastRunAt.Time
	}
	return v
}

func automationRunView(r database.AutomationRun) AutomationRunView {
	v := AutomationRunView{ID: r.ID, Trigger: r.Trigger, Status: r.Status, Output: r.Output, Error: r.Error, StartedAt: r.StartedAt}
	if len(v.Output) == 0 {
		v.Output = json.RawMessage(`{}`)
	}
	if r.FinishedAt.Valid {
		v.FinishedAt = &r.FinishedAt.Time
	}
	return v
}

func decodeAutomationInput(w http.ResponseWriter, r *http.Request) (automation.Input, bool) {
	var in automation.Input
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return in, false
	}
	in, err := in.Normalize()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return in, false
	}
	return in, true
}

// load fetches the {id} automation in the caller's workspace, writing 400/404/500.
func (h *AutomationHandler) load(w http.ResponseWriter, r *http.Request) (database.Automation, bool) {
	id := chi.URLParam(r, "id")
	if !parseUUID(id).Valid {
		writeError(w, http.StatusBadRequest, "invalid automation id")
		return database.Automation{}, false
	}
	a, err := h.q.GetAutomation(r.Context(), database.GetAutomationParams{ID: id, WorkspaceID: middleware.WorkspaceID(r.Context())})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "automation not found")
		return a, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load automation")
		return a, false
	}
	return a, true
}

func (h *AutomationHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListAutomations(r.Context(), middleware.WorkspaceID(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list automations")
		return
	}
	out := make([]AutomationView, len(rows))
	for i, a := range rows {
		out[i] = automationView(a)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *AutomationHandler) Get(w http.ResponseWriter, r *http.Request) {
	if a, ok := h.load(w, r); ok {
		writeJSON(w, http.StatusOK, automationView(a))
	}
}

func (h *AutomationHandler) Create(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeAutomationInput(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	ws := middleware.WorkspaceID(ctx)
	existing, err := h.q.ListAutomations(ctx, ws)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create automation")
		return
	}
	if len(existing) >= maxAutomationsPerWorkspace {
		writeError(w, http.StatusBadRequest, "automation limit reached ("+strconv.Itoa(maxAutomationsPerWorkspace)+")")
		return
	}
	cfg, _ := json.Marshal(in.Config)
	a, err := h.q.CreateAutomation(ctx, database.CreateAutomationParams{
		WorkspaceID: ws, Name: in.Name, Kind: in.Kind, Frequency: in.Schedule.Frequency, RunTime: in.Schedule.Time,
		Weekday: in.Schedule.Weekday, Timezone: in.Schedule.Timezone, Enabled: in.Enabled, Config: cfg,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create automation")
		return
	}
	writeJSON(w, http.StatusCreated, automationView(a))
}

func (h *AutomationHandler) Update(w http.ResponseWriter, r *http.Request) {
	cur, ok := h.load(w, r)
	if !ok {
		return
	}
	in, ok := decodeAutomationInput(w, r)
	if !ok {
		return
	}
	cfg, _ := json.Marshal(in.Config)
	a, err := h.q.UpdateAutomation(r.Context(), database.UpdateAutomationParams{
		ID: cur.ID, WorkspaceID: cur.WorkspaceID, Name: in.Name, Kind: in.Kind, Frequency: in.Schedule.Frequency,
		RunTime: in.Schedule.Time, Weekday: in.Schedule.Weekday, Timezone: in.Schedule.Timezone, Enabled: in.Enabled, Config: cfg,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update automation")
		return
	}
	writeJSON(w, http.StatusOK, automationView(a))
}

func (h *AutomationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	a, ok := h.load(w, r)
	if !ok {
		return
	}
	if _, err := h.q.DeleteAutomation(r.Context(), database.DeleteAutomationParams{ID: a.ID, WorkspaceID: a.WorkspaceID}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete automation")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Run executes the automation now and returns the finished run. A run still
// in progress (younger than the run timeout) answers 409.
func (h *AutomationHandler) Run(w http.ResponseWriter, r *http.Request) {
	a, ok := h.load(w, r)
	if !ok {
		return
	}
	last, err := h.q.ListAutomationRuns(r.Context(), database.ListAutomationRunsParams{AutomationID: a.ID, WorkspaceID: a.WorkspaceID, Lim: 1})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start run")
		return
	}
	if len(last) == 1 && last[0].Status == "running" && time.Since(last[0].StartedAt) < automation.RunTimeout {
		writeError(w, http.StatusConflict, "this automation is already running")
		return
	}
	// Detached: a client that disconnects must not leave the run half-done.
	run, err := h.runner.RunNow(context.WithoutCancel(r.Context()), a)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to run automation")
		return
	}
	writeJSON(w, http.StatusOK, automationRunView(run))
}

func (h *AutomationHandler) Runs(w http.ResponseWriter, r *http.Request) {
	a, ok := h.load(w, r)
	if !ok {
		return
	}
	lim := int32(20)
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 100 {
		lim = int32(v)
	}
	rows, err := h.q.ListAutomationRuns(r.Context(), database.ListAutomationRunsParams{AutomationID: a.ID, WorkspaceID: a.WorkspaceID, Lim: lim})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	out := make([]AutomationRunView, len(rows))
	for i, run := range rows {
		out[i] = automationRunView(run)
	}
	writeJSON(w, http.StatusOK, out)
}

// ListSkills serves the embedded prompt library (docs/skills).
func ListSkills(w http.ResponseWriter, _ *http.Request) {
	list, err := skills.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load skills")
		return
	}
	writeJSON(w, http.StatusOK, list)
}
