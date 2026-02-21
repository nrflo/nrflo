package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// TestBuildAgentTags verifies buildAgentTags builds a correct agent_id → tag map,
// excluding agents with empty tags.
func TestBuildAgentTags(t *testing.T) {
	tests := []struct {
		name     string
		agents   map[string]service.SpawnerAgentConfig
		wantTags map[string]string
	}{
		{
			name: "builds map for tagged agents only",
			agents: map[string]service.SpawnerAgentConfig{
				"fe-impl":  {Tag: "fe"},
				"be-impl":  {Tag: "be"},
				"analyzer": {Tag: ""},
			},
			wantTags: map[string]string{"fe-impl": "fe", "be-impl": "be"},
		},
		{
			name:     "empty input returns empty map",
			agents:   map[string]service.SpawnerAgentConfig{},
			wantTags: map[string]string{},
		},
		{
			name: "all untagged agents excluded",
			agents: map[string]service.SpawnerAgentConfig{
				"untagged": {Tag: ""},
			},
			wantTags: map[string]string{},
		},
		{
			name: "multiple agents same tag",
			agents: map[string]service.SpawnerAgentConfig{
				"impl-a": {Tag: "be"},
				"impl-b": {Tag: "be"},
			},
			wantTags: map[string]string{"impl-a": "be", "impl-b": "be"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAgentTags(tt.agents)
			if len(got) != len(tt.wantTags) {
				t.Errorf("buildAgentTags() len = %d, want %d (got %v, want %v)", len(got), len(tt.wantTags), got, tt.wantTags)
				return
			}
			for k, v := range tt.wantTags {
				if got[k] != v {
					t.Errorf("buildAgentTags()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// TestShouldSkipLayer verifies the skip logic across all key scenarios.
func TestShouldSkipLayer(t *testing.T) {
	tests := []struct {
		name      string
		skipTags  []string
		agentTags map[string]string
		phases    []service.SpawnerPhaseDef
		wantSkip  bool
		wantTag   string
	}{
		{
			name:      "no skip_tags - never skip",
			skipTags:  []string{},
			agentTags: map[string]string{"fe-impl": "fe"},
			phases:    []service.SpawnerPhaseDef{{Agent: "fe-impl", Layer: 1}},
			wantSkip:  false,
			wantTag:   "",
		},
		{
			name:      "matching tag returns true with tag name",
			skipTags:  []string{"fe"},
			agentTags: map[string]string{"fe-impl": "fe"},
			phases:    []service.SpawnerPhaseDef{{Agent: "fe-impl", Layer: 1}},
			wantSkip:  true,
			wantTag:   "fe",
		},
		{
			name:      "non-matching tag - no skip",
			skipTags:  []string{"fe"},
			agentTags: map[string]string{"be-impl": "be"},
			phases:    []service.SpawnerPhaseDef{{Agent: "be-impl", Layer: 1}},
			wantSkip:  false,
			wantTag:   "",
		},
		{
			name:      "empty agent tag never matched",
			skipTags:  []string{"fe"},
			agentTags: map[string]string{}, // agent not in map → treated as no tag
			phases:    []service.SpawnerPhaseDef{{Agent: "untagged-agent", Layer: 1}},
			wantSkip:  false,
			wantTag:   "",
		},
		{
			name:      "multiple skip_tags matches any",
			skipTags:  []string{"be", "fe"},
			agentTags: map[string]string{"be-impl": "be"},
			phases:    []service.SpawnerPhaseDef{{Agent: "be-impl", Layer: 1}},
			wantSkip:  true,
			wantTag:   "be",
		},
		{
			name:     "one matching agent in multi-agent layer skips entire layer",
			skipTags: []string{"fe"},
			agentTags: map[string]string{
				"fe-impl": "fe",
				"be-impl": "be",
			},
			phases: []service.SpawnerPhaseDef{
				{Agent: "fe-impl", Layer: 2},
				{Agent: "be-impl", Layer: 2},
			},
			wantSkip: true,
			wantTag:  "fe",
		},
		{
			name:      "agent not in agentTags map treated as untagged",
			skipTags:  []string{"fe"},
			agentTags: map[string]string{},
			phases:    []service.SpawnerPhaseDef{{Agent: "mystery-agent", Layer: 0}},
			wantSkip:  false,
			wantTag:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.createTicket(t, "SKIP-1", "Skip tag test")
			wfiID := env.initWorkflow(t, "SKIP-1")

			if len(tt.skipTags) > 0 {
				tagsJSON, _ := json.Marshal(tt.skipTags)
				wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, env.orch.clock)
				if err := wfiRepo.UpdateSkipTags(wfiID, string(tagsJSON)); err != nil {
					t.Fatalf("UpdateSkipTags: %v", err)
				}
			}

			gotSkip, gotTag := env.orch.shouldSkipLayer(context.Background(), wfiID, tt.phases, tt.agentTags)
			if gotSkip != tt.wantSkip {
				t.Errorf("shouldSkipLayer() skip = %v, want %v", gotSkip, tt.wantSkip)
			}
			if gotTag != tt.wantTag {
				t.Errorf("shouldSkipLayer() tag = %q, want %q", gotTag, tt.wantTag)
			}
		})
	}
}

// TestCreateSkippedSessions verifies that skipped sessions are created in the DB
// with the correct status, result, timestamps, and metadata.
func TestCreateSkippedSessions(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "SKIP-SES", "Session creation test")
	wfiID := env.initWorkflow(t, "SKIP-SES")

	phases := []service.SpawnerPhaseDef{
		{Agent: "fe-impl", Layer: 1},
		{Agent: "fe-test", Layer: 1},
	}
	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "SKIP-SES",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}

	env.orch.createSkippedSessions(context.Background(), wfiID, req, phases, env.pool)

	// Verify correct number of sessions created
	var count int
	if err := env.pool.QueryRow(
		`SELECT COUNT(*) FROM agent_sessions WHERE workflow_instance_id = ?`, wfiID,
	).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 skipped sessions, got %d", count)
	}

	// Verify each session has correct status, result, and timestamps
	rows, err := env.pool.Query(
		`SELECT phase, agent_type, status, result, started_at, ended_at FROM agent_sessions WHERE workflow_instance_id = ? ORDER BY phase`,
		wfiID)
	if err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	defer rows.Close()

	wantAgents := map[string]bool{"fe-impl": false, "fe-test": false}
	for rows.Next() {
		var phase, agentType, status, result, startedAt, endedAt string
		if err := rows.Scan(&phase, &agentType, &status, &result, &startedAt, &endedAt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if status != string(model.AgentSessionSkipped) {
			t.Errorf("session %q status = %q, want %q", phase, status, model.AgentSessionSkipped)
		}
		if result != "skipped" {
			t.Errorf("session %q result = %q, want %q", phase, result, "skipped")
		}
		if startedAt == "" || endedAt == "" {
			t.Errorf("session %q has empty timestamps: started_at=%q ended_at=%q", phase, startedAt, endedAt)
		}
		if agentType != phase {
			t.Errorf("session %q agent_type = %q, want same as phase", phase, agentType)
		}
		wantAgents[phase] = true
	}
	for agent, seen := range wantAgents {
		if !seen {
			t.Errorf("expected session for agent %q, not found", agent)
		}
	}
}

// TestSkipLayerWSEvents verifies that EventLayerSkipped and per-agent EventAgentCompleted
// events are broadcast and received by subscribed WS clients.
func TestSkipLayerWSEvents(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "SKIP-WS", "WS skip event test")
	wfiID := env.initWorkflow(t, "SKIP-WS")

	ch := env.subscribeWSClient(t, "ws-client-skip", "SKIP-WS")

	phases := []service.SpawnerPhaseDef{
		{Agent: "fe-impl", Layer: 1},
	}
	agentNames := []string{"fe-impl"}

	// Simulate what runLoop broadcasts when a layer is skipped
	env.hub.Broadcast(ws.NewEvent(ws.EventAgentCompleted, env.project, "SKIP-WS", "test", map[string]interface{}{
		"agent_id":   phases[0].Agent,
		"agent_type": phases[0].Agent,
		"result":     "skipped",
	}))
	env.hub.Broadcast(ws.NewEvent(ws.EventLayerSkipped, env.project, "SKIP-WS", "test", map[string]interface{}{
		"instance_id": wfiID,
		"layer":       1,
		"skip_tag":    "fe",
		"agents":      agentNames,
	}))

	// EventAgentCompleted arrives first
	agentEvent := expectEvent(t, ch, ws.EventAgentCompleted, 2*time.Second)
	if agentEvent.Data["result"] != "skipped" {
		t.Errorf("EventAgentCompleted result = %v, want \"skipped\"", agentEvent.Data["result"])
	}

	// EventLayerSkipped arrives second
	layerEvent := expectEvent(t, ch, ws.EventLayerSkipped, 2*time.Second)
	if layerEvent.Data["skip_tag"] != "fe" {
		t.Errorf("EventLayerSkipped skip_tag = %v, want \"fe\"", layerEvent.Data["skip_tag"])
	}
}

// TestEventLayerSkippedConstant verifies the EventLayerSkipped constant value matches spec.
func TestEventLayerSkippedConstant(t *testing.T) {
	if ws.EventLayerSkipped != "layer.skipped" {
		t.Errorf("EventLayerSkipped = %q, want %q", ws.EventLayerSkipped, "layer.skipped")
	}
}
