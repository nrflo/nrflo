package service

import (
	"testing"

	"be/internal/types"
)

// TestCreateAgentDef_WithStallStartTimeoutSec verifies stall_start_timeout_sec is persisted.
func TestCreateAgentDef_WithStallStartTimeoutSec(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	n := 90
	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                   "agent-stall-start",
		Prompt:               "do work",
		StallStartTimeoutSec: &n,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.StallStartTimeoutSec == nil {
		t.Fatal("StallStartTimeoutSec is nil, want non-nil")
	}
	if *def.StallStartTimeoutSec != 90 {
		t.Errorf("StallStartTimeoutSec = %d, want 90", *def.StallStartTimeoutSec)
	}
}

// TestCreateAgentDef_WithBothStallTimeouts verifies both stall timeout fields persist together.
func TestCreateAgentDef_WithBothStallTimeouts(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	start := 120
	running := 480
	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                     "agent-both-stall",
		Prompt:                 "do work",
		StallStartTimeoutSec:   &start,
		StallRunningTimeoutSec: &running,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.StallStartTimeoutSec == nil || *def.StallStartTimeoutSec != 120 {
		t.Errorf("StallStartTimeoutSec = %v, want 120", def.StallStartTimeoutSec)
	}
	if def.StallRunningTimeoutSec == nil || *def.StallRunningTimeoutSec != 480 {
		t.Errorf("StallRunningTimeoutSec = %v, want 480", def.StallRunningTimeoutSec)
	}
}

// TestCreateAgentDef_NilStallTimeouts confirms omitting stall timeout fields produces nil.
func TestCreateAgentDef_NilStallTimeouts(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-no-stall",
		Prompt: "do work",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.StallStartTimeoutSec != nil {
		t.Errorf("StallStartTimeoutSec = %v, want nil", def.StallStartTimeoutSec)
	}
	if def.StallRunningTimeoutSec != nil {
		t.Errorf("StallRunningTimeoutSec = %v, want nil", def.StallRunningTimeoutSec)
	}
}

// TestCreateAgentDef_StallTimeoutsZero verifies explicit 0 means "disabled", not nil.
func TestCreateAgentDef_StallTimeoutsZero(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	zero := 0
	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                     "agent-zero-stall",
		Prompt:                 "do work",
		StallStartTimeoutSec:   &zero,
		StallRunningTimeoutSec: &zero,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.StallStartTimeoutSec == nil {
		t.Fatal("StallStartTimeoutSec is nil, want non-nil pointer to 0")
	}
	if *def.StallStartTimeoutSec != 0 {
		t.Errorf("StallStartTimeoutSec = %d, want 0", *def.StallStartTimeoutSec)
	}
	if def.StallRunningTimeoutSec == nil {
		t.Fatal("StallRunningTimeoutSec is nil, want non-nil pointer to 0")
	}
	if *def.StallRunningTimeoutSec != 0 {
		t.Errorf("StallRunningTimeoutSec = %d, want 0", *def.StallRunningTimeoutSec)
	}
}

// TestGetAgentDef_ReturnsStallTimeouts verifies round-trip through GetAgentDef.
func TestGetAgentDef_ReturnsStallTimeouts(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	start := 60
	running := 300
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                     "agent-get-stall",
		Prompt:                 "do work",
		StallStartTimeoutSec:   &start,
		StallRunningTimeoutSec: &running,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "agent-get-stall")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.StallStartTimeoutSec == nil || *def.StallStartTimeoutSec != 60 {
		t.Errorf("StallStartTimeoutSec = %v, want 60", def.StallStartTimeoutSec)
	}
	if def.StallRunningTimeoutSec == nil || *def.StallRunningTimeoutSec != 300 {
		t.Errorf("StallRunningTimeoutSec = %v, want 300", def.StallRunningTimeoutSec)
	}
}

// TestGetAgentDef_StallTimeoutsNilRoundTrip verifies nil returned when never set.
func TestGetAgentDef_StallTimeoutsNilRoundTrip(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-nil-stall",
		Prompt: "do work",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "agent-nil-stall")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.StallStartTimeoutSec != nil {
		t.Errorf("StallStartTimeoutSec = %v, want nil", def.StallStartTimeoutSec)
	}
	if def.StallRunningTimeoutSec != nil {
		t.Errorf("StallRunningTimeoutSec = %v, want nil", def.StallRunningTimeoutSec)
	}
}

// TestUpdateAgentDef_SetsStallTimeouts verifies UpdateAgentDef can set stall timeouts.
func TestUpdateAgentDef_SetsStallTimeouts(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-upd-stall",
		Prompt: "do work",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	start := 200
	running := 600
	if err := svc.UpdateAgentDef("proj1", wfID, "agent-upd-stall", &types.AgentDefUpdateRequest{
		StallStartTimeoutSec:   &start,
		StallRunningTimeoutSec: &running,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "agent-upd-stall")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	if def.StallStartTimeoutSec == nil || *def.StallStartTimeoutSec != 200 {
		t.Errorf("StallStartTimeoutSec = %v, want 200", def.StallStartTimeoutSec)
	}
	if def.StallRunningTimeoutSec == nil || *def.StallRunningTimeoutSec != 600 {
		t.Errorf("StallRunningTimeoutSec = %v, want 600", def.StallRunningTimeoutSec)
	}
}

// TestListAgentDefs_ReturnsStallTimeouts verifies list includes stall timeout values.
func TestListAgentDefs_ReturnsStallTimeouts(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	start := 150
	running := 500
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                     "list-stall-agent",
		Prompt:                 "do work",
		StallStartTimeoutSec:   &start,
		StallRunningTimeoutSec: &running,
	})
	if err != nil {
		t.Fatalf("create with stall timeouts: %v", err)
	}

	// Also create one without stall timeouts
	_, err = svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "list-no-stall-agent",
		Prompt: "do work",
	})
	if err != nil {
		t.Fatalf("create without stall timeouts: %v", err)
	}

	defs, err := svc.ListAgentDefs("proj1", wfID)
	if err != nil {
		t.Fatalf("ListAgentDefs: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}

	var withStall, withoutStall int
	for _, d := range defs {
		if d.StallStartTimeoutSec != nil {
			withStall++
			if *d.StallStartTimeoutSec != 150 {
				t.Errorf("StallStartTimeoutSec = %d, want 150", *d.StallStartTimeoutSec)
			}
			if d.StallRunningTimeoutSec == nil || *d.StallRunningTimeoutSec != 500 {
				t.Errorf("StallRunningTimeoutSec = %v, want 500", d.StallRunningTimeoutSec)
			}
		} else {
			withoutStall++
		}
	}
	if withStall != 1 || withoutStall != 1 {
		t.Errorf("withStall=%d withoutStall=%d, want 1 each", withStall, withoutStall)
	}
}
