package consoleui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestInvokeArgLine_Format verifies the "<name> (<type>[, required][, JSON]):"
// label shape: JSON hint only for object fields, required only when the
// field is required, and both together in the right order.
func TestInvokeArgLine_Format(t *testing.T) {
	tests := []struct {
		name  string
		field argField
		want  string
	}{
		{"optional string", argField{Name: "note", Type: "string"}, "note (string):"},
		{"required string", argField{Name: "name", Type: "string", Required: true}, "name (string, required):"},
		{"optional object gets JSON hint, no required", argField{Name: "meta", Type: "object"}, "meta (object, JSON):"},
		{"required object gets both", argField{Name: "meta", Type: "object", Required: true}, "meta (object, required, JSON):"},
		{"required array has no JSON hint", argField{Name: "tags", Type: "array", Required: true}, "tags (array, required):"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &model{invoke: invokeState{phase: invokePhaseArgs, fields: []argField{tt.field}, index: 0}}
			line := ansi.Strip(m.invokeArgLine())
			if !strings.HasPrefix(line, tt.want) {
				t.Errorf("invokeArgLine() = %q, want prefix %q", line, tt.want)
			}
		})
	}
}

// TestInvokeArgLine_OutOfRangeIndex verifies an out-of-range index (e.g. a
// zero-value invokeState) renders empty rather than panicking.
func TestInvokeArgLine_OutOfRangeIndex(t *testing.T) {
	m := &model{invoke: invokeState{phase: invokePhaseArgs, fields: nil, index: 0}}
	if got := m.invokeArgLine(); got != "" {
		t.Errorf("invokeArgLine() with no fields = %q, want empty", got)
	}

	m2 := &model{invoke: invokeState{phase: invokePhaseArgs, fields: []argField{{Name: "a"}}, index: 5}}
	if got := m2.invokeArgLine(); got != "" {
		t.Errorf("invokeArgLine() with out-of-range index = %q, want empty", got)
	}
}
