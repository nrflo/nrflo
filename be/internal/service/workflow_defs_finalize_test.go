package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/types"
)

// seedFinalizeScripts inserts an agent-kind and a tool-kind python_script into the test DB.
// Returns (agentScriptID, toolScriptID).
func seedFinalizeScripts(t *testing.T, svc *WorkflowService, projectID string) (string, string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	agentID := "fin-ps-agent"
	toolID := "fin-ps-tool"

	if _, err := svc.pool.Exec(
		`INSERT INTO python_scripts (id, project_id, name, description, code, kind, created_at, updated_at)
		 VALUES (?, ?, ?, '', '', 'agent', ?, ?)`,
		agentID, projectID, "fin-agent-script", now, now,
	); err != nil {
		t.Fatalf("insert agent python_script: %v", err)
	}
	if _, err := svc.pool.Exec(
		`INSERT INTO python_scripts (id, project_id, name, description, code, kind, tool_description, created_at, updated_at)
		 VALUES (?, ?, ?, '', '', 'tool', 'does work', ?, ?)`,
		toolID, projectID, "fin-tool-script", now, now,
	); err != nil {
		t.Fatalf("insert tool python_script: %v", err)
	}
	return agentID, toolID
}

// slotCreateRequest builds a WorkflowDefCreateRequest that sets command/script_id on
// the named slot ("success", "failure", or "pause"). A "use-agent"/"use-tool"
// scriptID sentinel is resolved to the seeded script IDs by the caller.
func slotCreateRequest(id, slot, cmd, scriptID string) *types.WorkflowDefCreateRequest {
	req := &types.WorkflowDefCreateRequest{ID: id}
	switch slot {
	case "success":
		req.FinalizeSuccessCommand = cmd
		req.FinalizeSuccessScriptID = scriptID
	case "failure":
		req.FinalizeFailureCommand = cmd
		req.FinalizeFailureScriptID = scriptID
	case "pause":
		req.PauseEventCommand = cmd
		req.PauseEventScriptID = scriptID
	}
	return req
}

// TestCreateWorkflowDef_SlotValidation covers the shared validateFinalizeSlot logic
// reached by all three slots (finalize_success, finalize_failure, pause_event).
// command-only and agent-kind script are accepted; both-set, missing script, and
// tool-kind script are rejected with the corresponding error.
func TestCreateWorkflowDef_SlotValidation(t *testing.T) {
	t.Parallel()

	// scriptID sentinels resolved against seeded scripts per subtest.
	const (
		useAgent = "<agent>"
		useTool  = "<tool>"
	)

	cases := []struct {
		name     string
		cmd      string
		scriptID string // "", literal, or a sentinel
		wantErr  string // substring; "" means success expected
	}{
		{name: "command_only", cmd: "echo done"},
		{name: "agent_kind_script", scriptID: useAgent},
		{name: "both_command_and_script", cmd: "echo ok", scriptID: "some-script", wantErr: "mutually exclusive"},
		{name: "missing_script", scriptID: "nonexistent-script", wantErr: "python_script_not_found"},
		{name: "tool_kind_script", scriptID: useTool, wantErr: "python_script_kind_mismatch"},
	}

	for _, slot := range []string{"success", "failure", "pause"} {
		for _, tc := range cases {
			t.Run(slot+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				_, svc := setupWorkflowDefsTestEnv(t)
				agentID, toolID := seedFinalizeScripts(t, svc, "proj1")

				scriptID := tc.scriptID
				switch scriptID {
				case useAgent:
					scriptID = agentID
				case useTool:
					scriptID = toolID
				}

				wfID := "wf-" + slot + "-" + tc.name
				_, err := svc.CreateWorkflowDef("proj1", slotCreateRequest(wfID, slot, tc.cmd, scriptID))

				if tc.wantErr == "" {
					if err != nil {
						t.Fatalf("CreateWorkflowDef(%s/%s): unexpected error: %v", slot, tc.name, err)
					}
					return
				}
				if err == nil {
					t.Fatalf("CreateWorkflowDef(%s/%s): expected error containing %q, got nil", slot, tc.name, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("CreateWorkflowDef(%s/%s): error = %q, want to contain %q", slot, tc.name, err.Error(), tc.wantErr)
				}
			})
		}
	}
}

// TestGetWorkflowDef_ReturnsFinalizeFields verifies Get returns all 4 finalize fields.
func TestGetWorkflowDef_ReturnsFinalizeFields(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                     "wf-fin-get",
		FinalizeSuccessCommand: "success-cmd",
		FinalizeFailureCommand: "failure-cmd",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-fin-get")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if def.FinalizeSuccessCommand != "success-cmd" {
		t.Errorf("FinalizeSuccessCommand = %q, want %q", def.FinalizeSuccessCommand, "success-cmd")
	}
	if def.FinalizeSuccessScriptID != "" {
		t.Errorf("FinalizeSuccessScriptID = %q, want empty", def.FinalizeSuccessScriptID)
	}
	if def.FinalizeFailureCommand != "failure-cmd" {
		t.Errorf("FinalizeFailureCommand = %q, want %q", def.FinalizeFailureCommand, "failure-cmd")
	}
	if def.FinalizeFailureScriptID != "" {
		t.Errorf("FinalizeFailureScriptID = %q, want empty", def.FinalizeFailureScriptID)
	}
}

// TestListWorkflowDefs_ReturnsFinalizeFields verifies List returns all 4 finalize fields.
func TestListWorkflowDefs_ReturnsFinalizeFields(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	agentID, _ := seedFinalizeScripts(t, svc, "proj1")
	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                      "wf-fin-list",
		FinalizeSuccessScriptID: agentID,
		FinalizeFailureCommand:  "on-fail",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	defs, err := svc.ListWorkflowDefs("proj1")
	if err != nil {
		t.Fatalf("ListWorkflowDefs: %v", err)
	}
	def, ok := defs["wf-fin-list"]
	if !ok {
		t.Fatal("wf-fin-list not in list result")
	}
	if def.FinalizeSuccessScriptID != agentID {
		t.Errorf("List FinalizeSuccessScriptID = %q, want %q", def.FinalizeSuccessScriptID, agentID)
	}
	if def.FinalizeSuccessCommand != "" {
		t.Errorf("List FinalizeSuccessCommand = %q, want empty", def.FinalizeSuccessCommand)
	}
	if def.FinalizeFailureCommand != "on-fail" {
		t.Errorf("List FinalizeFailureCommand = %q, want %q", def.FinalizeFailureCommand, "on-fail")
	}
	if def.FinalizeFailureScriptID != "" {
		t.Errorf("List FinalizeFailureScriptID = %q, want empty", def.FinalizeFailureScriptID)
	}
}
