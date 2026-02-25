package service

import (
	"testing"

	"be/internal/types"
)

// TestCreateAgentDef_WithMaxFailRestarts verifies that max_fail_restarts is persisted on create.
func TestCreateAgentDef_WithMaxFailRestarts(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	n := 3
	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:              "agent-mfr",
		Prompt:          "do work",
		MaxFailRestarts: &n,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.MaxFailRestarts == nil {
		t.Fatal("MaxFailRestarts is nil, want non-nil")
	}
	if *def.MaxFailRestarts != 3 {
		t.Errorf("MaxFailRestarts = %d, want 3", *def.MaxFailRestarts)
	}
}

// TestCreateAgentDef_NilMaxFailRestarts confirms that omitting max_fail_restarts
// produces nil (not 0) — preserving the "not configured" sentinel.
func TestCreateAgentDef_NilMaxFailRestarts(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-no-mfr",
		Prompt: "do work",
		// MaxFailRestarts omitted
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.MaxFailRestarts != nil {
		t.Errorf("MaxFailRestarts = %v, want nil", def.MaxFailRestarts)
	}
}

// TestGetAgentDef_ReturnsMaxFailRestarts verifies the round-trip through GetAgentDef.
func TestGetAgentDef_ReturnsMaxFailRestarts(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	n := 5
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:              "agent-get-mfr",
		Prompt:          "do work",
		MaxFailRestarts: &n,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "agent-get-mfr")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.MaxFailRestarts == nil {
		t.Fatal("MaxFailRestarts is nil after get, want non-nil")
	}
	if *def.MaxFailRestarts != 5 {
		t.Errorf("MaxFailRestarts = %d, want 5", *def.MaxFailRestarts)
	}
}

// TestGetAgentDef_MaxFailRestartsNilRoundTrip verifies nil is returned when
// max_fail_restarts was never set (DB column is NULL).
func TestGetAgentDef_MaxFailRestartsNilRoundTrip(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-nil-mfr",
		Prompt: "do work",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "agent-nil-mfr")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.MaxFailRestarts != nil {
		t.Errorf("MaxFailRestarts = %v, want nil for unset field", def.MaxFailRestarts)
	}
}

// TestUpdateAgentDef_SetsMaxFailRestarts verifies that UpdateAgentDef can set
// max_fail_restarts on an existing definition.
func TestUpdateAgentDef_SetsMaxFailRestarts(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-upd-mfr",
		Prompt: "do work",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	n := 2
	if err := svc.UpdateAgentDef("proj1", wfID, "agent-upd-mfr", &types.AgentDefUpdateRequest{
		MaxFailRestarts: &n,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "agent-upd-mfr")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	if def.MaxFailRestarts == nil {
		t.Fatal("MaxFailRestarts is nil after update, want non-nil")
	}
	if *def.MaxFailRestarts != 2 {
		t.Errorf("MaxFailRestarts = %d, want 2", *def.MaxFailRestarts)
	}
}

// TestUpdateAgentDef_ChangesMaxFailRestarts verifies that the value can be
// changed from one non-zero value to another.
func TestUpdateAgentDef_ChangesMaxFailRestarts(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	initial := 1
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:              "agent-chg-mfr",
		Prompt:          "do work",
		MaxFailRestarts: &initial,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated := 10
	if err := svc.UpdateAgentDef("proj1", wfID, "agent-chg-mfr", &types.AgentDefUpdateRequest{
		MaxFailRestarts: &updated,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "agent-chg-mfr")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.MaxFailRestarts == nil || *def.MaxFailRestarts != 10 {
		t.Errorf("MaxFailRestarts = %v, want 10", def.MaxFailRestarts)
	}
}

// TestListAgentDefs_ReturnsMaxFailRestarts verifies that ListAgentDefs includes
// max_fail_restarts values correctly.
func TestListAgentDefs_ReturnsMaxFailRestarts(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	n := 4
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:              "list-mfr-agent",
		Prompt:          "do work",
		MaxFailRestarts: &n,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Also create one without max_fail_restarts
	_, err = svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "list-no-mfr-agent",
		Prompt: "do work",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	defs, err := svc.ListAgentDefs("proj1", wfID)
	if err != nil {
		t.Fatalf("ListAgentDefs: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}

	var withMFR, withoutMFR int
	for _, d := range defs {
		if d.MaxFailRestarts != nil {
			withMFR++
			if *d.MaxFailRestarts != 4 {
				t.Errorf("MaxFailRestarts = %d, want 4", *d.MaxFailRestarts)
			}
		} else {
			withoutMFR++
		}
	}
	if withMFR != 1 || withoutMFR != 1 {
		t.Errorf("withMFR=%d withoutMFR=%d, want 1 each", withMFR, withoutMFR)
	}
}

// TestCreateAgentDef_MaxFailRestartsZero verifies that explicitly passing 0
// stores it as 0 (not nil) — callers can explicitly disable.
func TestCreateAgentDef_MaxFailRestartsZero(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	zero := 0
	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:              "agent-zero-mfr",
		Prompt:          "do work",
		MaxFailRestarts: &zero,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.MaxFailRestarts == nil {
		t.Fatal("MaxFailRestarts is nil, want non-nil pointer to 0")
	}
	if *def.MaxFailRestarts != 0 {
		t.Errorf("MaxFailRestarts = %d, want 0", *def.MaxFailRestarts)
	}
}
