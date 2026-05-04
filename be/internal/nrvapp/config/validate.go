package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

// validateTool checks required fields and rejects removed runtime types.
func validateTool(rt *rawTool) error {
	if rt.Name == "" {
		return fmt.Errorf("name is required")
	}
	if rt.Description == "" {
		return fmt.Errorf("description is required")
	}
	if rt.Type == "" {
		return fmt.Errorf("type is required")
	}
	switch rt.Type {
	case "builtin":
		return fmt.Errorf("type %q is no longer supported; the builtin and config_template runtimes have been removed", rt.Type)
	case "config_template":
		return fmt.Errorf("type %q is no longer supported; the builtin and config_template runtimes have been removed", rt.Type)
	case string(TypePythonScript):
		if rt.Script == "" {
			return fmt.Errorf("script is required for type %q", rt.Type)
		}
	default:
		return fmt.Errorf("unknown type %q", rt.Type)
	}
	if rt.InputSchema == nil {
		return fmt.Errorf("input_schema is required")
	}
	return nil
}

// compileSchema marshals the raw schema map to JSON and compiles it with Draft2020.
// Returns the canonical JSON bytes of the schema, or an error if invalid.
func compileSchema(raw map[string]interface{}) ([]byte, error) {
	if raw == nil {
		return nil, nil
	}
	schemaJSON, err := json.Marshal(normalizeMap(raw))
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	if err := compiler.AddResource("schema://compile", bytes.NewReader(schemaJSON)); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	if _, err := compiler.Compile("schema://compile"); err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}
	return schemaJSON, nil
}

// ValidateInput validates input against the named tool's compiled input_schema.
// Returns nil if the tool has no schema or input passes validation.
func (m *Manifest) ValidateInput(toolName string, input interface{}) error {
	t, ok := m.Tool(toolName)
	if !ok {
		return fmt.Errorf("tool not found: %s", toolName)
	}
	if len(t.InputSchema) == 0 {
		return nil
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	if err := compiler.AddResource("schema://input", bytes.NewReader(t.InputSchema)); err != nil {
		return err
	}
	sch, err := compiler.Compile("schema://input")
	if err != nil {
		return err
	}
	if err := sch.Validate(input); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			return ve
		}
		return err
	}
	return nil
}

// normalizeMap converts map[interface{}]interface{} (yaml.v2 style) to map[string]interface{}.
// yaml.v3 already produces map[string]interface{}, but we normalize defensively.
func normalizeMap(v interface{}) interface{} {
	switch m := v.(type) {
	case map[interface{}]interface{}:
		result := make(map[string]interface{}, len(m))
		for k, val := range m {
			result[fmt.Sprintf("%v", k)] = normalizeMap(val)
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{}, len(m))
		for k, val := range m {
			result[k] = normalizeMap(val)
		}
		return result
	case []interface{}:
		for i, item := range m {
			m[i] = normalizeMap(item)
		}
		return m
	default:
		return v
	}
}
