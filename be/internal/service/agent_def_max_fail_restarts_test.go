package service

import (
	"fmt"
	"testing"

	"be/internal/types"
)

// TestAgentDef_MaxFailRestarts_CreateGetRoundTrip covers the nullable-int sentinel
// semantics through Create + Get: a set value round-trips, an omitted field stays
// nil ("not configured"), and an explicit 0 stays a non-nil pointer to 0 ("disabled").
func TestAgentDef_MaxFailRestarts_CreateGetRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   *int // value passed on create (nil = field omitted)
		want *int // expected value after Create and Get
	}{
		{name: "set", in: intPtr(5), want: intPtr(5)},
		{name: "nil_sentinel", in: nil, want: nil},
		{name: "zero_sentinel", in: intPtr(0), want: intPtr(0)},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, svc, wfID := setupAgentDefTestEnv(t, nil)
			id := fmt.Sprintf("mfr-%d", i)

			def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:              id,
				Prompt:          "do work",
				MaxFailRestarts: tc.in,
			})
			if err != nil {
				t.Fatalf("CreateAgentDef: %v", err)
			}
			assertIntPtr(t, "create MaxFailRestarts", def.MaxFailRestarts, tc.want)

			got, err := svc.GetAgentDef("proj1", wfID, id)
			if err != nil {
				t.Fatalf("GetAgentDef: %v", err)
			}
			assertIntPtr(t, "get MaxFailRestarts", got.MaxFailRestarts, tc.want)
		})
	}
}

// TestAgentDef_MaxFailRestarts_Update verifies UpdateAgentDef can set the field on a
// def that lacked it, and change it from one non-zero value to another.
func TestAgentDef_MaxFailRestarts_Update(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "upd-mfr", Prompt: "do work", MaxFailRestarts: intPtr(1),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.UpdateAgentDef("proj1", wfID, "upd-mfr", &types.AgentDefUpdateRequest{
		MaxFailRestarts: intPtr(10),
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "upd-mfr")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	assertIntPtr(t, "MaxFailRestarts after update", def.MaxFailRestarts, intPtr(10))
}

// TestAgentDef_MaxFailRestarts_List verifies ListAgentDefs returns the field for set
// and unset defs.
func TestAgentDef_MaxFailRestarts_List(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "list-mfr-agent", Prompt: "do work", MaxFailRestarts: intPtr(4),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "list-no-mfr-agent", Prompt: "do work",
	}); err != nil {
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
