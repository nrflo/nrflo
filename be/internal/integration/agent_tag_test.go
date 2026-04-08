package integration

import (
	"testing"

	"be/internal/types"
)

// TestActiveAgentsIncludesTag verifies that active_agents in the workflow status
// response includes the tag field when the agent definition has a tag set.
func TestActiveAgentsIncludesTag(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AT-1", "Tag in active agents")
	env.InitWorkflow(t, "AT-1")
	wfiID := env.GetWorkflowInstanceID(t, "AT-1", "test")

	// Add "be-team" to workflow groups so the tag is valid
	groups := []string{"be-team"}
	if err := env.WorkflowSvc.UpdateWorkflowDef(env.ProjectID, "test", &types.WorkflowDefUpdateRequest{
		Groups: &groups,
	}); err != nil {
		t.Fatalf("failed to update workflow groups: %v", err)
	}

	svc := env.getAgentDefService(t)
	tag := "be-team"
	err := svc.UpdateAgentDef(env.ProjectID, "test", "analyzer", &types.AgentDefUpdateRequest{
		Tag: &tag,
	})
	if err != nil {
		t.Fatalf("failed to update agent def: %v", err)
	}

	env.InsertAgentSession(t, "sess-at-1", "AT-1", wfiID, "analyzer", "analyzer", "claude:sonnet")

	status, err := getWorkflowStatus(t, env, "AT-1", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents map, got %T", status["active_agents"])
	}

	agent, ok := activeAgents["analyzer:claude:sonnet"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent entry, got keys: %v", keysOf(activeAgents))
	}

	tagVal, ok := agent["tag"].(string)
	if !ok {
		t.Fatalf("expected tag to be string, got %T (value: %v)", agent["tag"], agent["tag"])
	}
	if tagVal != "be-team" {
		t.Fatalf("expected tag %q, got %q", "be-team", tag)
	}
}

// TestActiveAgentsOmitsTagWhenEmpty verifies that active_agents omits the tag
// field when the agent definition has no tag set.
func TestActiveAgentsOmitsTagWhenEmpty(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AT-2", "No tag in active agents")
	env.InitWorkflow(t, "AT-2")
	wfiID := env.GetWorkflowInstanceID(t, "AT-2", "test")

	// "analyzer" already exists from testenv seeding — no update needed (tag is empty by default)

	env.InsertAgentSession(t, "sess-at-2", "AT-2", wfiID, "analyzer", "analyzer", "claude:sonnet")

	status, err := getWorkflowStatus(t, env, "AT-2", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents map, got %T", status["active_agents"])
	}

	agent, ok := activeAgents["analyzer:claude:sonnet"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent entry, got keys: %v", keysOf(activeAgents))
	}

	if _, exists := agent["tag"]; exists {
		t.Fatalf("expected tag to be absent when empty, but got %v", agent["tag"])
	}
}

// TestActiveAgentsOmitsTagWithNoAgentDef verifies that active_agents omits the
// tag field when no matching agent definition exists (LEFT JOIN produces NULL).
func TestActiveAgentsOmitsTagWithNoAgentDef(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AT-3", "No agent def")
	env.InitWorkflow(t, "AT-3")
	wfiID := env.GetWorkflowInstanceID(t, "AT-3", "test")

	// Insert session with no matching agent definition
	env.InsertAgentSession(t, "sess-at-3", "AT-3", wfiID, "analyzer", "unknown-agent", "claude:sonnet")

	status, err := getWorkflowStatus(t, env, "AT-3", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents map, got %T", status["active_agents"])
	}

	agent, ok := activeAgents["unknown-agent:claude:sonnet"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent entry, got keys: %v", keysOf(activeAgents))
	}

	if _, exists := agent["tag"]; exists {
		t.Fatalf("expected tag to be absent with no agent def, but got %v", agent["tag"])
	}
}

// TestAgentHistoryIncludesTag verifies that agent_history entries in the workflow
// status response include the tag field when the agent definition has a tag set.
func TestAgentHistoryIncludesTag(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AT-4", "Tag in agent history")
	env.InitWorkflow(t, "AT-4")
	wfiID := env.GetWorkflowInstanceID(t, "AT-4", "test")

	// Add "fe-team" to workflow groups so the tag is valid
	groups := []string{"fe-team"}
	if err := env.WorkflowSvc.UpdateWorkflowDef(env.ProjectID, "test", &types.WorkflowDefUpdateRequest{
		Groups: &groups,
	}); err != nil {
		t.Fatalf("failed to update workflow groups: %v", err)
	}

	svc := env.getAgentDefService(t)
	tag := "fe-team"
	err := svc.UpdateAgentDef(env.ProjectID, "test", "analyzer", &types.AgentDefUpdateRequest{
		Tag: &tag,
	})
	if err != nil {
		t.Fatalf("failed to update agent def: %v", err)
	}

	insertCompletedSession(t, env, "sess-at-4", "AT-4", wfiID, "analyzer", "analyzer", "claude:sonnet", "completed", "pass")

	status, err := getWorkflowStatus(t, env, "AT-4", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry, ok := history[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected history entry map, got %T", history[0])
	}

	tagVal, ok := entry["tag"].(string)
	if !ok {
		t.Fatalf("expected tag to be string, got %T (value: %v)", entry["tag"], entry["tag"])
	}
	if tagVal != "fe-team" {
		t.Fatalf("expected tag %q, got %q", "fe-team", tag)
	}
}

// TestAgentHistoryOmitsTagWhenEmpty verifies that agent_history entries omit the
// tag field when the agent definition has no tag set.
func TestAgentHistoryOmitsTagWhenEmpty(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AT-5", "No tag in history")
	env.InitWorkflow(t, "AT-5")
	wfiID := env.GetWorkflowInstanceID(t, "AT-5", "test")

	// "analyzer" already exists from testenv seeding — tag is empty by default

	insertCompletedSession(t, env, "sess-at-5", "AT-5", wfiID, "analyzer", "analyzer", "claude:sonnet", "completed", "pass")

	status, err := getWorkflowStatus(t, env, "AT-5", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry, ok := history[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected history entry map, got %T", history[0])
	}

	if _, exists := entry["tag"]; exists {
		t.Fatalf("expected tag to be absent when empty, but got %v", entry["tag"])
	}
}

// TestAgentHistoryOmitsTagWithNoAgentDef verifies that agent_history entries omit
// the tag field when no matching agent definition exists.
func TestAgentHistoryOmitsTagWithNoAgentDef(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AT-6", "No agent def in history")
	env.InitWorkflow(t, "AT-6")
	wfiID := env.GetWorkflowInstanceID(t, "AT-6", "test")

	// No agent def — LEFT JOIN produces NULL tag
	insertCompletedSession(t, env, "sess-at-6", "AT-6", wfiID, "analyzer", "unknown-agent", "claude:sonnet", "completed", "pass")

	status, err := getWorkflowStatus(t, env, "AT-6", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry, ok := history[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected history entry map, got %T", history[0])
	}

	if _, exists := entry["tag"]; exists {
		t.Fatalf("expected tag to be absent with no agent def, but got %v", entry["tag"])
	}
}
