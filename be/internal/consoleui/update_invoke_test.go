package consoleui

import (
	"testing"

	"charm.land/bubbles/v2/textarea"
)

// newInvokeTestModel builds a *model literal sufficient to exercise
// handleInvokeKey/prefillInvokeComposer without a terminal or client.
func newInvokeTestModel(st invokeState) *model {
	input := textarea.New()
	return &model{input: input, invoke: st}
}

// TestHandleInvokeKey_ConfirmYWithInvalidObject_RoutesBackToArgs verifies
// pressing "y" at confirm with an invalid-JSON object field routes the flow
// back to invokePhaseArgs at that field's index, sets m.notice, and does not
// dispatch (cmd is nil, no cancel to inactive state).
func TestHandleInvokeKey_ConfirmYWithInvalidObject_RoutesBackToArgs(t *testing.T) {
	fields := []argField{
		{Name: "name", Type: "string", Required: true},
		{Name: "meta", Type: "object"},
	}
	st := invokeState{
		active: true,
		tool:   "mytool",
		fields: fields,
		values: map[string]string{"name": "x", "meta": "not json"},
		phase:  invokePhaseConfirm,
		inform: true,
	}
	m := newInvokeTestModel(st)
	cmd, handled := m.handleInvokeKey("y")
	if !handled {
		t.Fatalf("handleInvokeKey(y) handled = false, want true")
	}
	if cmd != nil {
		t.Errorf("handleInvokeKey(y) with invalid object field returned a dispatch cmd, want nil")
	}
	if m.invoke.phase != invokePhaseArgs {
		t.Errorf("m.invoke.phase = %v, want invokePhaseArgs", m.invoke.phase)
	}
	if m.invoke.index != 1 {
		t.Errorf("m.invoke.index = %d, want 1 (the invalid meta field)", m.invoke.index)
	}
	if m.notice != "meta: expected valid JSON" {
		t.Errorf("m.notice = %q, want %q", m.notice, "meta: expected valid JSON")
	}
	if !m.invoke.active {
		t.Errorf("m.invoke.active = false, want still true (flow not cancelled)")
	}
	if got := m.input.Value(); got != "not json" {
		t.Errorf("composer prefilled with %q, want the prior invalid value %q", got, "not json")
	}
}

// TestHandleInvokeKey_ConfirmYWithValidFields_Dispatches verifies "y" at
// confirm with all-valid fields cancels the invoke flow (returns to
// inactive) and returns a non-nil dispatch cmd.
func TestHandleInvokeKey_ConfirmYWithValidFields_Dispatches(t *testing.T) {
	fields := []argField{
		{Name: "name", Type: "string", Required: true},
		{Name: "meta", Type: "object"},
	}
	st := invokeState{
		active: true,
		tool:   "mytool",
		fields: fields,
		values: map[string]string{"name": "x", "meta": `{"a":1}`},
		phase:  invokePhaseConfirm,
		inform: true,
	}
	m := newInvokeTestModel(st)
	cmd, handled := m.handleInvokeKey("y")
	if !handled {
		t.Fatalf("handleInvokeKey(y) handled = false, want true")
	}
	if cmd == nil {
		t.Errorf("handleInvokeKey(y) with valid fields returned nil cmd, want dispatch cmd")
	}
	if m.invoke.active {
		t.Errorf("m.invoke.active = true, want false (flow cancelled after dispatch)")
	}
}

// TestPrefillInvokeComposer_PrefersStoredValueOverDefault verifies the
// composer is prefilled from m.invoke.values (the user's prior input) rather
// than the field's schema default, so routing back for correction doesn't
// clobber it.
func TestPrefillInvokeComposer_PrefersStoredValueOverDefault(t *testing.T) {
	fields := []argField{{Name: "meta", Type: "object", Default: `{"default":true}`}}
	st := invokeState{
		phase:  invokePhaseArgs,
		fields: fields,
		index:  0,
		values: map[string]string{"meta": "bad json"},
	}
	m := newInvokeTestModel(st)
	m.prefillInvokeComposer()
	if got := m.input.Value(); got != "bad json" {
		t.Errorf("composer = %q, want stored value %q (not the default)", got, "bad json")
	}
}

// TestPrefillInvokeComposer_FallsBackToDefaultWhenNoStoredValue verifies a
// field never yet visited (no entry in values) still prefills from Default.
func TestPrefillInvokeComposer_FallsBackToDefaultWhenNoStoredValue(t *testing.T) {
	fields := []argField{{Name: "count", Type: "integer", Default: "3"}}
	st := invokeState{phase: invokePhaseArgs, fields: fields, index: 0, values: map[string]string{}}
	m := newInvokeTestModel(st)
	m.prefillInvokeComposer()
	if got := m.input.Value(); got != "3" {
		t.Errorf("composer = %q, want default %q", got, "3")
	}
}

// TestHandleInvokeKey_ArgsPhaseEnter_AdvancesAndPrefills verifies the args
// phase "enter" branch (unrelated to the object-validation change) still
// stores the value and prefills the next field's default.
func TestHandleInvokeKey_ArgsPhaseEnter_AdvancesAndPrefills(t *testing.T) {
	fields := []argField{
		{Name: "a", Type: "string"},
		{Name: "b", Type: "string", Default: "b-default"},
	}
	st := startInvoke("t", fields)
	m := newInvokeTestModel(st)
	m.input.SetValue("va")
	cmd, handled := m.handleInvokeKey("enter")
	if !handled || cmd != nil {
		t.Fatalf("handleInvokeKey(enter) = (%v, %v), want (nil, true)", cmd, handled)
	}
	if m.invoke.index != 1 {
		t.Fatalf("m.invoke.index = %d, want 1", m.invoke.index)
	}
	if m.invoke.values["a"] != "va" {
		t.Errorf("m.invoke.values[a] = %q, want va", m.invoke.values["a"])
	}
	if got := m.input.Value(); got != "b-default" {
		t.Errorf("composer after advancing = %q, want next field's default %q", got, "b-default")
	}
}
