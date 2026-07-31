package service

import (
	"testing"

	"be/internal/types"
)

// TestSystemAgentDef_Create_IsolateWorktree_DefaultsFalse verifies Create
// defaults isolate_worktree to false when the request omits it.
func TestSystemAgentDef_Create_IsolateWorktree_DefaultsFalse(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	def, err := svc.Create(&types.SystemAgentDefCreateRequest{ID: "iso-default", Prompt: "p", Model: "sonnet-5"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if def.IsolateWorktree {
		t.Errorf("IsolateWorktree = true, want false by default")
	}

	got, err := svc.Get("iso-default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.IsolateWorktree {
		t.Errorf("Get IsolateWorktree = true, want false by default")
	}
}

// TestSystemAgentDef_Create_IsolateWorktree_ExplicitTrue verifies Create
// persists an explicit true.
func TestSystemAgentDef_Create_IsolateWorktree_ExplicitTrue(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	iso := true
	def, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID: "iso-true", Prompt: "p", Model: "sonnet-5", IsolateWorktree: &iso,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !def.IsolateWorktree {
		t.Errorf("IsolateWorktree = false, want true")
	}

	got, err := svc.Get("iso-true")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.IsolateWorktree {
		t.Errorf("Get IsolateWorktree = false, want true")
	}
}

// TestSystemAgentDef_Update_IsolateWorktree_ToggledAndPreserved verifies
// Update sets isolate_worktree when supplied and a subsequent no-field
// update leaves it untouched.
func TestSystemAgentDef_Update_IsolateWorktree_ToggledAndPreserved(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.SystemAgentDefCreateRequest{ID: "iso-upd", Prompt: "p", Model: "sonnet-5"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	iso := true
	if err := svc.Update("iso-upd", &types.SystemAgentDefUpdateRequest{IsolateWorktree: &iso}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := svc.Get("iso-upd")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.IsolateWorktree {
		t.Errorf("IsolateWorktree = false after Update(true), want true")
	}

	// No-op update (field omitted) must not reset it back to false.
	if err := svc.Update("iso-upd", &types.SystemAgentDefUpdateRequest{}); err != nil {
		t.Fatalf("no-op Update: %v", err)
	}
	got2, err := svc.Get("iso-upd")
	if err != nil {
		t.Fatalf("Get after no-op update: %v", err)
	}
	if !got2.IsolateWorktree {
		t.Errorf("IsolateWorktree = false after no-op update, want preserved true")
	}
}

// TestSystemAgentDef_T1Executor_SeededIsolateWorktreeTrue verifies migration
// 000224's seed: _t1_executor is isolated by default, unlike other tiers.
func TestSystemAgentDef_T1Executor_SeededIsolateWorktreeTrue(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	executor, err := svc.Get("_t1_executor")
	if err != nil {
		t.Fatalf("Get _t1_executor: %v", err)
	}
	if !executor.IsolateWorktree {
		t.Errorf("_t1_executor IsolateWorktree = false, want true (seeded default)")
	}

	extractor, err := svc.Get("_t2_extractor")
	if err != nil {
		t.Fatalf("Get _t2_extractor: %v", err)
	}
	if extractor.IsolateWorktree {
		t.Errorf("_t2_extractor IsolateWorktree = true, want false")
	}
}
