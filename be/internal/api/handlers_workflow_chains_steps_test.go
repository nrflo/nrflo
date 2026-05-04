package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"be/internal/ws"
)

// -- Update chain --

func TestHandleUpdateWorkflowChain_MissingProject(t *testing.T) {
	s := newWorkflowChainServer(t)
	rr := doChainReq(t, s, s.handleUpdateWorkflowChain, http.MethodPatch, "/api/v1/workflow-chains/x",
		"", `{"name":"Y"}`, map[string]string{"id": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleUpdateWorkflowChain_NotFound(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wcu-nf", "", "")
	rr := doChainReq(t, s, s.handleUpdateWorkflowChain, http.MethodPatch, "/api/v1/workflow-chains/no-chain",
		"proj-wcu-nf", `{"name":"Y"}`, map[string]string{"id": "no-chain"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleUpdateWorkflowChain_Valid(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wcu-ok", "wf-wcu", "project")
	doCreateChain(t, s, "proj-wcu-ok", "chain-wcu", "step-wcu", "wf-wcu")
	rr := doChainReq(t, s, s.handleUpdateWorkflowChain, http.MethodPatch, "/api/v1/workflow-chains/chain-wcu",
		"proj-wcu-ok", `{"name":"Renamed","description":"desc updated"}`, map[string]string{"id": "chain-wcu"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	chain := decodeChainResp(t, rr)
	if chain.Name != "Renamed" {
		t.Errorf("Name = %q, want 'Renamed'", chain.Name)
	}
}

// -- Delete chain --

func TestHandleDeleteWorkflowChain_MissingProject(t *testing.T) {
	s := newWorkflowChainServer(t)
	rr := doChainReq(t, s, s.handleDeleteWorkflowChain, http.MethodDelete, "/api/v1/workflow-chains/x",
		"", "", map[string]string{"id": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleDeleteWorkflowChain_NotFound(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wcd-nf", "", "")
	rr := doChainReq(t, s, s.handleDeleteWorkflowChain, http.MethodDelete, "/api/v1/workflow-chains/no-chain",
		"proj-wcd-nf", "", map[string]string{"id": "no-chain"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleDeleteWorkflowChain_Valid(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wcd-ok", "wf-wcd", "project")
	doCreateChain(t, s, "proj-wcd-ok", "chain-wcd", "step-wcd", "wf-wcd")
	rr := doChainReq(t, s, s.handleDeleteWorkflowChain, http.MethodDelete, "/api/v1/workflow-chains/chain-wcd",
		"proj-wcd-ok", "", map[string]string{"id": "chain-wcd"})
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204; body: %s", rr.Code, rr.Body.String())
	}
	// Verify chain no longer retrievable.
	rr2 := doChainReq(t, s, s.handleGetWorkflowChain, http.MethodGet, "/api/v1/workflow-chains/chain-wcd",
		"proj-wcd-ok", "", map[string]string{"id": "chain-wcd"})
	if rr2.Code != http.StatusNotFound {
		t.Errorf("after delete: status = %d, want 404", rr2.Code)
	}
}

// -- AppendStep --

func TestHandleAppendChainStep_MissingProject(t *testing.T) {
	s := newWorkflowChainServer(t)
	rr := doChainReq(t, s, s.handleAppendChainStep, http.MethodPost, "/api/v1/workflow-chains/x/steps",
		"", `{"workflow_name":"w","scope_type":"ticket"}`, map[string]string{"id": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleAppendChainStep_ChainNotFound(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wca-nf", "", "")
	rr := doChainReq(t, s, s.handleAppendChainStep, http.MethodPost, "/api/v1/workflow-chains/no-chain/steps",
		"proj-wca-nf", `{"workflow_name":"w","scope_type":"ticket"}`, map[string]string{"id": "no-chain"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleAppendChainStep_Valid(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wca-ok", "wf-proj-a", "project")
	seedChainProject(t, s, "proj-wca-ok", "wf-tick-a", "ticket")
	doCreateChain(t, s, "proj-wca-ok", "chain-app", "step-app0", "wf-proj-a")
	body := `{"id":"step-app1","workflow_name":"wf-tick-a","scope_type":"ticket","require_ticket_handoff":true}`
	rr := doChainReq(t, s, s.handleAppendChainStep, http.MethodPost, "/api/v1/workflow-chains/chain-app/steps",
		"proj-wca-ok", body, map[string]string{"id": "chain-app"})
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	chain := decodeChainResp(t, rr)
	if len(chain.Steps) != 2 {
		t.Errorf("Steps len = %d, want 2", len(chain.Steps))
	}
	if chain.Steps[1].Position != 1 {
		t.Errorf("Step[1].Position = %d, want 1", chain.Steps[1].Position)
	}
}

// -- UpdateStep --

func TestHandleUpdateChainStep_StepNotFound(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wcs-nf", "wf-wcs-nf", "project")
	doCreateChain(t, s, "proj-wcs-nf", "chain-wcs-nf", "step-wcs-nf-0", "wf-wcs-nf")
	rr := doChainReq(t, s, s.handleUpdateChainStep, http.MethodPatch, "/api/v1/workflow-chains/chain-wcs-nf/steps/no-step",
		"proj-wcs-nf", `{"base_instructions":"hi"}`,
		map[string]string{"id": "chain-wcs-nf", "stepId": "no-step"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleUpdateChainStep_Valid(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wcs-ok", "wf-wcs-ok", "project")
	doCreateChain(t, s, "proj-wcs-ok", "chain-wcs-ok", "step-wcs-ok0", "wf-wcs-ok")
	body := `{"base_instructions":"updated instructions"}`
	rr := doChainReq(t, s, s.handleUpdateChainStep, http.MethodPatch,
		"/api/v1/workflow-chains/chain-wcs-ok/steps/step-wcs-ok0",
		"proj-wcs-ok", body, map[string]string{"id": "chain-wcs-ok", "stepId": "step-wcs-ok0"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
}

// -- DeleteStep --

func TestHandleDeleteChainStep_ChainNotFound(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wcds-cf", "", "")
	rr := doChainReq(t, s, s.handleDeleteChainStep, http.MethodDelete,
		"/api/v1/workflow-chains/no-chain/steps/s1",
		"proj-wcds-cf", "", map[string]string{"id": "no-chain", "stepId": "s1"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleDeleteChainStep_StepNotFound(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wcds-nf", "wf-wcds-nf", "project")
	doCreateChain(t, s, "proj-wcds-nf", "chain-wcds-nf", "step-wcds-nf0", "wf-wcds-nf")
	rr := doChainReq(t, s, s.handleDeleteChainStep, http.MethodDelete,
		"/api/v1/workflow-chains/chain-wcds-nf/steps/no-step",
		"proj-wcds-nf", "", map[string]string{"id": "chain-wcds-nf", "stepId": "no-step"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleDeleteChainStep_Valid(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wcds-ok", "wf-proj-ds", "project")
	seedChainProject(t, s, "proj-wcds-ok", "wf-tick-ds", "ticket")
	// Create chain with 2 steps.
	body := `{"id":"chain-ds","name":"DS","steps":[
		{"id":"step-ds0","workflow_name":"wf-proj-ds","scope_type":"project"},
		{"id":"step-ds1","workflow_name":"wf-tick-ds","scope_type":"ticket"}]}`
	req2 := doChainReq(t, s, s.handleCreateWorkflowChain, http.MethodPost, "/api/v1/workflow-chains",
		"proj-wcds-ok", body, nil)
	if req2.Code != http.StatusCreated {
		t.Fatalf("create chain: status=%d body=%s", req2.Code, req2.Body.String())
	}
	// Delete the ticket step.
	rr := doChainReq(t, s, s.handleDeleteChainStep, http.MethodDelete,
		"/api/v1/workflow-chains/chain-ds/steps/step-ds1",
		"proj-wcds-ok", "", map[string]string{"id": "chain-ds", "stepId": "step-ds1"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	chain := decodeChainResp(t, rr)
	if len(chain.Steps) != 1 {
		t.Errorf("remaining steps = %d, want 1", len(chain.Steps))
	}
}

// -- ReorderSteps --

func TestHandleReorderChainSteps_WrongCount(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wcr-wc", "wf-wcr-wc", "project")
	doCreateChain(t, s, "proj-wcr-wc", "chain-wcr-wc", "step-wcr-wc0", "wf-wcr-wc")
	// Send 2 IDs but chain only has 1 step.
	rr := doChainReq(t, s, s.handleReorderChainSteps, http.MethodPost,
		"/api/v1/workflow-chains/chain-wcr-wc/steps/reorder",
		"proj-wcr-wc", `{"ordered_step_ids":["step-wcr-wc0","extra-id"]}`,
		map[string]string{"id": "chain-wcr-wc"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleReorderChainSteps_Valid(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wcr-ok", "wf-proj-r", "project")
	seedChainProject(t, s, "proj-wcr-ok", "wf-tick-r", "ticket")
	body := `{"id":"chain-r","name":"R","steps":[
		{"id":"step-r0","workflow_name":"wf-proj-r","scope_type":"project"},
		{"id":"step-r1","workflow_name":"wf-tick-r","scope_type":"ticket"}]}`
	rCreate := doChainReq(t, s, s.handleCreateWorkflowChain, http.MethodPost, "/api/v1/workflow-chains",
		"proj-wcr-ok", body, nil)
	if rCreate.Code != http.StatusCreated {
		t.Fatalf("create chain: %d", rCreate.Code)
	}
	// Reorder: swap step-r1 to first, step-r0 to second.
	// NOTE: step-r0 must be project-scope to stay valid at position 0.
	// We keep the same order so validation passes (position 0 stays project-scope).
	rr := doChainReq(t, s, s.handleReorderChainSteps, http.MethodPost,
		"/api/v1/workflow-chains/chain-r/steps/reorder",
		"proj-wcr-ok", `{"ordered_step_ids":["step-r0","step-r1"]}`,
		map[string]string{"id": "chain-r"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	chain := decodeChainResp(t, rr)
	if len(chain.Steps) != 2 {
		t.Errorf("steps len = %d, want 2", len(chain.Steps))
	}
	if chain.Steps[0].Position != 0 || chain.Steps[1].Position != 1 {
		t.Errorf("positions = %d,%d, want 0,1", chain.Steps[0].Position, chain.Steps[1].Position)
	}
}

// -- WS events --

func TestHandleWorkflowChain_WSEvents(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-ws", "wf-wc-ws", "project")

	client, ch := ws.NewTestClient(s.wsHub, "wc-ws-client")
	s.wsHub.Subscribe(client, "proj-wc-ws", "")

	// Create → chain_def.created.
	doCreateChain(t, s, "proj-wc-ws", "chain-wc-ws", "step-wc-ws0", "wf-wc-ws")
	if !drainForEvent(ch, ws.EventWorkflowChainCreated, 500*time.Millisecond) {
		t.Error("did not receive chain_def.created WS event")
	}

	// Update → chain_def.updated.
	doChainReq(t, s, s.handleUpdateWorkflowChain, http.MethodPatch, "/api/v1/workflow-chains/chain-wc-ws",
		"proj-wc-ws", `{"name":"Upd"}`, map[string]string{"id": "chain-wc-ws"})
	if !drainForEvent(ch, ws.EventWorkflowChainUpdated, 500*time.Millisecond) {
		t.Error("did not receive chain_def.updated WS event on chain update")
	}

	// Delete → chain_def.deleted.
	doChainReq(t, s, s.handleDeleteWorkflowChain, http.MethodDelete, "/api/v1/workflow-chains/chain-wc-ws",
		"proj-wc-ws", "", map[string]string{"id": "chain-wc-ws"})
	if !drainForEvent(ch, ws.EventWorkflowChainDeleted, 500*time.Millisecond) {
		t.Error("did not receive chain_def.deleted WS event")
	}
}

func TestHandleWorkflowChain_StepMutationsEmitUpdated(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wcs-ev", "wf-proj-ev", "project")
	seedChainProject(t, s, "proj-wcs-ev", "wf-tick-ev", "ticket")
	doCreateChain(t, s, "proj-wcs-ev", "chain-ev", "step-ev0", "wf-proj-ev")

	client, ch := ws.NewTestClient(s.wsHub, "wcs-ev-client")
	s.wsHub.Subscribe(client, "proj-wcs-ev", "")

	// Append step → chain_def.updated.
	body := `{"id":"step-ev1","workflow_name":"wf-tick-ev","scope_type":"ticket","require_ticket_handoff":true}`
	doChainReq(t, s, s.handleAppendChainStep, http.MethodPost, "/api/v1/workflow-chains/chain-ev/steps",
		"proj-wcs-ev", body, map[string]string{"id": "chain-ev"})
	if !drainForEvent(ch, ws.EventWorkflowChainUpdated, 500*time.Millisecond) {
		t.Error("did not receive chain_def.updated WS event on append step")
	}

	// Delete step → chain_def.updated.
	doChainReq(t, s, s.handleDeleteChainStep, http.MethodDelete,
		"/api/v1/workflow-chains/chain-ev/steps/step-ev1",
		"proj-wcs-ev", "", map[string]string{"id": "chain-ev", "stepId": "step-ev1"})
	if !drainForEvent(ch, ws.EventWorkflowChainUpdated, 500*time.Millisecond) {
		t.Error("did not receive chain_def.updated WS event on delete step")
	}
}

func TestHandleWorkflowChain_MultiProjectIsolation(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-iso-a", "wf-iso-a", "project")
	seedChainProject(t, s, "proj-iso-b", "wf-iso-b", "project")
	doCreateChain(t, s, "proj-iso-a", "chain-iso-a", "step-iso-a0", "wf-iso-a")
	doCreateChain(t, s, "proj-iso-b", "chain-iso-b", "step-iso-b0", "wf-iso-b")

	// Project A should only see its own chain.
	rr := doChainReq(t, s, s.handleListWorkflowChains, http.MethodGet, "/api/v1/workflow-chains",
		"proj-iso-a", "", nil)
	var list []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("proj-iso-a list len = %d, want 1", len(list))
	}
	if list[0]["id"] != "chain-iso-a" {
		t.Errorf("proj-iso-a chain id = %v, want chain-iso-a", list[0]["id"])
	}

	// chain-iso-b is not visible from project A.
	rr2 := doChainReq(t, s, s.handleGetWorkflowChain, http.MethodGet, "/api/v1/workflow-chains/chain-iso-b",
		"proj-iso-a", "", map[string]string{"id": "chain-iso-b"})
	if rr2.Code != http.StatusNotFound {
		t.Errorf("cross-project get status = %d, want 404", rr2.Code)
	}

	// Deleting from wrong project returns 404.
	rr3 := doChainReq(t, s, s.handleDeleteWorkflowChain, http.MethodDelete, "/api/v1/workflow-chains/chain-iso-b",
		"proj-iso-a", "", map[string]string{"id": "chain-iso-b"})
	if rr3.Code != http.StatusNotFound {
		t.Errorf("cross-project delete status = %d, want 404", rr3.Code)
	}
}

func TestHandleCreateWorkflowChain_MultiStep_PositionsDense(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-ms", "wf-ms-p", "project")
	seedChainProject(t, s, "proj-wc-ms", "wf-ms-t", "ticket")
	body := `{"id":"chain-ms","name":"Multi","steps":[
		{"id":"step-ms0","workflow_name":"wf-ms-p","scope_type":"project"},
		{"id":"step-ms1","workflow_name":"wf-ms-t","scope_type":"ticket","require_ticket_handoff":true},
		{"id":"step-ms2","workflow_name":"wf-ms-t","scope_type":"ticket"}]}`
	rr := doChainReq(t, s, s.handleCreateWorkflowChain, http.MethodPost, "/api/v1/workflow-chains",
		"proj-wc-ms", body, nil)
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	chain := decodeChainResp(t, rr)
	if len(chain.Steps) != 3 {
		t.Fatalf("steps len = %d, want 3", len(chain.Steps))
	}
	for i, step := range chain.Steps {
		if step.Position != i {
			t.Errorf("step[%d].Position = %d, want %d", i, step.Position, i)
		}
	}
}

