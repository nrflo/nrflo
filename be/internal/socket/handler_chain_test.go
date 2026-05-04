package socket

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

// seedChainRunForHandler seeds a workflow chain + run with 2 steps and assigns
// workflow_instance_id to step 0. Returns the instance ID used for step 0.
func seedChainRunForHandler(t *testing.T, env *handlerTestEnv) string {
	t.Helper()
	clk := clock.Real()
	projectID := env.project

	chainID := "chain-handler-test"
	cr := repo.NewWorkflowChainRepo(env.pool, clk)
	if err := cr.CreateChain(&model.WorkflowChain{
		ID:          chainID,
		ProjectID:   projectID,
		Name:        "Handler Test Chain",
		Description: "test",
	}); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	sr := repo.NewWorkflowChainStepRepo(env.pool, clk)
	for i, scopeType := range []string{"project", "ticket"} {
		step := &model.WorkflowChainStep{
			ID:           "handler-step-" + string(rune('0'+i)),
			ProjectID:    projectID,
			ChainID:      chainID,
			Position:     i,
			WorkflowName: "feature",
			ScopeType:    scopeType,
		}
		if err := sr.UpsertStep(step); err != nil {
			t.Fatalf("UpsertStep %d: %v", i, err)
		}
	}

	rr := repo.NewWorkflowChainRunRepo(env.pool, clk)
	run := &model.WorkflowChainRun{
		ID:        "handler-run",
		ProjectID: projectID,
		ChainID:   chainID,
		Status:    "running",
	}
	if err := rr.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	steps, err := rr.ListRunSteps("handler-run")
	if err != nil || len(steps) == 0 {
		// Materialize via GetNextPendingStep — no steps means MaterializeRunSteps was not called.
		// Re-create by listing chain steps and materializing.
		chainSteps, _ := sr.ListSteps(chainID)
		steps, err = rr.MaterializeRunSteps("handler-run", chainSteps)
		if err != nil {
			t.Fatalf("MaterializeRunSteps: %v", err)
		}
	}

	const instanceID = "wfi-handler-0"
	if err := rr.SetRunStepInstance(steps[0].ID, instanceID, "", ""); err != nil {
		t.Fatalf("SetRunStepInstance: %v", err)
	}
	return instanceID
}

func TestHandler_AgentChainNextInstructions_HappyPath(t *testing.T) {
	env := newHandlerTestEnv(t)
	instanceID := seedChainRunForHandler(t, env)

	params, _ := json.Marshal(map[string]string{
		"instance_id":  instanceID,
		"instructions": "handoff instructions",
	})
	resp := env.handler.Handle(Request{
		ID:      "req-1",
		Method:  "agent.chain_next_instructions",
		Project: env.project,
		Params:  params,
	})

	if resp.Error != nil {
		t.Fatalf("expected success, got error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %q, want ok", result["status"])
	}

	// Verify the DB was updated
	rr := repo.NewWorkflowChainRunRepo(env.pool, clock.Real())
	steps, err := rr.ListRunSteps("handler-run")
	if err != nil {
		t.Fatalf("ListRunSteps: %v", err)
	}
	if len(steps) < 2 {
		t.Fatalf("expected at least 2 steps, got %d", len(steps))
	}
	if steps[1].InstructionsUsed != "handoff instructions" {
		t.Errorf("Steps[1].InstructionsUsed = %q, want handoff instructions", steps[1].InstructionsUsed)
	}
}

func TestHandler_AgentChainNextInstructions_MissingInstanceID(t *testing.T) {
	env := newHandlerTestEnv(t)

	params, _ := json.Marshal(map[string]string{
		"instructions": "some instructions",
	})
	resp := env.handler.Handle(Request{
		ID:      "req-2",
		Method:  "agent.chain_next_instructions",
		Project: env.project,
		Params:  params,
	})

	if resp.Error == nil {
		t.Fatal("expected validation error for missing instance_id, got nil")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ErrCodeValidation)
	}
}

func TestHandler_AgentChainNextInstructions_MissingInstructions(t *testing.T) {
	env := newHandlerTestEnv(t)

	params, _ := json.Marshal(map[string]string{
		"instance_id": "some-instance",
	})
	resp := env.handler.Handle(Request{
		ID:      "req-3",
		Method:  "agent.chain_next_instructions",
		Project: env.project,
		Params:  params,
	})

	if resp.Error == nil {
		t.Fatal("expected validation error for missing instructions, got nil")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ErrCodeValidation)
	}
}

func TestHandler_AgentChainNextInstructions_UnknownInstance(t *testing.T) {
	env := newHandlerTestEnv(t)

	params, _ := json.Marshal(map[string]string{
		"instance_id":  "no-such-instance",
		"instructions": "some instructions",
	})
	resp := env.handler.Handle(Request{
		ID:      "req-4",
		Method:  "agent.chain_next_instructions",
		Project: env.project,
		Params:  params,
	})

	if resp.Error == nil {
		t.Fatal("expected internal error for unknown instance, got nil")
	}
	if resp.Error.Code != ErrCodeInternal {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ErrCodeInternal)
	}
}

func TestHandler_AgentChainNextTicket_HappyPath(t *testing.T) {
	env := newHandlerTestEnv(t)
	instanceID := seedChainRunForHandler(t, env)

	params, _ := json.Marshal(map[string]string{
		"instance_id": instanceID,
		"ticket_id":   "TICKET-42",
	})
	resp := env.handler.Handle(Request{
		ID:      "req-5",
		Method:  "agent.chain_next_ticket",
		Project: env.project,
		Params:  params,
	})

	if resp.Error != nil {
		t.Fatalf("expected success, got error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %q, want ok", result["status"])
	}

	rr := repo.NewWorkflowChainRunRepo(env.pool, clock.Real())
	steps, err := rr.ListRunSteps("handler-run")
	if err != nil {
		t.Fatalf("ListRunSteps: %v", err)
	}
	if len(steps) < 2 {
		t.Fatalf("expected at least 2 steps, got %d", len(steps))
	}
	if !steps[1].TicketID.Valid || steps[1].TicketID.String != "TICKET-42" {
		t.Errorf("Steps[1].TicketID = %v, want TICKET-42", steps[1].TicketID)
	}
}

func TestHandler_AgentChainNextTicket_MissingInstanceID(t *testing.T) {
	env := newHandlerTestEnv(t)

	params, _ := json.Marshal(map[string]string{"ticket_id": "TICKET-1"})
	resp := env.handler.Handle(Request{
		ID:      "req-6",
		Method:  "agent.chain_next_ticket",
		Project: env.project,
		Params:  params,
	})

	if resp.Error == nil {
		t.Fatal("expected validation error for missing instance_id, got nil")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ErrCodeValidation)
	}
}

func TestHandler_AgentChainNextTicket_MissingTicketID(t *testing.T) {
	env := newHandlerTestEnv(t)

	params, _ := json.Marshal(map[string]string{"instance_id": "some-instance"})
	resp := env.handler.Handle(Request{
		ID:      "req-7",
		Method:  "agent.chain_next_ticket",
		Project: env.project,
		Params:  params,
	})

	if resp.Error == nil {
		t.Fatal("expected validation error for missing ticket_id, got nil")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ErrCodeValidation)
	}
}

func TestHandler_AgentChainNextTicket_UnknownInstance(t *testing.T) {
	env := newHandlerTestEnv(t)

	params, _ := json.Marshal(map[string]string{
		"instance_id": "no-such-instance",
		"ticket_id":   "TICKET-1",
	})
	resp := env.handler.Handle(Request{
		ID:      "req-8",
		Method:  "agent.chain_next_ticket",
		Project: env.project,
		Params:  params,
	})

	if resp.Error == nil {
		t.Fatal("expected internal error for unknown instance, got nil")
	}
	if resp.Error.Code != ErrCodeInternal {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ErrCodeInternal)
	}
}
