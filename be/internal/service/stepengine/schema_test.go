package stepengine

import (
	"encoding/json"
	"testing"

	"be/internal/model"
)

func TestValidateSchemaValue_JSONArrayPathChange(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		raw       string
		wantErr   bool
		wantPaths []string
	}{
		{"empty array ok", `[]`, false, nil},
		{"path+change shape", `[{"path":"a.go","change":"added func"}]`, false, []string{"a.go"}},
		{"path+purpose shape", `[{"path":"b.go","purpose":"new helper"}]`, false, []string{"b.go"}},
		{"multiple elements", `[{"path":"a.go","change":"x"},{"path":"b.go","purpose":"y"}]`, false, []string{"a.go", "b.go"}},
		{"non-array rejected", `{"path":"a.go","change":"x"}`, true, nil},
		{"non-array string rejected", `"just a string"`, true, nil},
		{"element without path", `[{"change":"added func"}]`, true, nil},
		{"element with empty path", `[{"path":"","change":"added func"}]`, true, nil},
		{"element with whitespace-only path", `[{"path":"  ","change":"added func"}]`, true, nil},
		{"element with no descriptive sibling", `[{"path":"a.go"}]`, true, nil},
		{"element with empty descriptive sibling only", `[{"path":"a.go","change":""}]`, true, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			paths, err := validateSchemaValue(model.FindingSchemaJSONArrayPathChange, json.RawMessage(tc.raw))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if len(paths) != len(tc.wantPaths) {
				t.Fatalf("paths = %v, want %v", paths, tc.wantPaths)
			}
			for i, p := range tc.wantPaths {
				if paths[i] != p {
					t.Errorf("paths[%d] = %q, want %q", i, paths[i], p)
				}
			}
		})
	}
}

func TestValidateSchemaValue_NonemptyText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"none accepted", `"none"`, false},
		{"normal text accepted", `"did the thing"`, false},
		{"empty string rejected", `""`, true},
		{"whitespace only rejected", `"   "`, true},
		{"non-string rejected", `123`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateSchemaValue(model.FindingSchemaNonemptyText, json.RawMessage(tc.raw))
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateSchemaValue_OrderedLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"valid two lines", `"1. a\n2. b"`, false},
		{"valid with parens", `"1) a\n2) b"`, false},
		{"single line rejected", `"1. a"`, true},
		{"unnumbered lines rejected", `"a\nb"`, true},
		{"descending numbers rejected", `"2. a\n1. b"`, true},
		{"repeated numbers rejected", `"1. a\n1. b"`, true},
		{"non-string rejected", `5`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateOrderedLines(json.RawMessage(tc.raw))
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestUnwrapOnce_SingleLevelJSONStringUnwrap(t *testing.T) {
	t.Parallel()
	// A json_array_path_change value stored as a double-encoded JSON string
	// must unwrap once and validate as the inner array.
	doubleEncoded := `"[{\"path\":\"a.go\",\"change\":\"x\"}]"`
	paths, err := validateSchemaValue(model.FindingSchemaJSONArrayPathChange, json.RawMessage(doubleEncoded))
	if err != nil {
		t.Fatalf("validateSchemaValue(double-encoded array): %v", err)
	}
	if len(paths) != 1 || paths[0] != "a.go" {
		t.Errorf("paths = %v, want [a.go]", paths)
	}
}

func TestUnwrapOnce_PlainStringNeverUnwraps(t *testing.T) {
	t.Parallel()
	// A plain string value (not itself JSON) must pass through untouched.
	if err := validateNonemptyText(unwrapOnce(json.RawMessage(`"plain text"`))); err != nil {
		t.Errorf("unwrapOnce(plain string) broke nonempty_text validation: %v", err)
	}
}

func TestValidateSchemaValue_UnknownSchemaReturnsError(t *testing.T) {
	t.Parallel()
	if _, err := validateSchemaValue("not_a_real_schema", json.RawMessage(`"x"`)); err == nil {
		t.Error("validateSchemaValue(unknown schema) = nil error, want error")
	}
}

// TestValidateSchemaValue_PathCandidatesOnlyFromJSONArrayPathChange verifies
// nonempty_text/ordered_lines never produce path candidates even when their
// text content looks path-like.
func TestValidateSchemaValue_PathCandidatesOnlyFromJSONArrayPathChange(t *testing.T) {
	t.Parallel()
	paths, err := validateSchemaValue(model.FindingSchemaNonemptyText, json.RawMessage(`"src/pkg/foo.go"`))
	if err != nil {
		t.Fatalf("validateSchemaValue: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("paths = %v, want none from nonempty_text", paths)
	}
}
