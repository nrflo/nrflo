package orchestrator

import (
	"context"
	"database/sql"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
)

// TestDecomposeCallback_ResetScopeUsesNodeID verifies decomposeCallback's
// resetScope is built from p.NodeID, not p.Agent — the field that matters
// once a node's execution identity diverges from its template identity
// (fan-out: same Agent, distinct NodeID).
func TestDecomposeCallback_ResetScopeUsesNodeID(t *testing.T) {
	groups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{NodeID: "analyzer#1", Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{NodeID: "builder#1", Agent: "builder", Layer: 1}}},
	}
	req := &spawner.CallbackError{Level: 1, Instructions: "fix it"}
	d := decomposeCallback(req, 1, groups)

	for _, nodeID := range d.resetScope {
		if nodeID == "analyzer" || nodeID == "builder" {
			t.Errorf("resetScope contains bare Agent name %q, want NodeID (e.g. %q)", nodeID, nodeID+"#1")
		}
	}
	found := false
	for _, nodeID := range d.resetScope {
		if nodeID == "builder#1" {
			found = true
		}
	}
	if !found {
		t.Errorf("resetScope = %v, want it to contain %q", d.resetScope, "builder#1")
	}
}

// TestResetCallbackSessions_FlipsOnlyMatchingNodeID verifies
// resetCallbackSessions (the orchestrator's wrapper around
// ResetAgentSessionsInWorkflow) flips only the session whose node_id is in
// scope to status=callback, leaving a session with the same Phase/AgentType
// but a different NodeID untouched.
func TestResetCallbackSessions_FlipsOnlyMatchingNodeID(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "NID-1", "Node identity reset test")
	wfiID := env.initWorkflow(t, "NID-1")

	asRepo := repo.NewAgentSessionRepo(env.pool, clock.Real())
	inScope := &model.AgentSession{
		ID:                 "sess-in-scope",
		ProjectID:          env.project,
		TicketID:           "NID-1",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		NodeID:             "analyzer#1",
		AgentType:          "analyzer",
		Status:             model.AgentSessionCompleted,
		Result:             sql.NullString{String: "pass", Valid: true},
	}
	outOfScope := &model.AgentSession{
		ID:                 "sess-out-of-scope",
		ProjectID:          env.project,
		TicketID:           "NID-1",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		NodeID:             "analyzer#2",
		AgentType:          "analyzer",
		Status:             model.AgentSessionCompleted,
		Result:             sql.NullString{String: "pass", Valid: true},
	}
	if err := asRepo.Create(inScope); err != nil {
		t.Fatalf("create in-scope session: %v", err)
	}
	if err := asRepo.Create(outOfScope); err != nil {
		t.Fatalf("create out-of-scope session: %v", err)
	}

	resetCallbackSessions(context.Background(), asRepo, wfiID, []string{"analyzer#1"})

	got, err := asRepo.Get("sess-in-scope")
	if err != nil {
		t.Fatalf("Get in-scope: %v", err)
	}
	if got.Status != model.AgentSessionCallback {
		t.Errorf("in-scope session status = %q, want callback", got.Status)
	}

	untouched, err := asRepo.Get("sess-out-of-scope")
	if err != nil {
		t.Fatalf("Get out-of-scope: %v", err)
	}
	if untouched.Status != model.AgentSessionCompleted {
		t.Errorf("out-of-scope session status = %q, want unchanged completed (same Phase/AgentType, different NodeID)", untouched.Status)
	}
}

// TestRetryFailedAgent_ResolvesByNodeIDNotPhase verifies retryFailed resolves
// the restart layer from session.NodeID even when session.Phase holds a
// stale/unrelated value — proving the layer lookup is genuinely keyed on
// NodeID and not on Phase.
func TestRetryFailedAgent_ResolvesByNodeIDNotPhase(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "NID-2", "Retry by NodeID test")
	wfiID := env.initWorkflow(t, "NID-2")

	asRepo := repo.NewAgentSessionRepo(env.pool, clock.Real())
	session := &model.AgentSession{
		ID:                 "sess-nid-retry",
		ProjectID:          env.project,
		TicketID:           "NID-2",
		WorkflowInstanceID: wfiID,
		Phase:              "not-a-real-phase-label",
		NodeID:             "builder",
		AgentType:          "builder",
		Status:             model.AgentSessionFailed,
		Result:             sql.NullString{String: "fail", Valid: true},
	}
	if err := asRepo.Create(session); err != nil {
		t.Fatalf("create failed session: %v", err)
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
	if err := wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceFailed); err != nil {
		t.Fatalf("mark workflow failed: %v", err)
	}

	if err := env.orch.RetryFailedAgent(context.Background(), env.project, "NID-2", "test", "sess-nid-retry"); err != nil {
		t.Fatalf("RetryFailedAgent: %v (Phase held a bogus value; resolution must use NodeID)", err)
	}
	env.stopAndWaitRun(t, wfiID)
}

// TestCreateSkippedSessions_CarriesNodeID verifies createSkippedSessions
// writes node_id (not just phase/agent_type) on the resulting skipped rows,
// using a NodeID distinct from Agent to prove it isn't just echoing the
// template name.
func TestCreateSkippedSessions_CarriesNodeID(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "NID-3", "Skipped session node_id test")
	wfiID := env.initWorkflow(t, "NID-3")

	phases := []service.SpawnerPhaseDef{
		{NodeID: "fe-impl#1", Agent: "fe-impl", Layer: 1},
	}
	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "NID-3",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}

	env.orch.createSkippedSessions(context.Background(), wfiID, req, phases, env.pool)

	var nodeID, agentType string
	if err := env.pool.QueryRow(
		`SELECT node_id, agent_type FROM agent_sessions WHERE workflow_instance_id = ?`, wfiID,
	).Scan(&nodeID, &agentType); err != nil {
		t.Fatalf("query skipped session: %v", err)
	}
	if nodeID != "fe-impl#1" {
		t.Errorf("node_id = %q, want fe-impl#1", nodeID)
	}
	if agentType != "fe-impl" {
		t.Errorf("agent_type = %q, want fe-impl", agentType)
	}
}
