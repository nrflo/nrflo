CREATE INDEX IF NOT EXISTS idx_audit_log_resource ON audit_log (resource_type, resource_id, created_at DESC);
