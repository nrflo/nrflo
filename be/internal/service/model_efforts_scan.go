package service

import (
	"database/sql"
	"encoding/json"
	"time"

	"be/internal/model"
)

// parseSupportedEfforts decodes the supported_efforts TEXT column (a JSON
// array). A parse error or an empty array yields nil.
func parseSupportedEfforts(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || len(out) == 0 {
		return nil
	}
	return out
}

// marshalSupportedEfforts encodes a supported_efforts list as JSON for the
// TEXT column; a nil/empty slice becomes "[]".
func marshalSupportedEfforts(efforts []string) string {
	if len(efforts) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(efforts)
	return string(b)
}

// resolveCreateEfforts normalizes a create request's supported_efforts,
// defaulting to [reasoningEffort] when the list is empty but an effort is set,
// then validates reasoningEffort against the resulting list.
func resolveCreateEfforts(supported []string, reasoningEffort string) ([]string, error) {
	normalized, err := NormalizeSupportedEfforts(supported)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 && reasoningEffort != "" {
		normalized = []string{reasoningEffort}
	}
	if err := ValidateEffortAllowed(reasoningEffort, normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

// scanCLIModel scans a row into a CLIModel
func scanCLIModel(rows *sql.Rows) (*model.CLIModel, error) {
	m := &model.CLIModel{}
	var createdAt, updatedAt, supportedRaw string
	var readOnly, enabled int

	err := rows.Scan(
		&m.ID, &m.CLIType, &m.DisplayName, &m.MappedModel,
		&m.ReasoningEffort, &supportedRaw, &m.FallbackModels, &m.ContextLength, &readOnly, &enabled,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	m.SupportedEfforts = parseSupportedEfforts(supportedRaw)
	m.ReadOnly = readOnly == 1
	m.Enabled = enabled == 1
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return m, nil
}

// scanAPIModel scans a row into an APIModel
func scanAPIModel(rows *sql.Rows) (*model.APIModel, error) {
	m := &model.APIModel{}
	var createdAt, updatedAt, supportedRaw string
	var readOnly, enabled int

	err := rows.Scan(
		&m.ID, &m.Provider, &m.DisplayName, &m.MappedModel,
		&m.ReasoningEffort, &supportedRaw, &m.ContextLength, &readOnly, &enabled,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	m.SupportedEfforts = parseSupportedEfforts(supportedRaw)
	m.ReadOnly = readOnly == 1
	m.Enabled = enabled == 1
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return m, nil
}
