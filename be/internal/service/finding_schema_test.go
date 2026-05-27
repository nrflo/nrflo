package service

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/types"
)

func fs(key, schema, example string) types.FindingSchema {
	return types.FindingSchema{Key: key, Schema: json.RawMessage(schema), Example: json.RawMessage(example)}
}

func TestValidateFindingSchemas(t *testing.T) {
	t.Parallel()
	arraySchema := `{"type":"array","items":{"type":"object","properties":{"file":{"type":"string"},"severity":{"type":"string"}},"required":["file","severity"]}}`

	cases := []struct {
		name    string
		defs    []types.FindingSchema
		wantErr string // substring; "" = expect success
	}{
		{"valid", []types.FindingSchema{fs("security_issues", arraySchema, `[{"file":"a.go","severity":"high"}]`)}, ""},
		{"empty slice", nil, ""},
		{"empty key", []types.FindingSchema{fs("", arraySchema, `[]`)}, "key cannot be empty"},
		{"duplicate key", []types.FindingSchema{fs("k", arraySchema, `[]`), fs("k", arraySchema, `[]`)}, "duplicate"},
		{"missing schema", []types.FindingSchema{{Key: "k", Example: json.RawMessage(`[]`)}}, "schema for key 'k' is required"},
		{"missing example", []types.FindingSchema{{Key: "k", Schema: json.RawMessage(arraySchema)}}, "example for key 'k' is required"},
		{"invalid schema json", []types.FindingSchema{fs("k", `{not json`, `[]`)}, "invalid schema"},
		{"example mismatch", []types.FindingSchema{fs("k", arraySchema, `[{"file":"a.go"}]`)}, "does not satisfy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFindingSchemas(tc.defs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateFindingSchemas_TooMany(t *testing.T) {
	t.Parallel()
	defs := make([]types.FindingSchema, maxFindingSchemas+1)
	for i := range defs {
		defs[i] = fs(string(rune('a'+i%26))+string(rune('0'+i/26)), `{"type":"array"}`, `[]`)
	}
	if err := ValidateFindingSchemas(defs); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("expected too-many error, got %v", err)
	}
}
