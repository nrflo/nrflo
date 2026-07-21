package consoleui

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestBuildInvokeArguments covers string quoting, numeric/bool passthrough,
// object JSON parsing, and empty-optional omission; required fields are
// always included even when empty. Array coercion (typedArrayValue) has its
// own dedicated test below.
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

// TestTypedArrayValue covers the four array-coercion branches: empty
// (required) input -> empty array, JSON-array passthrough, JSON-scalar wrap,
// and plain non-JSON text wrap.
func TestTypedArrayValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want any
	}{
		{"empty required -> empty array", "", []any{}},
		{"plain text wraps as one-element string array", "foo bar", []any{"foo bar"}},
		{"JSON array passes through unchanged", `["a","b"]`, []any{"a", "b"}},
		{"JSON string scalar wraps", `"foo"`, []any{"foo"}},
		{"JSON number scalar wraps", `42`, []any{float64(42)}},
		{"JSON bool scalar wraps", `true`, []any{true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := typedArrayValue(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("typedArrayValue(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

// TestBuildInvokeArguments_ArrayField exercises the array case through the
// full buildInvokeArguments/json.Marshal/json.Unmarshal round trip, matching
// a web_search-style {"queries": [...]}-shaped tool.
func TestBuildInvokeArguments_ArrayField(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []any
	}{
		{"plain text", "foo bar", []any{"foo bar"}},
		{"JSON array passthrough", `["a","b"]`, []any{"a", "b"}},
		{"JSON scalar wrap", `"foo"`, []any{"foo"}},
		{"JSON number wrap", `42`, []any{float64(42)}},
		{"empty required -> empty array", "", []any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := []argField{{Name: "queries", Type: "array", Required: true, Scalar: false}}
			raw := buildInvokeArguments(fields, map[string]string{"queries": tt.in})
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("buildInvokeArguments produced invalid JSON: %v (%s)", err, raw)
			}
			queries, ok := got["queries"]
			if !ok {
				t.Fatalf("queries field missing from %v", got)
			}
			if !reflect.DeepEqual(queries, tt.want) {
				t.Errorf("queries = %#v, want %#v", queries, tt.want)
			}
		})
	}
}

// TestFirstInvalidObjectField verifies the confirm-time object-JSON
// validation: only object-typed fields are checked, empty values are
// skipped, and the first non-empty invalid-JSON field's index is returned.
func TestFirstInvalidObjectField(t *testing.T) {
	fields := []argField{
		{Name: "name", Type: "string"},
		{Name: "meta", Type: "object"},
		{Name: "extra", Type: "object"},
	}
	tests := []struct {
		name   string
		values map[string]string
		want   int
	}{
		{"all valid/empty -> -1", map[string]string{"name": "x", "meta": `{"a":1}`, "extra": ""}, -1},
		{"non-object field ignored even if garbage", map[string]string{"name": "not json at all", "meta": "", "extra": ""}, -1},
		{"empty object field skipped", map[string]string{"meta": "", "extra": ""}, -1},
		{"first invalid object field returned", map[string]string{"meta": "not json", "extra": `{"b":2}`}, 1},
		{"second invalid object field returned when first is valid", map[string]string{"meta": `{"a":1}`, "extra": "{invalid"}, 2},
		{"no values at all -> -1", map[string]string{}, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstInvalidObjectField(fields, tt.values)
			if got != tt.want {
				t.Errorf("firstInvalidObjectField(%v) = %d, want %d", tt.values, got, tt.want)
			}
		})
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
