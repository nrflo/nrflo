package service

import (
	"testing"

	"be/internal/types"
)

// TestSystemAgentDef_List split out of system_agent_definition_test.go to stay
// under the file-size ratchet after the planner-system/planner-system-api seed
// rows (migration 000158) required deleting them here too.
func TestSystemAgentDef_List(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	// Delete seeded data so test starts from a known empty state.
	_ = svc.Delete("conflict-resolver")
	_ = svc.Delete("context-saver")
	_ = svc.Delete("context-saver-api")
	_ = svc.Delete("spec-normalizer")
	_ = svc.Delete("planner-system")
	_ = svc.Delete("planner-system-api")

	// Initially empty.
	defs, err := svc.List()
	if err != nil {
		t.Fatalf("List (empty): %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("initial List len = %d, want 0", len(defs))
	}

	ids := []string{"agent-b", "agent-a", "agent-c"}
	for _, id := range ids {
		if _, err := svc.Create(&types.SystemAgentDefCreateRequest{ID: id, Prompt: "p"}); err != nil {
			t.Fatalf("Create %q: %v", id, err)
		}
	}

	defs, err = svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(defs) != 3 {
		t.Fatalf("List len = %d, want 3", len(defs))
	}

	// Verify ORDER BY id ascending.
	wantOrder := []string{"agent-a", "agent-b", "agent-c"}
	for i, want := range wantOrder {
		if defs[i].ID != want {
			t.Errorf("List[%d].ID = %q, want %q", i, defs[i].ID, want)
		}
	}
}
