-- Per-instance opt-in for the self-drafting plan boundary to auto-approve
-- (mode=auto) instead of suspending at waiting_approval, gated additionally by
-- the dynamic_workflow_auto_enabled config KV (service/plan_auto.go).
ALTER TABLE workflow_instances ADD COLUMN plan_auto_approve INTEGER NOT NULL DEFAULT 0;
