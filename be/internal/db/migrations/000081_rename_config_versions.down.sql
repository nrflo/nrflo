DROP INDEX IF EXISTS idx_customer_config_versions_unique;

ALTER TABLE customer_config_versions RENAME TO nrvapp_config_versions;

CREATE UNIQUE INDEX IF NOT EXISTS idx_nrvapp_config_versions_unique
    ON nrvapp_config_versions (project_id, file, version);
