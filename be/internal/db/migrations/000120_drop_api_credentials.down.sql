CREATE TABLE IF NOT EXISTS api_credentials (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    project_id TEXT,
    secret_ref TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_api_credentials_provider_project ON api_credentials (provider, project_id);
