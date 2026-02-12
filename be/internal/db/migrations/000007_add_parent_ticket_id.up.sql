ALTER TABLE tickets ADD COLUMN parent_ticket_id TEXT;
CREATE INDEX idx_tickets_parent ON tickets(project_id, parent_ticket_id);
