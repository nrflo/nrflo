package service

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/types"
)

// The reserved _workflow_plan schema must compile under JSON Schema Draft
// 2020 and its bundled example must validate against it — the same
// invariant ValidateFindingSchemas enforces for operator-supplied schemas.
func TestWorkflowPlanSchema_CompilesAndExampleValidates(t *testing.T) {
	t.Parallel()
	sch, err := compileJSONSchema(workflowPlanJSONSchema)
	if err != nil {
		t.Fatalf("workflowPlanJSONSchema failed to compile: %v", err)
	}
	var ex interface{}
	if err := json.Unmarshal([]byte(workflowPlanExample), &ex); err != nil {
		t.Fatalf("workflowPlanExample is not valid JSON: %v", err)
	}
	if err := sch.Validate(ex); err != nil {
		t.Fatalf("workflowPlanExample does not satisfy workflowPlanJSONSchema: %v", err)
	}
}

// An operator must never be able to declare/override the reserved
// _workflow_plan key via workflow-def create/update.
func TestValidateFindingSchemas_RejectsReservedWorkflowPlanKey(t *testing.T) {
	t.Parallel()
	defs := []types.FindingSchema{
		fs(WorkflowPlanFindingKey, `{"type":"object"}`, `{}`),
	}
	err := ValidateFindingSchemas(defs)
	if err == nil {
		t.Fatal("expected error declaring reserved key '_workflow_plan', got nil")
	}
	if !strings.Contains(err.Error(), WorkflowPlanFindingKey) {
		t.Fatalf("expected error to name %q, got %v", WorkflowPlanFindingKey, err)
	}
}

func TestIsReservedFindingKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key  string
		want bool
	}{
		{"_workflow_plan", true},
		{"_consult_answer", false},
		{"some_other_key", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			if got := IsReservedFindingKey(tc.key); got != tc.want {
				t.Errorf("IsReservedFindingKey(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

func TestGuardReservedFindingKey(t *testing.T) {
	t.Parallel()

	t.Run("reserved key returns error mentioning emit_findings", func(t *testing.T) {
		t.Parallel()
		err := GuardReservedFindingKey(WorkflowPlanFindingKey)
		if err == nil {
			t.Fatal("expected error for reserved key, got nil")
		}
		if !strings.Contains(err.Error(), "emit_findings") {
			t.Fatalf("expected error to mention 'emit_findings', got %v", err)
		}
	})

	t.Run("random key is not reserved", func(t *testing.T) {
		t.Parallel()
		if err := GuardReservedFindingKey("some_random_key"); err != nil {
			t.Fatalf("expected nil for a non-reserved key, got %v", err)
		}
	})

	// CRITICAL regression guard: _consult_answer is written by every
	// consultant agent via findings_add (spawner/consult.go) and must never
	// become reserved, or every consultant write breaks.
	t.Run("_consult_answer is NOT reserved", func(t *testing.T) {
		t.Parallel()
		if err := GuardReservedFindingKey("_consult_answer"); err != nil {
			t.Fatalf("_consult_answer must never be reserved (breaks spawner/consult.go findings_add), got error: %v", err)
		}
	})
}

func TestGuardReservedFindingKeys(t *testing.T) {
	t.Parallel()

	t.Run("no reserved keys present", func(t *testing.T) {
		t.Parallel()
		err := GuardReservedFindingKeys(map[string]string{
			"a": "1", "_consult_answer": "2", "b": "3",
		})
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("reserved key present", func(t *testing.T) {
		t.Parallel()
		err := GuardReservedFindingKeys(map[string]string{
			"a": "1", WorkflowPlanFindingKey: "2",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestReservedFindingKeys(t *testing.T) {
	t.Parallel()
	got := ReservedFindingKeys()
	want := []string{"_workflow_plan"}
	if len(got) != len(want) {
		t.Fatalf("ReservedFindingKeys() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ReservedFindingKeys() = %v, want %v", got, want)
		}
	}
}

func TestReservedFindingSchema(t *testing.T) {
	t.Parallel()

	t.Run("known reserved key", func(t *testing.T) {
		t.Parallel()
		schema, ok := ReservedFindingSchema(WorkflowPlanFindingKey)
		if !ok {
			t.Fatal("expected ok=true for reserved key")
		}
		if schema.Key != WorkflowPlanFindingKey {
			t.Fatalf("Key = %q, want %q", schema.Key, WorkflowPlanFindingKey)
		}
		if len(schema.Schema) == 0 || len(schema.Example) == 0 {
			t.Fatalf("expected non-empty schema/example, got %+v", schema)
		}
	})

	t.Run("unknown key", func(t *testing.T) {
		t.Parallel()
		_, ok := ReservedFindingSchema("not_reserved")
		if ok {
			t.Fatal("expected ok=false for non-reserved key")
		}
	})
}
