ALTER TABLE nrvapp_review_items RENAME TO review_items;

DROP INDEX IF EXISTS idx_nrvapp_review_items_lookup;

CREATE INDEX IF NOT EXISTS idx_review_items_lookup
    ON review_items (project_id, status, created_at DESC);
