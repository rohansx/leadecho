package handler

// workflows: real-time rules ("when a lead like this comes in, do these
// actions"), their run history, and a side-effect-free preview.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
	"leadecho/internal/netsafe"
	"leadecho/internal/workflow"
)

const maxWorkflowsPerWorkspace = 25

// WorkflowStore is the slice of database.Queries the handler uses.
type WorkflowStore interface {
	workflow.RecentLeads
	ListWorkflows(ctx context.Context, workspaceID string) ([]database.Workflow, error)
	GetWorkflow(ctx context.Context, arg database.GetWorkflowParams) (database.Workflow, error)
	CountWorkflows(ctx context.Context, workspaceID string) (int64, error)
	CreateWorkflow(ctx context.Context, arg database.CreateWorkflowParams) (database.Workflow, error)
	UpdateWorkflow(ctx context.Context, arg database.UpdateWorkflowParams) (database.Workflow, error)
	DeleteWorkflow(ctx context.Context, arg database.DeleteWorkflowParams) (int64, error)
	ListWorkflowExecutions(ctx context.Context, arg database.ListWorkflowExecutionsParams) ([]database.ListWorkflowExecutionsRow, error)
}

type WorkflowHandler struct {
	q WorkflowStore
	// validateURL checks webhook targets (netsafe.ValidatePublicURL; swapped in tests).
	validateURL func(ctx context.Context, raw string) error
}

func NewWorkflowHandler(q WorkflowStore) *WorkflowHandler {
	return &WorkflowHandler{q: q, validateURL: netsafe.ValidatePublicURL}
}

type WorkflowView struct {
	ID         string                  `json:"id"`
	Name       string                  `json:"name"`
	Enabled    bool                    `json:"enabled"`
	Trigger    workflow.Trigger        `json:"trigger"`
	Actions    []workflow.ActionConfig `json:"actions"`
	MaxPerHour int                     `json:"max_per_hour"`
	RunCount   int                     `json:"run_count"`
	LastRunAt  *time.Time              `json:"last_run_at"`
	CreatedAt  time.Time               `json:"created_at"`
}

func workflowView(w database.Workflow) WorkflowView {
	v := WorkflowView{
		ID: w.ID, Name: w.Name, Enabled: w.Status == database.WorkflowStatusActive,
		MaxPerHour: int(w.MaxTriggersPerHour), RunCount: int(w.ExecutionCount), CreatedAt: w.CreatedAt,
		Actions: []workflow.ActionConfig{},
	}
	_ = json.Unmarshal(w.TriggerConfig, &v.Trigger) // validated on write
	_ = json.Unmarshal(w.ActionChain, &v.Actions)
	if w.LastTriggeredAt.Valid {
		t := w.LastTriggeredAt.Time
		v.LastRunAt = &t
	}
	return v
}

func (h *WorkflowHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListWorkflows(r.Context(), middleware.WorkspaceID(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workflows")
		return
	}
	out := make([]WorkflowView, 0, len(rows))
	for _, row := range rows {
		out = append(out, workflowView(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// decodeDefinition reads, normalises and checks a workflow body.
func (h *WorkflowHandler) decodeDefinition(w http.ResponseWriter, r *http.Request) (workflow.Definition, bool) {
	var d workflow.Definition
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&d); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return d, false
	}
	d, err := d.Normalize()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return d, false
	}
	for _, a := range d.Actions {
		if a.Type != workflow.ActionWebhook {
			continue
		}
		if u, _ := a.Config["url"].(string); h.validateURL(r.Context(), u) != nil {
			writeError(w, http.StatusBadRequest, "webhook url must be a public https address")
			return d, false
		}
	}
	return d, true
}

func definitionColumns(d workflow.Definition) (status database.WorkflowStatus, trigger, actions []byte) {
	status = database.WorkflowStatusPaused
	if d.Enabled {
		status = database.WorkflowStatusActive
	}
	trigger, _ = json.Marshal(d.Trigger)
	actions, _ = json.Marshal(d.Actions)
	return status, trigger, actions
}

func (h *WorkflowHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ws := middleware.WorkspaceID(ctx)
	d, ok := h.decodeDefinition(w, r)
	if !ok {
		return
	}
	if n, err := h.q.CountWorkflows(ctx, ws); err != nil || n >= maxWorkflowsPerWorkspace {
		writeError(w, http.StatusBadRequest, "workflow limit reached (25)")
		return
	}
	status, trigger, actions := definitionColumns(d)
	var createdBy pgtype.UUID
	_ = createdBy.Scan(middleware.UserID(ctx))
	row, err := h.q.CreateWorkflow(ctx, database.CreateWorkflowParams{
		WorkspaceID: ws, Name: d.Name, Status: status, TriggerConfig: trigger, ActionChain: actions,
		MaxTriggersPerHour: int32(d.MaxPerHour), CreatedBy: createdBy,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workflow")
		return
	}
	writeJSON(w, http.StatusCreated, workflowView(row))
}

func (h *WorkflowHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid workflow id")
		return
	}
	d, ok := h.decodeDefinition(w, r)
	if !ok {
		return
	}
	status, trigger, actions := definitionColumns(d)
	row, err := h.q.UpdateWorkflow(ctx, database.UpdateWorkflowParams{
		ID: id, WorkspaceID: middleware.WorkspaceID(ctx), Name: d.Name, Status: status,
		TriggerConfig: trigger, ActionChain: actions, MaxTriggersPerHour: int32(d.MaxPerHour),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update workflow")
		return
	}
	writeJSON(w, http.StatusOK, workflowView(row))
}

func (h *WorkflowHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid workflow id")
		return
	}
	n, err := h.q.DeleteWorkflow(r.Context(), database.DeleteWorkflowParams{ID: id, WorkspaceID: middleware.WorkspaceID(r.Context())})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type WorkflowRunView struct {
	ID        string                `json:"id"`
	Status    string                `json:"status"`
	Steps     []workflow.StepResult `json:"steps"`
	Error     string                `json:"error,omitempty"`
	CreatedAt time.Time             `json:"created_at"`
	MentionID string                `json:"mention_id,omitempty"`
	Platform  string                `json:"platform,omitempty"`
	Title     string                `json:"title,omitempty"`
	URL       string                `json:"url,omitempty"`
	Score     *float32              `json:"score,omitempty"`
}

func (h *WorkflowHandler) Runs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid workflow id")
		return
	}
	rows, err := h.q.ListWorkflowExecutions(r.Context(), database.ListWorkflowExecutionsParams{WorkflowID: id, WorkspaceID: middleware.WorkspaceID(r.Context())})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load runs")
		return
	}
	out := make([]WorkflowRunView, 0, len(rows))
	for _, row := range rows {
		v := WorkflowRunView{ID: row.ID, Status: string(row.Status), Error: row.ErrorMessage.String, CreatedAt: row.CreatedAt, Steps: []workflow.StepResult{}}
		_ = json.Unmarshal(row.Steps, &v.Steps)
		if row.MentionID.Valid {
			v.MentionID = row.MentionID.String()
			v.Platform, v.Title, v.URL = string(row.Platform.PlatformType), row.Title.String, row.Url.String
			if row.RelevanceScore.Valid {
				s := row.RelevanceScore.Float32
				v.Score = &s
			}
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// Preview: how many of the latest scored posts this trigger would have caught.
func (h *WorkflowHandler) Preview(w http.ResponseWriter, r *http.Request) {
	var t workflow.Trigger
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	res, err := workflow.Preview(r.Context(), h.q, middleware.WorkspaceID(r.Context()), t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to preview")
		return
	}
	writeJSON(w, http.StatusOK, res)
}
