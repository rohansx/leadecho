package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreateLLMUsageEventParams stores one attempted model call. Token/cost fields
// are optional for providers where the MVP adapter cannot read usage yet.
type CreateLLMUsageEventParams struct {
	WorkspaceID      string
	Task             string
	Provider         string
	Model            string
	Status           string
	PromptTokens     int32
	CompletionTokens int32
	TotalTokens      int32
	EstimatedCostUSD string
	LatencyMs        int32
	ErrorMessage     pgtype.Text
	FallbackFrom     pgtype.Text
	KeySource        string
}

func (q *Queries) CreateLLMUsageEvent(ctx context.Context, arg CreateLLMUsageEventParams) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO llm_usage_events (
			workspace_id, task, provider, model, status,
			prompt_tokens, completion_tokens, total_tokens, estimated_cost_usd,
			latency_ms, error_message, fallback_from, key_source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::numeric, $10, $11, $12, $13)
	`, arg.WorkspaceID, arg.Task, arg.Provider, arg.Model, arg.Status,
		arg.PromptTokens, arg.CompletionTokens, arg.TotalTokens, arg.EstimatedCostUSD,
		arg.LatencyMs, arg.ErrorMessage, arg.FallbackFrom, arg.KeySource)
	if isUndefinedTable(err) {
		return nil
	}
	return err
}

type LLMUsageSummaryRow struct {
	Task             string    `json:"task"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	Calls            int32     `json:"calls"`
	Errors           int32     `json:"errors"`
	Fallbacks        int32     `json:"fallbacks"`
	TotalTokens      int32     `json:"total_tokens"`
	EstimatedCostUSD string    `json:"estimated_cost_usd"`
	LastUsedAt       time.Time `json:"last_used_at"`
}

func (q *Queries) LLMUsageSummary(ctx context.Context, workspaceID string) ([]LLMUsageSummaryRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT
			task,
			provider,
			model,
			COUNT(*)::int AS calls,
			COUNT(*) FILTER (WHERE status = 'error')::int AS errors,
			COUNT(*) FILTER (WHERE fallback_from IS NOT NULL)::int AS fallbacks,
			COALESCE(SUM(total_tokens), 0)::int AS total_tokens,
			COALESCE(SUM(estimated_cost_usd), 0)::text AS estimated_cost_usd,
			MAX(created_at) AS last_used_at
		FROM llm_usage_events
		WHERE workspace_id = $1 AND created_at >= now() - interval '30 days'
		GROUP BY task, provider, model
		ORDER BY calls DESC, task, provider
	`, workspaceID)
	if err != nil {
		if isUndefinedTable(err) {
			return []LLMUsageSummaryRow{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	items := []LLMUsageSummaryRow{}
	for rows.Next() {
		var i LLMUsageSummaryRow
		if err := rows.Scan(&i.Task, &i.Provider, &i.Model, &i.Calls, &i.Errors, &i.Fallbacks, &i.TotalTokens, &i.EstimatedCostUSD, &i.LastUsedAt); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

func isUndefinedTable(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}
