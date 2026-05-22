package integration

import (
	"testing"

	"be/internal/types"
)

// TestWorkflowCallbackField_SeparateFromWorkflowFindings verifies that _callback
// is excluded from workflow_findings but exposed as a top-level "callback" field.
func TestWorkflowCallbackField_SeparateFromWorkflowFindings(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WCF-5", "callback separate from wf_findings")
	env.InitWorkflow(t, "WCF-5")
	wfiID := env.GetWorkflowInstanceID(t, "WCF-5", "test")

	setWorkflowInstanceFindings(t, env, wfiID, map[string]interface{}{
		"summary": "done",
		"_callback": map[string]interface{}{
			"level":        float64(0),
			"instructions": "Retry implementor",
			"from_layer":   float64(2),
			"from_agent":   "qa-verifier",
		},
	})

	status, err := getWorkflowStatus(t, env, "WCF-5", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	// workflow_findings must NOT contain _callback
	wf, ok := status["workflow_findings"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workflow_findings map, got %T: %v", status["workflow_findings"], status["workflow_findings"])
	}
	if _, exists := wf["_callback"]; exists {
		t.Errorf("workflow_findings must not contain _callback")
	}
	if wf["summary"] != "done" {
		t.Errorf("workflow_findings[summary] = %v, want %q", wf["summary"], "done")
	}

	// top-level "callback" must be present with correct data
	cb, ok := status["callback"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'callback' map, got %T: %v", status["callback"], status["callback"])
	}
	if cb["from_agent"] != "qa-verifier" {
		t.Errorf("callback.from_agent = %v, want %q", cb["from_agent"], "qa-verifier")
	}
	if cb["instructions"] != "Retry implementor" {
		t.Errorf("callback.instructions = %v, want %q", cb["instructions"], "Retry implementor")
	}
}

// TestWorkflowCallbackField_TableDriven tests multiple scenarios for the callback field.
func TestWorkflowCallbackField_TableDriven(t *testing.T) {
	cases := []struct {
		name         string
		ticketID     string
		findings     map[string]interface{}
		wantCallback bool
		wantLevel    float64
		wantAgent    string
		wantLayer    float64
		wantInstr    string
	}{
		{
			name:         "level_zero_callback",
			ticketID:     "WCF-T1",
			findings:     map[string]interface{}{"_callback": map[string]interface{}{"level": float64(0), "instructions": "redo", "from_layer": float64(1), "from_agent": "tester"}},
			wantCallback: true,
			wantLevel:    0,
			wantAgent:    "tester",
			wantLayer:    1,
			wantInstr:    "redo",
		},
		{
			name:         "high_layer_callback",
			ticketID:     "WCF-T2",
			findings:     map[string]interface{}{"_callback": map[string]interface{}{"level": float64(3), "instructions": "fix bug", "from_layer": float64(5), "from_agent": "qa-verifier"}},
			wantCallback: true,
			wantLevel:    3,
			wantAgent:    "qa-verifier",
			wantLayer:    5,
			wantInstr:    "fix bug",
		},
		{
			name:         "no_callback_key",
			ticketID:     "WCF-T3",
			findings:     map[string]interface{}{"summary": "ok"},
			wantCallback: false,
		},
		{
			name:         "empty_findings",
			ticketID:     "WCF-T4",
			findings:     map[string]interface{}{},
			wantCallback: false,
		},
		{
			name:         "callback_not_a_map",
			ticketID:     "WCF-T5",
			findings:     map[string]interface{}{"_callback": 42},
			wantCallback: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := NewTestEnv(t)
			env.CreateTicket(t, tc.ticketID, tc.name)
			env.InitWorkflow(t, tc.ticketID)
			wfiID := env.GetWorkflowInstanceID(t, tc.ticketID, "test")

			if len(tc.findings) > 0 {
				setWorkflowInstanceFindings(t, env, wfiID, tc.findings)
			}

			status, err := getWorkflowStatus(t, env, tc.ticketID, &types.WorkflowGetRequest{Workflow: "test"})
			if err != nil {
				t.Fatalf("GetStatus: %v", err)
			}

			_, exists := status["callback"]
			if exists != tc.wantCallback {
				t.Errorf("callback present=%v, want %v (value: %v)", exists, tc.wantCallback, status["callback"])
			}

			if !tc.wantCallback {
				return
			}

			cb, ok := status["callback"].(map[string]interface{})
			if !ok {
				t.Fatalf("callback is not a map: %T", status["callback"])
			}
			if cb["level"] != tc.wantLevel {
				t.Errorf("callback.level = %v, want %v", cb["level"], tc.wantLevel)
			}
			if cb["from_agent"] != tc.wantAgent {
				t.Errorf("callback.from_agent = %v, want %q", cb["from_agent"], tc.wantAgent)
			}
			if cb["from_layer"] != tc.wantLayer {
				t.Errorf("callback.from_layer = %v, want %v", cb["from_layer"], tc.wantLayer)
			}
			if cb["instructions"] != tc.wantInstr {
				t.Errorf("callback.instructions = %v, want %q", cb["instructions"], tc.wantInstr)
			}
		})
	}
}
