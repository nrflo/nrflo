ALTER TABLE workflow_instances ADD COLUMN endless_loop INTEGER NOT NULL DEFAULT 0;
ALTER TABLE workflow_instances ADD COLUMN stop_endless_loop_after_iteration INTEGER NOT NULL DEFAULT 0;
