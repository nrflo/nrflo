// Integration tests for the workflow.skip socket command end-to-end.
package integration

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/types"
	"be/internal/ws"
)

// TestWorkflowSkipSocket_HappyPath tests the full socket round-trip for workflow.skip.
func TestWorkflowSkipSocket_HappyPath(t *testing.T) {
	env := NewTestEnv(t)

	// Create a workflow with groups (env seeds "test" without groups)
	phasesJSON, _ := json.Marshal([]map[string]interface{}{{"agent": "analyzer", "layer": 0}})
	if _, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:     "skip-wf",
		Phases: phasesJSON,
		Groups: []string{"be", "fe", "docs"},
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	env.CreateTicket(t, "SKIP-E2E", "skip e2e test")

	if _, err := env.WorkflowSvc.Init(env.ProjectID, "SKIP-E2E", &types.WorkflowInitRequest{
		Workflow: "skip-wf",
	}); err != nil {
		t.Fatalf("Init workflow: %v", err)
	}

	wfiID := env.GetWorkflowInstanceID(t, "SKIP-E2E", "skip-wf")

	// Subscribe WS client before issuing skip
	_, recvCh := env.NewWSClient(t, "e2e-ws-client", "SKIP-E2E")

	// Execute workflow.skip via socket
	params := map[string]string{"instance_id": wfiID, "tag": "be"}
	var result map[string]string
	env.MustExecute(t, "workflow.skip", params, &result)

	if result["status"] != "added" {
		t.Errorf("status = %q, want %q", result["status"], "added")
	}
	if result["tag"] != "be" {
		t.Errorf("tag = %q, want %q", result["tag"], "be")
	}

	// Verify skip_tag.added WS event
	select {
	case msg := <-recvCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal ws event: %v", err)
		}
		if event.Type != ws.EventSkipTagAdded {
			t.Errorf("event type = %q, want %q", event.Type, ws.EventSkipTagAdded)
		}
		if event.Data["tag"] != "be" {
			t.Errorf("event.tag = %v, want %q", event.Data["tag"], "be")
		}
		if event.Data["instance_id"] != wfiID {
			t.Errorf("event.instance_id = %v, want %q", event.Data["instance_id"], wfiID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for skip_tag.added WS event")
	}

	// Verify skip_tags persisted in DB
	var skipTags string
	if err := env.Pool.QueryRow(`SELECT COALESCE(skip_tags, '[]') FROM workflow_instances WHERE id = ?`, wfiID).Scan(&skipTags); err != nil {
		t.Fatalf("read skip_tags: %v", err)
	}
	if skipTags != `["be"]` {
		t.Errorf("skip_tags in DB = %q, want %q", skipTags, `["be"]`)
	}
}

// TestWorkflowSkipSocket_ValidationErrors tests various error scenarios.
func TestWorkflowSkipSocket_ValidationErrors(t *testing.T) {
	env := NewTestEnv(t)

	cases := []struct {
		name         string
		params       interface{}
		expectedCode int
	}{
		{
			name:         "missing instance_id",
			params:       map[string]string{"tag": "be"},
			expectedCode: -32606,
		},
		{
			name:         "missing tag",
			params:       map[string]string{"instance_id": "some-id"},
			expectedCode: -32606,
		},
		{
			name:         "instance not found",
			params:       map[string]string{"instance_id": "nonexistent-id", "tag": "be"},
			expectedCode: -32604,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env.ExpectError(t, "workflow.skip", tc.params, tc.expectedCode)
		})
	}
}

// TestWorkflowSkipSocket_TagNotInGroups verifies tag validation via socket.
func TestWorkflowSkipSocket_TagNotInGroups(t *testing.T) {
	env := NewTestEnv(t)

	phasesJSON, _ := json.Marshal([]map[string]interface{}{{"agent": "analyzer", "layer": 0}})
	if _, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:     "wf-inv-tag",
		Phases: phasesJSON,
		Groups: []string{"be"},
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	env.CreateTicket(t, "SKIP-INVALID", "invalid tag test")
	if _, err := env.WorkflowSvc.Init(env.ProjectID, "SKIP-INVALID", &types.WorkflowInitRequest{Workflow: "wf-inv-tag"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	wfiID := env.GetWorkflowInstanceID(t, "SKIP-INVALID", "wf-inv-tag")

	env.ExpectError(t, "workflow.skip",
		map[string]string{"instance_id": wfiID, "tag": "docs"},
		-32606)
}

// TestWorkflowSkipSocket_Idempotent verifies duplicate tags are not stored twice.
func TestWorkflowSkipSocket_Idempotent(t *testing.T) {
	env := NewTestEnv(t)

	phasesJSON, _ := json.Marshal([]map[string]interface{}{{"agent": "analyzer", "layer": 0}})
	if _, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:     "wf-idem",
		Phases: phasesJSON,
		Groups: []string{"be", "fe"},
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	env.CreateTicket(t, "SKIP-IDEM", "idempotent test")
	if _, err := env.WorkflowSvc.Init(env.ProjectID, "SKIP-IDEM", &types.WorkflowInitRequest{Workflow: "wf-idem"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	wfiID := env.GetWorkflowInstanceID(t, "SKIP-IDEM", "wf-idem")

	params := map[string]string{"instance_id": wfiID, "tag": "be"}

	// Call twice — both should succeed
	env.MustExecute(t, "workflow.skip", params, nil)
	env.MustExecute(t, "workflow.skip", params, nil)

	// Verify only one "be" in skip_tags
	var skipTags string
	if err := env.Pool.QueryRow(`SELECT COALESCE(skip_tags, '[]') FROM workflow_instances WHERE id = ?`, wfiID).Scan(&skipTags); err != nil {
		t.Fatalf("read skip_tags: %v", err)
	}
	if skipTags != `["be"]` {
		t.Errorf("idempotent skip_tags = %q, want %q", skipTags, `["be"]`)
	}
}
