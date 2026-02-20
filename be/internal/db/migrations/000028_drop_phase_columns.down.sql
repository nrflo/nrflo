ALTER TABLE workflow_instances ADD COLUMN phases TEXT NOT NULL DEFAULT '{}';
ALTER TABLE workflow_instances ADD COLUMN phase_order TEXT NOT NULL DEFAULT '[]';
ALTER TABLE workflow_instances ADD COLUMN current_phase TEXT;
