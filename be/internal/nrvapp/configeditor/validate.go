package configeditor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

// FieldError represents a single schema validation failure.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError holds one or more field-level validation failures.
type ValidationError struct {
	Fields []FieldError `json:"fields"`
}

func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "validation failed"
	}
	return fmt.Sprintf("validation failed: %s at %s", e.Fields[0].Message, e.Fields[0].Path)
}

// ValidateYAML validates YAML content against a JSON Schema (Draft2020).
// Returns nil when schemaBytes is empty or validation passes.
func ValidateYAML(content []byte, schemaBytes []byte) *ValidationError {
	if len(schemaBytes) == 0 {
		return nil
	}

	v, err := normalizeYAML(content)
	if err != nil {
		return &ValidationError{Fields: []FieldError{{Path: "", Message: "invalid YAML: " + err.Error()}}}
	}

	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	if err := compiler.AddResource("schema://validate", bytes.NewReader(schemaBytes)); err != nil {
		return &ValidationError{Fields: []FieldError{{Path: "", Message: "schema load error: " + err.Error()}}}
	}
	sch, err := compiler.Compile("schema://validate")
	if err != nil {
		return &ValidationError{Fields: []FieldError{{Path: "", Message: "schema compile error: " + err.Error()}}}
	}

	if err := sch.Validate(v); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			return &ValidationError{Fields: collectFieldErrors(ve)}
		}
		return &ValidationError{Fields: []FieldError{{Path: "", Message: err.Error()}}}
	}
	return nil
}

func collectFieldErrors(ve *jsonschema.ValidationError) []FieldError {
	if len(ve.Causes) == 0 {
		return []FieldError{{Path: ve.InstanceLocation, Message: ve.Message}}
	}
	var errs []FieldError
	for _, cause := range ve.Causes {
		errs = append(errs, collectFieldErrors(cause)...)
	}
	return errs
}

// normalizeYAML parses YAML and returns a JSON-compatible interface{}.
// The YAML→JSON round-trip ensures the value is safe to pass to jsonschema.Validate.
func normalizeYAML(content []byte) (interface{}, error) {
	var v interface{}
	if err := yaml.Unmarshal(content, &v); err != nil {
		return nil, err
	}
	// Round-trip through JSON to normalize types (yaml.v3 uses map[string]interface{})
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	var normalized interface{}
	if err := json.Unmarshal(b, &normalized); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	return normalized, nil
}
