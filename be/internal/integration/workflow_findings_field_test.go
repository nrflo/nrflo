package integration

import (
	"encoding/json"
	"testing"

	"be/internal/types"
)

// setWorkflowInstanceFindings sets the findings JSON on a workflow instance via direct SQL.
func setWorkflowInstanceFindings(t *testing.T, env *TestEnv, wfiID string, findings map[string]interface{}) {
	t.Helper()
	data, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("setWorkflowInstanceFindings: marshal: %v", err)
	}
	_, err = env.Pool.Exec(`UPDATE workflow_instances SET findings = ? WHERE id = ?`, string(data), wfiID)
	if err != nil {
		t.Fatalf("setWorkflowInstanceFindings: exec: %v", err)
	}
}

// TestWorkflowFindings_NonInternalKeysExposed verifies that non-internal
// workflow-level findings (no "_" prefix) appear in the workflow_findings field.
func TestWorkflowFindings_NonInternalKeysExposed(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WFF-1", "workflow findings exposed")
	env.InitWorkflow(t, "WFF-1")
	wfiID := env.GetWorkflowInstanceID(t, "WFF-1", "test")

	setWorkflowInstanceFindings(t, env, wfiID, map[string]interface{}{
		"summary":        "everything looks good",
		"reviewed_by":    "qa-agent",
		"_orchestration": "internal metadata",
		"_callback":      map[string]interface{}{"level": 0},
	})

	status, err := getWorkflowStatus(t, env, "WFF-1", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	wf, ok := status["workflow_findings"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workflow_findings map, got %T: %v", status["workflow_findings"], status["workflow_findings"])
	}

	if wf["summary"] != "everything looks good" {
		t.Errorf("workflow_findings[summary] = %v, want %q", wf["summary"], "everything looks good")
	}
	if wf["reviewed_by"] != "qa-agent" {
		t.Errorf("workflow_findings[reviewed_by] = %v, want %q", wf["reviewed_by"], "qa-agent")
	}
}

// TestWorkflowFindings_InternalKeysHidden verifies that keys starting with "_"
// are excluded from workflow_findings.
func TestWorkflowFindings_InternalKeysHidden(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WFF-2", "internal keys hidden")
	env.InitWorkflow(t, "WFF-2")
	wfiID := env.GetWorkflowInstanceID(t, "WFF-2", "test")

	setWorkflowInstanceFindings(t, env, wfiID, map[string]interface{}{
		"summary":        "done",
		"_orchestration": "internal metadata",
		"_callback":      map[string]interface{}{"level": 0},
	})

	status, err := getWorkflowStatus(t, env, "WFF-2", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	wf, ok := status["workflow_findings"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workflow_findings map, got %T: %v", status["workflow_findings"], status["workflow_findings"])
	}

	if _, exists := wf["_orchestration"]; exists {
		t.Errorf("workflow_findings should not contain _orchestration")
	}
	if _, exists := wf["_callback"]; exists {
		t.Errorf("workflow_findings should not contain _callback")
	}
	if wf["summary"] != "done" {
		t.Errorf("workflow_findings[summary] = %v, want %q", wf["summary"], "done")
	}
}

// TestWorkflowFindings_OmittedWhenAllInternal verifies that workflow_findings is
// absent from the response when all workflow-level findings are internal keys.
func TestWorkflowFindings_OmittedWhenAllInternal(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WFF-3", "all internal")
	env.InitWorkflow(t, "WFF-3")
	wfiID := env.GetWorkflowInstanceID(t, "WFF-3", "test")

	setWorkflowInstanceFindings(t, env, wfiID, map[string]interface{}{
		"_orchestration": "metadata",
		"_callback":      "cb",
	})

	status, err := getWorkflowStatus(t, env, "WFF-3", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if _, exists := status["workflow_findings"]; exists {
		t.Errorf("workflow_findings should be absent when all keys are internal, got %v", status["workflow_findings"])
	}
}

// TestWorkflowFindings_OmittedWhenEmpty verifies that workflow_findings is absent
// from the response when the workflow instance has no findings at all.
func TestWorkflowFindings_OmittedWhenEmpty(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WFF-4", "no findings")
	env.InitWorkflow(t, "WFF-4")

	status, err := getWorkflowStatus(t, env, "WFF-4", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if _, exists := status["workflow_findings"]; exists {
		t.Errorf("workflow_findings should be absent when workflow instance has no findings, got %v", status["workflow_findings"])
	}
}

// TestWorkflowFindings_SeparateFromCombinedFindings verifies that workflow_findings
// is separate from the combined findings field, which includes agent session findings.
func TestWorkflowFindings_SeparateFromCombinedFindings(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WFF-5", "separate fields")
	env.InitWorkflow(t, "WFF-5")
	wfiID := env.GetWorkflowInstanceID(t, "WFF-5", "test")

	// Set a workflow-level finding
	setWorkflowInstanceFindings(t, env, wfiID, map[string]interface{}{
		"wf_key": "wf_value",
	})

	// Add an agent session with its own findings
	env.InsertAgentSession(t, "sess-wff-5", "WFF-5", wfiID, "analyzer", "analyzer", "")
	setSessionFindings(t, env, "sess-wff-5", map[string]interface{}{
		"agent_key": "agent_value",
	})
	env.CompleteAgentSession(t, "sess-wff-5", "pass")

	status, err := getWorkflowStatus(t, env, "WFF-5", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	// workflow_findings should contain only the workflow-level key
	wf, ok := status["workflow_findings"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workflow_findings map, got %T: %v", status["workflow_findings"], status["workflow_findings"])
	}
	if wf["wf_key"] != "wf_value" {
		t.Errorf("workflow_findings[wf_key] = %v, want %q", wf["wf_key"], "wf_value")
	}
	// agent_key should NOT appear directly in workflow_findings
	if _, exists := wf["agent_key"]; exists {
		t.Errorf("workflow_findings should not contain agent session key agent_key")
	}
	// agent type key should NOT appear in workflow_findings
	if _, exists := wf["analyzer"]; exists {
		t.Errorf("workflow_findings should not contain agent type key 'analyzer'")
	}

	// combined findings include workflow-level entries merged at top level, and
	// agent session findings nested under their agent type key.
	combined, ok := status["findings"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected findings map, got %T: %v", status["findings"], status["findings"])
	}
	// workflow-level key appears at top level in combined
	if combined["wf_key"] != "wf_value" {
		t.Errorf("findings[wf_key] = %v, want %q", combined["wf_key"], "wf_value")
	}
	// agent session findings are nested under the agent type key
	agentFindings, ok := combined["analyzer"].(map[string]interface{})
	if !ok {
		t.Fatalf("findings[analyzer] should be a map, got %T: %v", combined["analyzer"], combined["analyzer"])
	}
	if agentFindings["agent_key"] != "agent_value" {
		t.Errorf("findings[analyzer][agent_key] = %v, want %q", agentFindings["agent_key"], "agent_value")
	}
}

// TestWorkflowFindings_TableDriven tests multiple scenarios for internal key filtering.
func TestWorkflowFindings_InternalKeyFiltering(t *testing.T) {
	cases := []struct {
		name             string
		ticketID         string
		input            map[string]interface{}
		wantPresent      bool
		wantKeys         []string
		wantAbsentKeys   []string
	}{
		{
			name:     "single_public_key",
			ticketID: "WFF-T1",
			input:    map[string]interface{}{"result": "ok"},
			wantPresent: true,
			wantKeys:    []string{"result"},
		},
		{
			name:     "mixed_public_and_internal",
			ticketID: "WFF-T2",
			input:    map[string]interface{}{"pub": "val", "_priv": "hidden"},
			wantPresent:    true,
			wantKeys:       []string{"pub"},
			wantAbsentKeys: []string{"_priv"},
		},
		{
			name:        "only_internal_keys",
			ticketID:    "WFF-T3",
			input:       map[string]interface{}{"_x": "a", "_y": "b"},
			wantPresent: false,
		},
		{
			name:        "empty_findings",
			ticketID:    "WFF-T4",
			input:       map[string]interface{}{},
			wantPresent: false,
		},
		{
			name:     "underscore_not_at_start",
			ticketID: "WFF-T5",
			input:    map[string]interface{}{"key_with_underscore": "val"},
			wantPresent: true,
			wantKeys:    []string{"key_with_underscore"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := NewTestEnv(t)
			env.CreateTicket(t, tc.ticketID, tc.name)
			env.InitWorkflow(t, tc.ticketID)
			wfiID := env.GetWorkflowInstanceID(t, tc.ticketID, "test")

			if len(tc.input) > 0 {
				setWorkflowInstanceFindings(t, env, wfiID, tc.input)
			}

			status, err := getWorkflowStatus(t, env, tc.ticketID, &types.WorkflowGetRequest{Workflow: "test"})
			if err != nil {
				t.Fatalf("GetStatus: %v", err)
			}

			_, exists := status["workflow_findings"]
			if exists != tc.wantPresent {
				t.Errorf("workflow_findings present=%v, want %v (value: %v)", exists, tc.wantPresent, status["workflow_findings"])
			}

			if !tc.wantPresent {
				return
			}

			wf, ok := status["workflow_findings"].(map[string]interface{})
			if !ok {
				t.Fatalf("workflow_findings is not a map: %T", status["workflow_findings"])
			}
			for _, k := range tc.wantKeys {
				if _, ok := wf[k]; !ok {
					t.Errorf("workflow_findings missing expected key %q", k)
				}
			}
			for _, k := range tc.wantAbsentKeys {
				if _, ok := wf[k]; ok {
					t.Errorf("workflow_findings should not contain key %q", k)
				}
			}
		})
	}
}
