package service

import (
	"fmt"
	"testing"

	"be/internal/types"
)

// assertIntPtr fails the test unless got and want are both nil or point to equal values.
func assertIntPtr(t *testing.T, label string, got, want *int) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %d, want nil", label, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %d", label, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s = %d, want %d", label, *got, *want)
	}
}

// TestAgentDef_StallTimeouts_CreateGetRoundTrip covers the two nullable-int stall
// timeout fields through Create + Get: set values round-trip, omitted fields stay nil,
// and explicit 0 stays a non-nil pointer to 0 ("disabled").
func TestAgentDef_StallTimeouts_CreateGetRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		start       *int
		running     *int
		wantStart   *int
		wantRunning *int
	}{
		{name: "both_set", start: intPtr(60), running: intPtr(300), wantStart: intPtr(60), wantRunning: intPtr(300)},
		{name: "nil_sentinel", start: nil, running: nil, wantStart: nil, wantRunning: nil},
		{name: "zero_sentinel", start: intPtr(0), running: intPtr(0), wantStart: intPtr(0), wantRunning: intPtr(0)},
		{name: "start_only", start: intPtr(90), running: nil, wantStart: intPtr(90), wantRunning: nil},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, svc, wfID := setupAgentDefTestEnv(t, nil)
			id := fmt.Sprintf("stall-%d", i)

			def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:                     id,
				Prompt:                 "do work",
				StallStartTimeoutSec:   tc.start,
				StallRunningTimeoutSec: tc.running,
			})
			if err != nil {
				t.Fatalf("CreateAgentDef: %v", err)
			}
			assertIntPtr(t, "create StallStartTimeoutSec", def.StallStartTimeoutSec, tc.wantStart)
			assertIntPtr(t, "create StallRunningTimeoutSec", def.StallRunningTimeoutSec, tc.wantRunning)

			got, err := svc.GetAgentDef("proj1", wfID, id)
			if err != nil {
				t.Fatalf("GetAgentDef: %v", err)
			}
			assertIntPtr(t, "get StallStartTimeoutSec", got.StallStartTimeoutSec, tc.wantStart)
			assertIntPtr(t, "get StallRunningTimeoutSec", got.StallRunningTimeoutSec, tc.wantRunning)
		})
	}
}

// TestAgentDef_StallTimeouts_Update verifies UpdateAgentDef can set both stall timeouts
// on a def that lacked them.
func TestAgentDef_StallTimeouts_Update(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "upd-stall", Prompt: "do work",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.UpdateAgentDef("proj1", wfID, "upd-stall", &types.AgentDefUpdateRequest{
		StallStartTimeoutSec:   intPtr(200),
		StallRunningTimeoutSec: intPtr(600),
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "upd-stall")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	assertIntPtr(t, "StallStartTimeoutSec after update", def.StallStartTimeoutSec, intPtr(200))
	assertIntPtr(t, "StallRunningTimeoutSec after update", def.StallRunningTimeoutSec, intPtr(600))
}

// TestAgentDef_StallTimeouts_List verifies ListAgentDefs returns stall timeout values for
// set and unset defs.
func TestAgentDef_StallTimeouts_List(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "list-stall-agent", Prompt: "do work",
		StallStartTimeoutSec: intPtr(150), StallRunningTimeoutSec: intPtr(500),
	}); err != nil {
		t.Fatalf("create with stall timeouts: %v", err)
	}
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "list-no-stall-agent", Prompt: "do work",
	}); err != nil {
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
