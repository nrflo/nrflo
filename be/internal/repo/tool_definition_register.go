package repo

import (
	"database/sql"
	"strings"
	"time"

	"be/internal/model"
)

// UpsertByName idempotently inserts or updates a global tool definition keyed on name.
// If a row with the same name exists, its id and created_at are preserved.
// project_id and workflow_id are always NULL (global registration).
func (r *ToolDefinitionRepo) UpsertByName(def *model.ToolDefinition) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)

	if def.AuthMethod == "" {
		def.AuthMethod = "none"
	}
	if def.TimeoutSec == 0 {
		def.TimeoutSec = 30
	}

	var existingID, existingCreatedAt string
	err := r.db.QueryRow(
		"SELECT id, created_at FROM tool_definitions WHERE LOWER(name) = LOWER(?)", def.Name,
	).Scan(&existingID, &existingCreatedAt)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if existingID != "" {
		_, err = r.db.Exec(`
			UPDATE tool_definitions SET
				name=?, description=?, input_schema=?, endpoint=?,
				auth_method=?, auth_ref=?, timeout_sec=?,
				project_id=NULL, workflow_id=NULL, updated_at=?
			WHERE id=?`,
			def.Name, def.Description, string(def.InputSchema), def.Endpoint,
			def.AuthMethod, def.AuthRef, def.TimeoutSec, now, existingID)
		return err
	}

	id := strings.ToLower(def.ID)
	if id == "" {
		id = slugify(def.Name)
	}
	_, err = r.db.Exec(`
		INSERT INTO tool_definitions
			(id, name, description, input_schema, endpoint, auth_method, auth_ref, timeout_sec, project_id, workflow_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)`,
		id, def.Name, def.Description, string(def.InputSchema), def.Endpoint,
		def.AuthMethod, def.AuthRef, def.TimeoutSec, now, now)
	return err
}

// ListGlobalRegistered returns tool definitions with no project or workflow scope.
func (r *ToolDefinitionRepo) ListGlobalRegistered() ([]*model.ToolDefinition, error) {
	return r.listRows(`
		SELECT id, name, description, input_schema, endpoint, auth_method, auth_ref, timeout_sec, project_id, workflow_id, created_at, updated_at
		FROM tool_definitions WHERE project_id IS NULL AND workflow_id IS NULL ORDER BY name ASC`)
}

// DeleteByName removes a tool definition by name. No-op when not found.
func (r *ToolDefinitionRepo) DeleteByName(name string) error {
	_, err := r.db.Exec("DELETE FROM tool_definitions WHERE LOWER(name) = LOWER(?)", name)
	return err
}

// slugify returns a lowercase, hyphen-separated identifier from s.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prev := rune('-')
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			b.WriteRune(ch)
			prev = ch
		} else if prev != '-' {
			b.WriteRune('-')
			prev = '-'
		}
	}
	return strings.Trim(b.String(), "-")
}
