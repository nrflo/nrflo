package configeditor_test

import (
	"strings"
	"testing"

	"be/internal/configeditor"
)

func TestValidateYAML_EmptySchema(t *testing.T) {
	if err := configeditor.ValidateYAML([]byte("key: value"), nil); err != nil {
		t.Errorf("ValidateYAML nil schema: %v", err)
	}
	if err := configeditor.ValidateYAML([]byte("key: value"), []byte{}); err != nil {
		t.Errorf("ValidateYAML empty schema: %v", err)
	}
}

func TestValidateYAML_Valid(t *testing.T) {
	schema := []byte(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	content := []byte("name: Alice\n")
	if err := configeditor.ValidateYAML(content, schema); err != nil {
		t.Errorf("ValidateYAML valid: %v", err)
	}
}

func TestValidateYAML_MissingRequired(t *testing.T) {
	schema := []byte(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	content := []byte("age: 30\n")
	err := configeditor.ValidateYAML(content, schema)
	if err == nil {
		t.Fatal("ValidateYAML missing required: expected error, got nil")
	}
	if len(err.Fields) == 0 {
		t.Error("ValidationError.Fields is empty, want at least one field")
	}
}

func TestValidateYAML_WrongType(t *testing.T) {
	schema := []byte(`{"type":"object","properties":{"count":{"type":"integer"}},"required":["count"]}`)
	content := []byte("count: not-a-number\n")
	err := configeditor.ValidateYAML(content, schema)
	if err == nil {
		t.Fatal("ValidateYAML wrong type: expected error, got nil")
	}
}

func TestValidateYAML_InvalidYAML(t *testing.T) {
	schema := []byte(`{"type":"object"}`)
	content := []byte("{invalid yaml:::")
	err := configeditor.ValidateYAML(content, schema)
	if err == nil {
		t.Fatal("ValidateYAML invalid YAML: expected error, got nil")
	}
	if len(err.Fields) == 0 {
		t.Error("ValidationError.Fields is empty for invalid YAML")
	}
	if !strings.Contains(err.Fields[0].Message, "YAML") {
		t.Errorf("error message = %q, want to mention 'YAML'", err.Fields[0].Message)
	}
}

func TestValidationError_Error_WithFields(t *testing.T) {
	ve := &configeditor.ValidationError{
		Fields: []configeditor.FieldError{
			{Path: "/name", Message: "required property missing"},
		},
	}
	msg := ve.Error()
	if !strings.Contains(msg, "required property missing") {
		t.Errorf("Error() = %q, want to contain 'required property missing'", msg)
	}
	if !strings.Contains(msg, "/name") {
		t.Errorf("Error() = %q, want to contain '/name'", msg)
	}
}

func TestValidationError_Error_Empty(t *testing.T) {
	ve := &configeditor.ValidationError{}
	if ve.Error() != "validation failed" {
		t.Errorf("Error() = %q, want 'validation failed'", ve.Error())
	}
}

func TestValidateYAML_AdditionalProperties(t *testing.T) {
	schema := []byte(`{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string"}}}`)
	content := []byte("name: ok\nextra: rejected\n")
	err := configeditor.ValidateYAML(content, schema)
	if err == nil {
		t.Fatal("ValidateYAML extra property: expected error, got nil")
	}
}

func TestValidateYAML_Array(t *testing.T) {
	schema := []byte(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"string"}}},"required":["items"]}`)
	valid := []byte("items:\n  - a\n  - b\n")
	if err := configeditor.ValidateYAML(valid, schema); err != nil {
		t.Errorf("ValidateYAML valid array: %v", err)
	}

	invalid := []byte("items:\n  - 1\n  - 2\n") // integers, not strings
	if err := configeditor.ValidateYAML(invalid, schema); err == nil {
		t.Error("ValidateYAML wrong array item type: expected error, got nil")
	}
}
