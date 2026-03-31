package socket

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// createWorkflowWithGroups is a helper that creates a workflow def and ticket workflow instance
// for testing workflow.skip. Returns the workflow instance ID.
func createWorkflowWithGroups(t *testing.T, env *handlerTestEnv, workflowID, ticketID string, groups []string) string {
	t.Helper()

	workflowSvc := service.NewWorkflowService(env.pool, clock.Real())
	phasesJSON, _ := json.Marshal([]map[string]interface{}{{"agent": "analyzer", "layer": 0}})
	if _, err := workflowSvc.CreateWorkflowDef(env.project, &types.WorkflowDefCreateRequest{
		ID:     workflowID,
		Phases: phasesJSON,
		Groups: groups,
	}); err != nil {
		t.Fatalf("CreateWorkflowDef(%s): %v", workflowID, err)
	}

	ticketSvc := service.NewTicketService(env.pool, clock.Real())
	if _, err := ticketSvc.Create(env.project, &types.TicketCreateRequest{
		ID:    ticketID,
		Title: ticketID,
	}); err != nil {
		t.Fatalf("create ticket %s: %v", ticketID, err)
	}

	if _, err := workflowSvc.Init(env.project, ticketID, &types.WorkflowInitRequest{Workflow: workflowID}); err != nil {
		t.Fatalf("init workflow %s/%s: %v", ticketID, workflowID, err)
	}

	var wfiID string
	if err := env.pool.QueryRow(
		`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, ticketID, workflowID).Scan(&wfiID); err != nil {
		t.Fatalf("get wfi for %s/%s: %v", ticketID, workflowID, err)
	}
	return wfiID
}

// TestWorkflowSkip_HappyPath verifies a valid workflow.skip succeeds and returns status=added.
func TestWorkflowSkip_HappyPath(t *testing.T) {
	env := newHandlerTestEnv(t)
	wfiID := createWorkflowWithGroups(t, env, "wf-skip-hp", "SKIP-HP", []string{"be", "fe"})

	params, _ := json.Marshal(map[string]string{"instance_id": wfiID, "tag": "be"})
	req := Request{ID: "req-skip-hp", Method: "workflow.skip", Project: env.project, Params: params}
	resp := env.handler.Handle(req)

	if resp.Error != nil {
		t.Fatalf("workflow.skip: unexpected error: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}

	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "added" {
		t.Errorf("status = %q, want %q", result["status"], "added")
	}
	if result["tag"] != "be" {
		t.Errorf("tag = %q, want %q", result["tag"], "be")
	}
}

// TestWorkflowSkip_MissingInstanceID verifies missing instance_id returns validation error.
func TestWorkflowSkip_MissingInstanceID(t *testing.T) {
	env := newHandlerTestEnv(t)

	params, _ := json.Marshal(map[string]string{"tag": "be"})
	req := Request{ID: "req-skip-noid", Method: "workflow.skip", Project: env.project, Params: params}
	resp := env.handler.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected error for missing instance_id, got success")
	}
	if resp.Error.Code != -32606 {
		t.Errorf("expected code -32606 (validation), got %d", resp.Error.Code)
	}
}

// TestWorkflowSkip_MissingTag verifies missing tag returns validation error.
func TestWorkflowSkip_MissingTag(t *testing.T) {
	env := newHandlerTestEnv(t)

	params, _ := json.Marshal(map[string]string{"instance_id": "some-id"})
	req := Request{ID: "req-skip-notag", Method: "workflow.skip", Project: env.project, Params: params}
	resp := env.handler.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected error for missing tag, got success")
	}
	if resp.Error.Code != -32606 {
		t.Errorf("expected code -32606 (validation), got %d", resp.Error.Code)
	}
}

// TestWorkflowSkip_InstanceNotFound verifies nonexistent instance returns not-found error.
func TestWorkflowSkip_InstanceNotFound(t *testing.T) {
	env := newHandlerTestEnv(t)

	params, _ := json.Marshal(map[string]string{"instance_id": "nonexistent-id", "tag": "be"})
	req := Request{ID: "req-skip-nf", Method: "workflow.skip", Project: env.project, Params: params}
	resp := env.handler.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected error for nonexistent instance, got success")
	}
	if resp.Error.Code != -32604 {
		t.Errorf("expected code -32604 (not found), got %d", resp.Error.Code)
	}
}

// TestWorkflowSkip_TagNotInGroups verifies tag not in workflow groups returns validation error.
func TestWorkflowSkip_TagNotInGroups(t *testing.T) {
	env := newHandlerTestEnv(t)
	wfiID := createWorkflowWithGroups(t, env, "wf-skip-inv", "SKIP-INV", []string{"be"})

	params, _ := json.Marshal(map[string]string{"instance_id": wfiID, "tag": "docs"})
	req := Request{ID: "req-skip-inv", Method: "workflow.skip", Project: env.project, Params: params}
	resp := env.handler.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected error for tag not in groups, got success")
	}
	if resp.Error.Code != -32606 {
		t.Errorf("expected code -32606 (validation), got %d", resp.Error.Code)
	}
}

// TestWorkflowSkip_UnknownAction verifies an unknown workflow action returns method not found.
func TestWorkflowSkip_UnknownAction(t *testing.T) {
	env := newHandlerTestEnv(t)

	req := Request{ID: "req-wf-unk", Method: "workflow.unknown", Project: env.project, Params: []byte("{}")}
	resp := env.handler.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected error for unknown workflow action, got success")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601 (method not found), got %d", resp.Error.Code)
	}
}

// TestWorkflowSkip_WSBroadcast verifies skip_tag.added event is broadcast to subscribers.
func TestWorkflowSkip_WSBroadcast(t *testing.T) {
	env := newHandlerTestEnv(t)
	wfiID := createWorkflowWithGroups(t, env, "wf-skip-ws", "SKIP-WS", []string{"be", "fe"})

	// Subscribe a WS client (Register/Subscribe are synchronous via mutex)
	wsClient, sendCh := ws.NewTestClient(env.hub, "ws-skip-client")
	env.hub.Register(wsClient)
	env.hub.Subscribe(wsClient, env.project, "SKIP-WS")

	params, _ := json.Marshal(map[string]string{"instance_id": wfiID, "tag": "fe"})
	req := Request{ID: "req-skip-ws", Method: "workflow.skip", Project: env.project, Params: params}
	resp := env.handler.Handle(req)

	if resp.Error != nil {
		t.Fatalf("workflow.skip: unexpected error: %v", resp.Error)
	}

	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventSkipTagAdded {
			t.Errorf("event type = %q, want %q", event.Type, ws.EventSkipTagAdded)
		}
		if event.Data["instance_id"] != wfiID {
			t.Errorf("event.instance_id = %v, want %q", event.Data["instance_id"], wfiID)
		}
		if event.Data["tag"] != "fe" {
			t.Errorf("event.tag = %v, want %q", event.Data["tag"], "fe")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for skip_tag.added WS event")
	}
}

// TestWorkflowSkip_Idempotent verifies adding the same tag twice via socket does not error and does not duplicate.
func TestWorkflowSkip_Idempotent(t *testing.T) {
	env := newHandlerTestEnv(t)
	wfiID := createWorkflowWithGroups(t, env, "wf-skip-idem", "SKIP-IDEM", []string{"be"})

	params, _ := json.Marshal(map[string]string{"instance_id": wfiID, "tag": "be"})

	for i := 0; i < 2; i++ {
		req := Request{ID: "req-idem", Method: "workflow.skip", Project: env.project, Params: params}
		resp := env.handler.Handle(req)
		if resp.Error != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, resp.Error)
		}
	}

	// Verify only one "be" tag in DB
	var skipTags string
	if err := env.pool.QueryRow(`SELECT COALESCE(skip_tags, '[]') FROM workflow_instances WHERE id = ?`, wfiID).Scan(&skipTags); err != nil {
		t.Fatalf("get skip_tags: %v", err)
	}
	if skipTags != `["be"]` {
		t.Errorf("idempotent skip_tags = %q, want %q", skipTags, `["be"]`)
	}
}
