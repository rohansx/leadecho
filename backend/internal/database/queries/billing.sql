-- name: TakeCredits :one
-- Atomically takes up to @want credits; returns how many were taken (0..want).
WITH cur AS (
    SELECT credit_balance FROM workspaces WHERE id = @workspace_id FOR UPDATE
)
UPDATE workspaces w
SET credit_balance = w.credit_balance - LEAST(cur.credit_balance, @want::int)
FROM cur
WHERE w.id = @workspace_id
RETURNING LEAST(cur.credit_balance, @want::int)::int AS taken;

-- name: SpendCredits :one
-- All-or-nothing: returns the new balance, or no row when the balance is short.
UPDATE workspaces SET credit_balance = credit_balance - @cost::int
WHERE id = @workspace_id AND credit_balance >= @cost::int
RETURNING credit_balance;

-- name: AddCreditUsage :exec
INSERT INTO credit_usage_daily (workspace_id, day, reason, credits)
VALUES (@workspace_id, CURRENT_DATE, @reason, @credits)
ON CONFLICT (workspace_id, day, reason) DO UPDATE SET credits = credit_usage_daily.credits + EXCLUDED.credits;

-- name: GrantCredits :one
-- Idempotent on (workspace_id, ref): returns no row when the grant already happened.
WITH ins AS (
    INSERT INTO credit_ledger (workspace_id, delta, balance_after, reason, ref)
    SELECT @workspace_id, @delta::int, w.credit_balance + @delta::int, @reason, @ref
    FROM workspaces w WHERE w.id = @workspace_id
    ON CONFLICT (workspace_id, ref) DO NOTHING
    RETURNING workspace_id, delta
)
UPDATE workspaces w SET credit_balance = w.credit_balance + ins.delta
FROM ins WHERE w.id = ins.workspace_id
RETURNING w.credit_balance;

-- name: GetBillingState :one
SELECT id, plan, credit_balance, keyword_limit, plan_renews_at, credits_refreshed_at,
       dodo_customer_id, dodo_subscription_id
FROM workspaces WHERE id = @workspace_id;

-- name: SetWorkspacePlan :exec
UPDATE workspaces
SET plan = @plan, keyword_limit = @keyword_limit,
    dodo_customer_id = COALESCE(sqlc.narg(dodo_customer_id), dodo_customer_id),
    dodo_subscription_id = sqlc.narg(dodo_subscription_id),
    plan_renews_at = sqlc.narg(plan_renews_at),
    updated_at = now()
WHERE id = @workspace_id;

-- name: FindWorkspaceByDodoCustomer :one
SELECT id FROM workspaces WHERE dodo_customer_id = @dodo_customer_id ORDER BY created_at LIMIT 1;

-- name: RefreshFreeCredits :execrows
-- Free plan: top the balance back up to the monthly allowance every 30 days.
UPDATE workspaces
SET credit_balance = GREATEST(credit_balance, @allowance::int), credits_refreshed_at = now()
WHERE plan = 'free' AND credits_refreshed_at < now() - interval '30 days';

-- name: CreditUsageSince :many
SELECT reason, SUM(credits)::int AS credits
FROM credit_usage_daily
WHERE workspace_id = @workspace_id AND day >= @since::date
GROUP BY reason ORDER BY 2 DESC;

-- name: ListCreditGrants :many
SELECT delta, balance_after, reason, ref, created_at
FROM credit_ledger WHERE workspace_id = @workspace_id
ORDER BY created_at DESC LIMIT @lim;

-- name: CountWorkspaceKeywords :one
SELECT count(*)::int FROM keywords WHERE workspace_id = @workspace_id AND is_active;
