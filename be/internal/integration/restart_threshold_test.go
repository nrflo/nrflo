package integration

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/types"
)

// TestAgentDefCreateWithRestartThreshold verifies that creating an agent definition
// with restart_threshold persists it correctly.
func TestAgentDefCreateWithRestartThreshold(t *testing.T) {
	env := NewTestEnv(t)

	threshold := 30
	req := &types.AgentDefCreateRequest{
		ID:               "test-agent",
		Model:            "sonnet",
		Timeout:          20,
		Prompt:           "Test prompt",
		RestartThreshold: &threshold,
	}

	svc := env.getAgentDefService(t)
	def, err := svc.CreateAgentDef(env.ProjectID, "test", req)
	if err != nil {
		t.Fatalf("failed to create agent def: %v", err)
	}

	if def.RestartThreshold == nil {
		t.Fatal("expected restart_threshold to be set")
	}
	if *def.RestartThreshold != 30 {
		t.Fatalf("expected restart_threshold 30, got %d", *def.RestartThreshold)
	}
}

// TestAgentDefCreateWithoutRestartThreshold verifies that creating an agent definition
// without restart_threshold leaves it NULL and omits it from JSON response.
func TestAgentDefCreateWithoutRestartThreshold(t *testing.T) {
	env := NewTestEnv(t)

	req := &types.AgentDefCreateRequest{
		ID:      "test-agent-2",
		Model:   "sonnet",
		Timeout: 20,
		Prompt:  "Test prompt",
	}

	svc := env.getAgentDefService(t)
	def, err := svc.CreateAgentDef(env.ProjectID, "test", req)
	if err != nil {
		t.Fatalf("failed to create agent def: %v", err)
	}

	if def.RestartThreshold != nil {
		t.Fatalf("expected restart_threshold to be nil, got %d", *def.RestartThreshold)
	}

	// Verify it's omitted from JSON via marshaling
	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if _, exists := result["restart_threshold"]; exists {
		t.Fatal("expected restart_threshold to be omitted from JSON when nil")
	}
}

// TestAgentDefGetRestartThreshold verifies that GET returns restart_threshold correctly.
func TestAgentDefGetRestartThreshold(t *testing.T) {
	env := NewTestEnv(t)

	// Create with threshold
	threshold := 35
	req := &types.AgentDefCreateRequest{
		ID:               "test-agent-3",
		Model:            "opus",
		Timeout:          25,
		Prompt:           "Test prompt",
		RestartThreshold: &threshold,
	}

	svc := env.getAgentDefService(t)
	_, err := svc.CreateAgentDef(env.ProjectID, "test", req)
	if err != nil {
		t.Fatalf("failed to create agent def: %v", err)
	}

	// Get it back
	def, err := svc.GetAgentDef(env.ProjectID, "test", "test-agent-3")
	if err != nil {
		t.Fatalf("failed to get agent def: %v", err)
	}

	if def.RestartThreshold == nil {
		t.Fatal("expected restart_threshold to be set")
	}
	if *def.RestartThreshold != 35 {
		t.Fatalf("expected restart_threshold 35, got %d", *def.RestartThreshold)
	}
}

// TestAgentDefListRestartThreshold verifies that LIST returns restart_threshold correctly.
func TestAgentDefListRestartThreshold(t *testing.T) {
	env := NewTestEnv(t)

	svc := env.getAgentDefService(t)

	// Create agent with threshold
	threshold := 40
	_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:               "agent-with-threshold",
		Prompt:           "Test",
		RestartThreshold: &threshold,
	})
	if err != nil {
		t.Fatalf("failed to create agent with threshold: %v", err)
	}

	// Create agent without threshold
	_, err = svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "agent-without-threshold",
		Prompt: "Test",
	})
	if err != nil {
		t.Fatalf("failed to create agent without threshold: %v", err)
	}

	// List
	defs, err := svc.ListAgentDefs(env.ProjectID, "test")
	if err != nil {
		t.Fatalf("failed to list agent defs: %v", err)
	}

	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}

	// Find each and verify
	var withThreshold, withoutThreshold *struct {
		ID               string
		RestartThreshold *int
	}
	for _, def := range defs {
		if def.ID == "agent-with-threshold" {
			withThreshold = &struct {
				ID               string
				RestartThreshold *int
			}{def.ID, def.RestartThreshold}
		}
		if def.ID == "agent-without-threshold" {
			withoutThreshold = &struct {
				ID               string
				RestartThreshold *int
			}{def.ID, def.RestartThreshold}
		}
	}

	if withThreshold == nil || withThreshold.RestartThreshold == nil || *withThreshold.RestartThreshold != 40 {
		t.Fatal("expected agent-with-threshold to have restart_threshold=40")
	}
	if withoutThreshold == nil || withoutThreshold.RestartThreshold != nil {
		t.Fatal("expected agent-without-threshold to have nil restart_threshold")
	}
}

// TestAgentDefUpdateRestartThreshold verifies that updating restart_threshold works.
func TestAgentDefUpdateRestartThreshold(t *testing.T) {
	env := NewTestEnv(t)

	svc := env.getAgentDefService(t)

	// Create without threshold
	_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "agent-update-test",
		Prompt: "Test",
	})
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}

	// Update to set threshold
	newThreshold := 50
	err = svc.UpdateAgentDef(env.ProjectID, "test", "agent-update-test", &types.AgentDefUpdateRequest{
		RestartThreshold: &newThreshold,
	})
	if err != nil {
		t.Fatalf("failed to update: %v", err)
	}

	// Verify
	def, err := svc.GetAgentDef(env.ProjectID, "test", "agent-update-test")
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	if def.RestartThreshold == nil || *def.RestartThreshold != 50 {
		t.Fatalf("expected restart_threshold 50, got %v", def.RestartThreshold)
	}
}

// TestAgentDefRestartThresholdBoundaryValues verifies edge cases for restart_threshold.
func TestAgentDefRestartThresholdBoundaryValues(t *testing.T) {
	env := NewTestEnv(t)

	svc := env.getAgentDefService(t)

	testCases := []struct {
		name      string
		threshold int
	}{
		{"zero", 0},
		{"one", 1},
		{"max", 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			id := "agent-" + tc.name
			th := tc.threshold
			_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
				ID:               id,
				Prompt:           "Test",
				RestartThreshold: &th,
			})
			if err != nil {
				t.Fatalf("failed to create with threshold %d: %v", tc.threshold, err)
			}

			def, err := svc.GetAgentDef(env.ProjectID, "test", id)
			if err != nil {
				t.Fatalf("failed to get: %v", err)
			}

			if def.RestartThreshold == nil || *def.RestartThreshold != tc.threshold {
				t.Fatalf("expected restart_threshold %d, got %v", tc.threshold, def.RestartThreshold)
			}
		})
	}
}

// TestActiveAgentsIncludesRestartThreshold verifies that active_agents in the
// workflow status response include the restart_threshold field when the agent
// definition has one set.
func TestActiveAgentsIncludesRestartThreshold(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "RT-1", "Restart threshold in active agents")
	env.InitWorkflow(t, "RT-1")

	wfiID := env.GetWorkflowInstanceID(t, "RT-1", "test")

	// Create agent definition with restart_threshold
	svc := env.getAgentDefService(t)
	threshold := 35
	_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:               "setup-analyzer",
		Prompt:           "Test",
		RestartThreshold: &threshold,
	})
	if err != nil {
		t.Fatalf("failed to create agent def: %v", err)
	}

	// Insert a running agent
	env.InsertAgentSession(t, "sess-rt-1", "RT-1", wfiID, "analyzer", "setup-analyzer", "claude:sonnet")

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "RT-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents to be map, got %T", status["active_agents"])
	}

	agent, ok := activeAgents["setup-analyzer:claude:sonnet"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent entry, got keys: %v", keysOf(activeAgents))
	}

	rt, ok := agent["restart_threshold"].(float64)
	if !ok {
		t.Fatalf("expected restart_threshold to be number, got %T (value: %v)", agent["restart_threshold"], agent["restart_threshold"])
	}
	if int(rt) != 35 {
		t.Fatalf("expected restart_threshold 35, got %v", rt)
	}
}

// TestActiveAgentsOmitsRestartThresholdWhenNull verifies that active_agents omit
// restart_threshold when the agent definition has NULL.
func TestActiveAgentsOmitsRestartThresholdWhenNull(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "RT-2", "No restart threshold")
	env.InitWorkflow(t, "RT-2")

	wfiID := env.GetWorkflowInstanceID(t, "RT-2", "test")

	// Create agent definition without restart_threshold
	svc := env.getAgentDefService(t)
	_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "setup-analyzer",
		Prompt: "Test",
	})
	if err != nil {
		t.Fatalf("failed to create agent def: %v", err)
	}

	// Insert a running agent
	env.InsertAgentSession(t, "sess-rt-2", "RT-2", wfiID, "analyzer", "setup-analyzer", "claude:sonnet")

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "RT-2", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents to be map, got %T", status["active_agents"])
	}

	agent, ok := activeAgents["setup-analyzer:claude:sonnet"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent entry, got keys: %v", keysOf(activeAgents))
	}

	if _, exists := agent["restart_threshold"]; exists {
		t.Fatalf("expected restart_threshold to be absent when NULL, but got %v", agent["restart_threshold"])
	}
}

// TestActiveAgentsRestartThresholdWithoutAgentDef verifies that when there is no
// matching agent definition, restart_threshold is omitted from active_agents.
func TestActiveAgentsRestartThresholdWithoutAgentDef(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "RT-3", "No agent def")
	env.InitWorkflow(t, "RT-3")

	wfiID := env.GetWorkflowInstanceID(t, "RT-3", "test")

	// Insert a running agent WITHOUT creating an agent definition
	env.InsertAgentSession(t, "sess-rt-3", "RT-3", wfiID, "analyzer", "some-agent", "claude:sonnet")

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "RT-3", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents to be map, got %T", status["active_agents"])
	}

	agent, ok := activeAgents["some-agent:claude:sonnet"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent entry, got keys: %v", keysOf(activeAgents))
	}

	if _, exists := agent["restart_threshold"]; exists {
		t.Fatalf("expected restart_threshold to be absent when no agent def, but got %v", agent["restart_threshold"])
	}
}

// getAgentDefService returns the AgentDefinitionService.
func (e *TestEnv) getAgentDefService(t *testing.T) *service.AgentDefinitionService {
	t.Helper()
	return service.NewAgentDefinitionService(e.Pool, clock.Real())
}
