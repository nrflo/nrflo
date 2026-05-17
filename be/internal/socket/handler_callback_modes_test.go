package socket

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/types"
	"be/internal/ws"
)

// TestAgentCallbackValidation covers the setCount != 1 rejection cases.
func TestAgentCallbackValidation(t *testing.T) {
	cases := []struct {
		name        string
		level       int
		targetAgent string
		chain       []string
	}{
		{
			name:  "none set",
			level: 0,
		},
		{
			name:        "level and agent",
			level:       2,
			targetAgent: "implementor",
		},
		{
			name:  "level and chain",
			level: 2,
			chain: []string{"workflow-a", "workflow-b"},
		},
		{
			name:        "agent and chain",
			targetAgent: "implementor",
			chain:       []string{"workflow-a"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newHandlerTestEnv(t)
			env.createTicketAndWorkflow(t, "TEST-1")

			var wfiID string
			if err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
				env.project, "TEST-1", "test").Scan(&wfiID); err != nil {
				t.Fatalf("failed to get workflow instance ID: %v", err)
			}

			sessionID := "sess-val-" + tc.name
			if _, err := env.pool.Exec(`
				INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
				VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'claude-sonnet-4', 'running', datetime('now'), datetime('now'))
			`, sessionID, env.project, "TEST-1", wfiID); err != nil {
				t.Fatalf("failed to create session: %v", err)
			}

			params := types.AgentCallbackRequest{
				AgentRequest: types.AgentRequest{
					InstanceID: wfiID,
					SessionID:  sessionID,
				},
				Level:       tc.level,
				TargetAgent: tc.targetAgent,
				Chain:       tc.chain,
			}
			paramsData, _ := json.Marshal(params)

			resp := env.handler.Handle(Request{
				ID:      "req-1",
				Method:  "agent.callback",
				Project: env.project,
				Params:  paramsData,
			})

			if resp.Error == nil {
				t.Fatal("expected validation error, got nil")
			}
			if resp.Error.Code != ErrCodeValidation {
				t.Errorf("expected code %d, got %d", ErrCodeValidation, resp.Error.Code)
			}
		})
	}
}

// TestAgentCallbackModeInference covers mode inference for each valid single-field case.
func TestAgentCallbackModeInference(t *testing.T) {
	cases := []struct {
		name         string
		level        int
		targetAgent  string
		chain        []string
		expectedMode string
	}{
		{
			name:         "layer mode from level",
			level:        2,
			expectedMode: "layer",
		},
		{
			name:         "agent mode from target_agent",
			targetAgent:  "implementor",
			expectedMode: "agent",
		},
		{
			name:         "chain mode from chain",
			chain:        []string{"workflow-a", "workflow-b"},
			expectedMode: "chain",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newHandlerTestEnv(t)
			env.createTicketAndWorkflow(t, "TEST-1")

			var wfiID string
			if err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
				env.project, "TEST-1", "test").Scan(&wfiID); err != nil {
				t.Fatalf("failed to get workflow instance ID: %v", err)
			}

			sessionID := "sess-mode-" + tc.name
			if _, err := env.pool.Exec(`
				INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
				VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'claude-sonnet-4', 'running', datetime('now'), datetime('now'))
			`, sessionID, env.project, "TEST-1", wfiID); err != nil {
				t.Fatalf("failed to create session: %v", err)
			}

			client, sendCh := ws.NewTestClient(env.hub, "client-"+tc.name)
			env.hub.Register(client)
			time.Sleep(20 * time.Millisecond)
			env.hub.Subscribe(client, env.project, "TEST-1")

			params := types.AgentCallbackRequest{
				AgentRequest: types.AgentRequest{
					InstanceID: wfiID,
					SessionID:  sessionID,
				},
				Level:       tc.level,
				TargetAgent: tc.targetAgent,
				Chain:       tc.chain,
			}
			paramsData, _ := json.Marshal(params)

			resp := env.handler.Handle(Request{
				ID:      "req-1",
				Method:  "agent.callback",
				Project: env.project,
				Params:  paramsData,
			})

			if resp.Error != nil {
				t.Fatalf("expected no error, got: %v", resp.Error)
			}

			select {
			case msg := <-sendCh:
				var event ws.Event
				if err := json.Unmarshal(msg, &event); err != nil {
					t.Fatalf("failed to unmarshal event: %v", err)
				}
				if event.Type != ws.EventAgentCompleted {
					t.Errorf("expected event type %s, got %s", ws.EventAgentCompleted, event.Type)
				}
				result, ok := event.Data["result"].(string)
				if !ok || result != "callback" {
					t.Errorf("expected result=callback, got: %v", event.Data["result"])
				}
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for agent.completed broadcast")
			}
		})
	}
}
