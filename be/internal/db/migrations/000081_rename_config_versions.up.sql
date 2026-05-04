ALTER TABLE nrvapp_config_versions RENAME TO customer_config_versions;

DROP INDEX IF EXISTS idx_nrvapp_config_versions_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_config_versions_unique
    ON customer_config_versions (project_id, file, version);
