package service

import (
	"encoding/json"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// ProjectFindingsService handles project-level key-value findings.
// Unlike FindingsService which stores findings as a JSON blob per agent session,
// this stores one row per key in the project_findings table.
type ProjectFindingsService struct {
	clock clock.Clock
	pool  *db.Pool
}

// NewProjectFindingsService creates a new project findings service
func NewProjectFindingsService(pool *db.Pool, clk clock.Clock) *ProjectFindingsService {
	return &ProjectFindingsService{pool: pool, clock: clk}
}

// Add upserts a single key-value finding for a project.
func (s *ProjectFindingsService) Add(projectID string, req *types.ProjectFindingsAddRequest) error {
	if req.Key == "" {
		return fmt.Errorf("key is required")
	}

	value := normalizeValue(req.Value)
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.pool.Exec(`
		INSERT INTO project_findings (project_id, key, value, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (project_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		projectID, req.Key, value, now)
	return err
}

// AddBulk upserts multiple key-value findings in a transaction.
func (s *ProjectFindingsService) AddBulk(projectID string, req *types.ProjectFindingsAddBulkRequest) error {
	if len(req.KeyValues) == 0 {
		return fmt.Errorf("at least one key-value pair is required")
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.pool.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for key, val := range req.KeyValues {
		value := normalizeValue(val)
		_, err := tx.Exec(`
			INSERT INTO project_findings (project_id, key, value, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (project_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			projectID, key, value, now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Get retrieves project findings. If no keys specified, returns all as map[string]interface{}.
// If specific keys requested, returns only those.
func (s *ProjectFindingsService) Get(projectID string, req *types.ProjectFindingsGetRequest) (interface{}, error) {
	keys := req.Keys
	if req.Key != "" && len(keys) == 0 {
		keys = []string{req.Key}
	}

	if len(keys) == 0 {
		return s.getAll(projectID)
	}

	if len(keys) == 1 {
		return s.getOne(projectID, keys[0])
	}

	return s.getMultiple(projectID, keys)
}

func (s *ProjectFindingsService) getAll(projectID string) (map[string]interface{}, error) {
	rows, err := s.pool.Query(
		`SELECT key, value FROM project_findings WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]interface{})
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		result[key] = parseValue(value)
	}
	return result, nil
}

func (s *ProjectFindingsService) getOne(projectID, key string) (interface{}, error) {
	var value string
	err := s.pool.QueryRow(
		`SELECT value FROM project_findings WHERE project_id = ? AND key = ?`,
		projectID, key).Scan(&value)
	if err != nil {
		return nil, fmt.Errorf("finding '%s' not found", key)
	}
	return parseValue(value), nil
}

func (s *ProjectFindingsService) getMultiple(projectID string, keys []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for _, key := range keys {
		var value string
		err := s.pool.QueryRow(
			`SELECT value FROM project_findings WHERE project_id = ? AND key = ?`,
			projectID, key).Scan(&value)
		if err == nil {
			result[key] = parseValue(value)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("none of the requested keys found")
	}
	return result, nil
}

// Append appends to an existing value using the same array-merge logic as findings.
func (s *ProjectFindingsService) Append(projectID string, req *types.ProjectFindingsAppendRequest) error {
	if req.Key == "" {
		return fmt.Errorf("key is required")
	}

	var newValue interface{}
	if err := json.Unmarshal([]byte(req.Value), &newValue); err != nil {
		newValue = req.Value
	}

	// Read existing value
	var existingStr string
	err := s.pool.QueryRow(
		`SELECT value FROM project_findings WHERE project_id = ? AND key = ?`,
		projectID, req.Key).Scan(&existingStr)

	var merged interface{}
	if err != nil {
		// Key doesn't exist yet — just use newValue
		merged = newValue
	} else {
		existing := parseValue(existingStr)
		merged = AppendValue(existing, newValue)
	}

	data, _ := json.Marshal(merged)
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.pool.Exec(`
		INSERT INTO project_findings (project_id, key, value, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (project_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		projectID, req.Key, string(data), now)
	return err
}

// AppendBulk appends multiple values in a transaction.
func (s *ProjectFindingsService) AppendBulk(projectID string, req *types.ProjectFindingsAppendBulkRequest) error {
	if len(req.KeyValues) == 0 {
		return fmt.Errorf("at least one key-value pair is required")
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.pool.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for key, val := range req.KeyValues {
		var newValue interface{}
		if err := json.Unmarshal([]byte(val), &newValue); err != nil {
			newValue = val
		}

		var existingStr string
		err := tx.QueryRow(
			`SELECT value FROM project_findings WHERE project_id = ? AND key = ?`,
			projectID, key).Scan(&existingStr)

		var merged interface{}
		if err != nil {
			merged = newValue
		} else {
			existing := parseValue(existingStr)
			merged = AppendValue(existing, newValue)
		}

		data, _ := json.Marshal(merged)
		_, err = tx.Exec(`
			INSERT INTO project_findings (project_id, key, value, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (project_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			projectID, key, string(data), now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Delete removes specified keys and returns the list of actually deleted keys.
func (s *ProjectFindingsService) Delete(projectID string, req *types.ProjectFindingsDeleteRequest) ([]string, error) {
	if len(req.Keys) == 0 {
		return nil, fmt.Errorf("at least one key is required")
	}

	// Check which keys exist before deleting
	var deleted []string
	for _, key := range req.Keys {
		var k string
		err := s.pool.QueryRow(
			`SELECT key FROM project_findings WHERE project_id = ? AND key = ?`,
			projectID, key).Scan(&k)
		if err == nil {
			deleted = append(deleted, key)
		}
	}

	if len(deleted) == 0 {
		return nil, nil
	}

	for _, key := range deleted {
		_, err := s.pool.Exec(
			`DELETE FROM project_findings WHERE project_id = ? AND key = ?`,
			projectID, key)
		if err != nil {
			return nil, err
		}
	}

	return deleted, nil
}

// normalizeValue attempts to parse the value as JSON for consistent storage.
// If parsing fails, stores the raw string as a JSON string.
func normalizeValue(val string) string {
	var parsed interface{}
	if err := json.Unmarshal([]byte(val), &parsed); err != nil {
		// Not valid JSON — store as JSON string
		data, _ := json.Marshal(val)
		return string(data)
	}
	// Re-serialize for consistent formatting
	data, _ := json.Marshal(parsed)
	return string(data)
}

// parseValue parses a JSON-stored value back to a Go interface.
func parseValue(val string) interface{} {
	var parsed interface{}
	if err := json.Unmarshal([]byte(val), &parsed); err != nil {
		return val
	}
	return parsed
}
