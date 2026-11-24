-- +goose Up
-- The Solo plan (billing/plans.go). Without it in the check, a Solo
-- purchase or ops comp fails to save the plan.
ALTER TABLE workspaces DROP CONSTRAINT workspaces_plan_check;
ALTER TABLE workspaces ADD CONSTRAINT workspaces_plan_check CHECK (plan IN ('free', 'solo', 'starter', 'pro'));

-- +goose Down
UPDATE workspaces SET plan = 'free', paid_data = false WHERE plan = 'solo';
ALTER TABLE workspaces DROP CONSTRAINT workspaces_plan_check;
ALTER TABLE workspaces ADD CONSTRAINT workspaces_plan_check CHECK (plan IN ('free', 'starter', 'pro'));
