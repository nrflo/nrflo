package consoleui

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestInvokeQuery mirrors TestSlashQuery: "/invoke " requires the trailing
// space, stays single-line, and is mutually exclusive with slashQuery (a bare
// "/invoke" still falls through to skill filtering).
func TestInvokeQuery(t *testing.T) {
	tests := []struct {
		value     string
		wantQuery string
		wantOK    bool
	}{
		{"/invoke ", "", true},
		{"/invoke tick", "tick", true},
		{"/invoke ticket_list", "ticket_list", true},
		{"/invoke", "", false},
		{"/invoke \nfoo", "", false},
		{"/invoke foo\nbar", "", false},
		{"/inv", "", false},
		{"hi", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			query, ok := invokeQuery(tt.value)
			if ok != tt.wantOK || query != tt.wantQuery {
				t.Errorf("invokeQuery(%q) = (%q, %v), want (%q, %v)", tt.value, query, ok, tt.wantQuery, tt.wantOK)
			}
		})
	}
}

// TestInvokeQuery_MutuallyExclusiveWithSlashQuery verifies bare "/invoke"
// still matches slashQuery (skill mode) while "/invoke " matches invokeQuery
// (tool mode) and not slashQuery.
func TestInvokeQuery_MutuallyExclusiveWithSlashQuery(t *testing.T) {
	if _, ok := slashQuery("/invoke"); !ok {
		t.Errorf("slashQuery(%q) = false, want true (bare /invoke still filters skills)", "/invoke")
	}
	if _, ok := invokeQuery("/invoke"); ok {
		t.Errorf("invokeQuery(%q) = true, want false (missing trailing space)", "/invoke")
	}
	if _, ok := slashQuery("/invoke "); ok {
		t.Errorf("slashQuery(%q) = true, want false (space flips to tool mode)", "/invoke ")
	}
	if _, ok := invokeQuery("/invoke "); !ok {
		t.Errorf("invokeQuery(%q) = false, want true", "/invoke ")
	}
}

func fieldNames(fields []argField) []string {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
	}
	return names
}

// TestToolArgFields covers ordering (required-first in schema `required`
// order, then remaining sorted alphabetically), scalar-type detection, and
// default extraction.
func TestToolArgFields(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"zeta": {"type": "string"},
			"count": {"type": "integer", "default": 3},
			"tags": {"type": "array"},
			"name": {"type": "string", "default": "foo"},
			"meta": {"type": "object"}
		},
		"required": ["name", "count"]
	}`)
	fields := toolArgFields(schema)
	gotNames := fieldNames(fields)
	wantNames := []string{"name", "count", "meta", "tags", "zeta"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("toolArgFields order = %v, want %v", gotNames, wantNames)
	}

	byName := make(map[string]argField, len(fields))
	for _, f := range fields {
		byName[f.Name] = f
	}

	if !byName["name"].Required || !byName["count"].Required {
		t.Errorf("required fields name/count not marked Required")
	}
	for _, n := range []string{"meta", "tags", "zeta"} {
		if byName[n].Required {
			t.Errorf("field %q should not be Required", n)
		}
	}

	scalarWant := map[string]bool{"name": true, "count": true, "tags": false, "meta": false, "zeta": true}
	for name, want := range scalarWant {
		if byName[name].Scalar != want {
			t.Errorf("field %q Scalar = %v, want %v", name, byName[name].Scalar, want)
		}
	}

	if byName["name"].Default != "foo" {
		t.Errorf("field name Default = %q, want %q", byName["name"].Default, "foo")
	}
	if byName["count"].Default != "3" {
		t.Errorf("field count Default = %q, want %q", byName["count"].Default, "3")
	}
	if byName["zeta"].Default != "" {
		t.Errorf("field zeta Default = %q, want empty", byName["zeta"].Default)
	}
}

// TestToolArgFields_RequiredOrderPreservesSchemaOrder verifies required
// fields are emitted in the schema's `required` array order, not sorted.
func TestToolArgFields_RequiredOrderPreservesSchemaOrder(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"alpha": {"type": "string"},
			"beta": {"type": "string"}
		},
		"required": ["beta", "alpha"]
	}`)
	got := fieldNames(toolArgFields(schema))
	want := []string{"beta", "alpha"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toolArgFields required order = %v, want %v", got, want)
	}
}

// TestToolArgFields_EmptySchema verifies a ticket_list-style schema with no
// properties yields zero fields (straight-to-confirm).
func TestToolArgFields_EmptySchema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","additionalProperties":false}`)
	got := toolArgFields(schema)
	if len(got) != 0 {
		t.Errorf("toolArgFields(empty schema) = %v, want empty", got)
	}
}

// TestToolArgFields_BlankOrInvalidSchema verifies nil/blank/unparseable
// schemas degrade to zero fields rather than panicking.
func TestToolArgFields_BlankOrInvalidSchema(t *testing.T) {
	cases := []json.RawMessage{nil, json.RawMessage(""), json.RawMessage("   "), json.RawMessage("not json")}
	for _, schema := range cases {
		got := toolArgFields(schema)
		if len(got) != 0 {
			t.Errorf("toolArgFields(%q) = %v, want empty", string(schema), got)
		}
	}
}

// TestStartInvoke_FieldsJumpsToArgsPhase verifies a tool with fields starts
// in the args phase at index 0.
func TestStartInvoke_FieldsJumpsToArgsPhase(t *testing.T) {
	fields := []argField{{Name: "a"}, {Name: "b"}}
	st := startInvoke("mytool", fields)
	if !st.active {
		t.Fatalf("startInvoke should set active=true")
	}
	if st.phase != invokePhaseArgs {
		t.Errorf("startInvoke phase = %v, want invokePhaseArgs", st.phase)
	}
	if st.index != 0 {
		t.Errorf("startInvoke index = %d, want 0", st.index)
	}
	if !st.inform {
		t.Errorf("startInvoke inform default = false, want true")
	}
	if st.tool != "mytool" {
		t.Errorf("startInvoke tool = %q, want mytool", st.tool)
	}
}

// TestStartInvoke_ZeroFieldsJumpsToConfirm covers the ticket_list-style
// straight-to-confirm case.
func TestStartInvoke_ZeroFieldsJumpsToConfirm(t *testing.T) {
	st := startInvoke("ticket_list", nil)
	if st.phase != invokePhaseConfirm {
		t.Errorf("startInvoke with no fields phase = %v, want invokePhaseConfirm", st.phase)
	}
}

// TestAcceptArg_AdvancesAndStores verifies acceptArg stores the value under
// the current field's name and advances the index, reaching confirm phase
// once the last field is accepted.
func TestAcceptArg_AdvancesAndStores(t *testing.T) {
	fields := []argField{{Name: "a"}, {Name: "b"}}
	st := startInvoke("t", fields)

	st = acceptArg(st, "va")
	if st.index != 1 {
		t.Fatalf("after 1st acceptArg index = %d, want 1", st.index)
	}
	if st.phase != invokePhaseArgs {
		t.Fatalf("after 1st acceptArg phase = %v, want invokePhaseArgs", st.phase)
	}
	if st.values["a"] != "va" {
		t.Errorf("st.values[a] = %q, want va", st.values["a"])
	}

	st = acceptArg(st, "vb")
	if st.index != 2 {
		t.Fatalf("after 2nd acceptArg index = %d, want 2", st.index)
	}
	if st.phase != invokePhaseConfirm {
		t.Errorf("after last acceptArg phase = %v, want invokePhaseConfirm", st.phase)
	}
	if st.values["b"] != "vb" {
		t.Errorf("st.values[b] = %q, want vb", st.values["b"])
	}
}

// TestAcceptArg_NoopOutsideArgsPhase verifies acceptArg is a no-op once in
// confirm phase or with an out-of-range index.
func TestAcceptArg_NoopOutsideArgsPhase(t *testing.T) {
	st := startInvoke("t", nil) // zero fields -> confirm phase immediately
	got := acceptArg(st, "x")
	if !reflect.DeepEqual(got, st) {
		t.Errorf("acceptArg in confirm phase mutated state: got %+v, want unchanged %+v", got, st)
	}

	fields := []argField{{Name: "a"}}
	st2 := startInvoke("t", fields)
	st2.index = 5 // out of range
	got2 := acceptArg(st2, "x")
	if !reflect.DeepEqual(got2, st2) {
		t.Errorf("acceptArg with out-of-range index mutated state: got %+v, want unchanged %+v", got2, st2)
	}
}

// TestCancelInvoke verifies cancelInvoke resets to the zero (inactive) value
// regardless of the starting state.
func TestCancelInvoke(t *testing.T) {
	st := startInvoke("t", []argField{{Name: "a"}})
	st = acceptArg(st, "v")
	st = toggleInform(st)
	_ = st
	got := cancelInvoke()
	if got.active {
		t.Errorf("cancelInvoke().active = true, want false")
	}
	if !reflect.DeepEqual(got, invokeState{}) {
		t.Errorf("cancelInvoke() = %+v, want zero value", got)
	}
}

// TestToggleInform verifies toggleInform flips the flag and starts true.
func TestToggleInform(t *testing.T) {
	st := startInvoke("t", nil)
	if !st.inform {
		t.Fatalf("startInvoke default inform = false, want true")
	}
	st = toggleInform(st)
	if st.inform {
		t.Errorf("toggleInform did not flip to false")
	}
	st = toggleInform(st)
	if !st.inform {
		t.Errorf("toggleInform did not flip back to true")
	}
}
