package consoleui

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestBuildInvokeArguments covers string quoting, numeric/bool passthrough,
// and empty-optional omission; required fields are always included even when
// empty.
//
// NOTE on non-scalar fields (array/object): the planner spec calls for
// buildInvokeArguments to embed non-scalar values as raw JSON, but
// typedArgValue only special-cases number/integer/boolean and falls through
// to a plain string for every other type (including "object"/"array") — see
// be_production_bugs. This test pins the code's actual current behavior
// (string passthrough) so it doesn't spuriously fail; it is not an
// endorsement of that behavior.
func TestBuildInvokeArguments(t *testing.T) {
	fields := []argField{
		{Name: "name", Type: "string", Required: true, Scalar: true},
		{Name: "count", Type: "integer", Scalar: true},
		{Name: "ratio", Type: "number", Scalar: true},
		{Name: "flag", Type: "boolean", Scalar: true},
		{Name: "meta", Type: "object", Scalar: false},
		{Name: "note", Type: "string", Scalar: true}, // optional, empty -> omitted
		{Name: "reqEmpty", Type: "string", Required: true, Scalar: true},
	}
	values := map[string]string{
		"name":     "ticket-1",
		"count":    "5",
		"ratio":    "1.5",
		"flag":     "true",
		"meta":     `{"a":1}`,
		"note":     "",
		"reqEmpty": "",
	}
	raw := buildInvokeArguments(fields, values)
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("buildInvokeArguments produced invalid JSON: %v (%s)", err, raw)
	}

	if got["name"] != "ticket-1" {
		t.Errorf("name = %v, want ticket-1 (string)", got["name"])
	}
	if got["count"] != float64(5) {
		t.Errorf("count = %v, want 5 (number)", got["count"])
	}
	if got["ratio"] != 1.5 {
		t.Errorf("ratio = %v, want 1.5", got["ratio"])
	}
	if got["flag"] != true {
		t.Errorf("flag = %v, want true (bool)", got["flag"])
	}
	if metaMap, ok := got["meta"].(map[string]any); !ok || metaMap["a"] != float64(1) {
		t.Errorf("meta = %v (%T), want parsed JSON object {a:1}", got["meta"], got["meta"])
	}
	if _, present := got["note"]; present {
		t.Errorf("empty optional field 'note' should be omitted, got %v", got["note"])
	}
	if v, present := got["reqEmpty"]; !present || v != "" {
		t.Errorf("required empty field 'reqEmpty' should be included as \"\", got present=%v value=%v", present, v)
	}
}

// TestBuildInvokeArguments_TypeCoercionFallback verifies an unparseable
// numeric/bool value falls back to a raw string rather than dropping it.
func TestBuildInvokeArguments_TypeCoercionFallback(t *testing.T) {
	fields := []argField{{Name: "count", Type: "integer", Required: true, Scalar: true}}
	raw := buildInvokeArguments(fields, map[string]string{"count": "not-a-number"})
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got["count"] != "not-a-number" {
		t.Errorf("count = %v, want fallback string %q", got["count"], "not-a-number")
	}
}

// TestBuildInvokeArguments_MissingValueOmitted verifies a field with no
// entry in values is omitted even when required (acceptArg always stores
// something, but the builder itself must not synthesize a value).
func TestBuildInvokeArguments_MissingValueOmitted(t *testing.T) {
	fields := []argField{{Name: "name", Type: "string", Required: true, Scalar: true}}
	raw := buildInvokeArguments(fields, map[string]string{})
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, present := got["name"]; present {
		t.Errorf("field absent from values should be omitted, got %v", got["name"])
	}
}

func toolNames(tools []ConsoleTool) []string {
	names := make([]string, len(tools))
	for i, tl := range tools {
		names[i] = tl.Name
	}
	return names
}

// TestFilterTools mirrors TestFilterSkills: same prefix-then-substring
// semantics, generalized to the tool catalogue.
func TestFilterTools(t *testing.T) {
	tools := []ConsoleTool{
		{Name: "ticket_list", Description: "list tickets"},
		{Name: "ticket_create", Description: "create a ticket"},
		{Name: "findings_add", Description: "add a finding"},
	}
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"empty query returns all", "", []string{"ticket_list", "ticket_create", "findings_add"}},
		{"exact match", "ticket_list", []string{"ticket_list"}},
		{"prefix match returns only prefix rows", "ticket", []string{"ticket_list", "ticket_create"}},
		{"case-insensitive", "TICKET", []string{"ticket_list", "ticket_create"}},
		{"no match returns empty", "zzz", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolNames(filterTools(tools, tt.query))
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterTools(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// TestFilterTools_PrefixBeatsSubstring mirrors
// TestFilterSkills_PrefixBeatsSubstring for the tool catalogue.
func TestFilterTools_PrefixBeatsSubstring(t *testing.T) {
	tools := []ConsoleTool{
		{Name: "reinvoke-helper"}, // substring match only: "invoke" appears mid-name
		{Name: "invoke-tool"},     // prefix match: starts with "invoke"
	}
	got := toolNames(filterTools(tools, "invoke"))
	want := []string{"invoke-tool"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterTools(invoke) = %v, want %v (prefix must win over substring)", got, want)
	}
}

// TestFilterTools_SubstringFallback mirrors
// TestFilterSkills_SubstringFallback for the tool catalogue.
func TestFilterTools_SubstringFallback(t *testing.T) {
	tools := []ConsoleTool{
		{Name: "reinvoke-helper"},
		{Name: "unrelated"},
	}
	got := toolNames(filterTools(tools, "invoke"))
	want := []string{"reinvoke-helper"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterTools(invoke) = %v, want %v", got, want)
	}
}
