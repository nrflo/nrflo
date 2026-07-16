package service

import (
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

type rowScanner interface {
	Scan(dest ...any) error
}

func scanModel(row rowScanner) (*model.Model, error) {
	m := &model.Model{}
	var createdAt, updatedAt, cliEfforts, apiEfforts string
	var readOnly, enabled int

	err := row.Scan(&m.ID, &m.Provider, &m.DisplayName, &m.CLIModel, &m.APIModel,
		&cliEfforts, &apiEfforts, &m.CLIContext, &m.APIContext, &m.FallbackModels,
		&m.DefaultEffort, &readOnly, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	m.CLIEfforts = parseSupportedEfforts(cliEfforts)
	m.APIEfforts = parseSupportedEfforts(apiEfforts)
	m.ReadOnly = readOnly == 1
	m.Enabled = enabled == 1
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return m, nil
}
