DROP INDEX IF EXISTS idx_review_items_lookup;

ALTER TABLE review_items RENAME TO nrvapp_review_items;

CREATE INDEX IF NOT EXISTS idx_nrvapp_review_items_lookup
    ON nrvapp_review_items (project_id, status, created_at DESC);
