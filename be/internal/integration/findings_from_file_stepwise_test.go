package integration

// TestFindingsFromFileStepwiseAcceptance exercises findings_add_from_file end
// to end: a real workdir file is stored as a session finding through the
// actual FindingsService, and a stepwise complete_step accepts it as
// evidence for the required "summary" finding — the same acceptance seam
// TestStepwiseFullLoop exercises for findings_add.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/tools_builtin"
	"be/internal/types"
)

func TestFindingsFromFileStepwiseAcceptance(t *testing.T) {
	env := NewTestEnv(t)

	ticketID := "SW-FILE-1"
	env.CreateTicket(t, ticketID, "Stepwise findings-from-file")
	env.InitWorkflow(t, ticketID)
	wfiID := env.GetWorkflowInstanceID(t, ticketID, "test")
	const nodeID = "stepper"

	def := createStepwiseAgentDef(t, env, nodeID, 2, stepwiseTwoSteps())
	snapshotCursor(t, env, def, wfiID, nodeID)

	sessionID := "sw-file-sess-1"
	env.InsertAgentSession(t, sessionID, ticketID, wfiID, nodeID, nodeID, "")

	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "evidence.txt"), []byte("did step one, verified"), 0o644); err != nil {
		t.Fatalf("write evidence file: %v", err)
	}

	fromFileEnv := apirun.ToolEnv{
		Pool:               env.Pool,
		WSHub:              env.Hub,
		Clock:              env.Clock,
		SessionID:          sessionID,
		ProjectID:          env.ProjectID,
		TicketID:           ticketID,
		WorkflowName:       "test",
		WorkflowInstanceID: wfiID,
		NodeID:             nodeID,
		WorkDir:            workDir,
		Findings:           env.FindingsSvc,
	}

	handler := tools_builtin.Builtins()["findings_add_from_file"]
	input, err := json.Marshal(map[string]interface{}{"key": "summary", "path": "evidence.txt"})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	out, isErr, err := handler.Invoke(context.Background(), fromFileEnv, json.RawMessage(input))
	if err != nil || isErr {
		t.Fatalf("findings_add_from_file failed: out=%q isErr=%v err=%v", out, isErr, err)
	}
	var stored struct {
		Key   string `json:"key"`
		Bytes int    `json:"bytes"`
	}
	if uerr := json.Unmarshal([]byte(out), &stored); uerr != nil {
		t.Fatalf("unmarshal findings_add_from_file output %q: %v", out, uerr)
	}
	if stored.Key != "summary" {
		t.Errorf("stored.Key = %q, want summary", stored.Key)
	}

	// findings_get returns the file content back out.
	getRes, err := env.FindingsSvc.Get(&types.FindingsGetRequest{
		Key:        "summary",
		SessionID:  sessionID,
		InstanceID: wfiID,
	})
	if err != nil {
		t.Fatalf("FindingsSvc.Get: %v", err)
	}
	if getRes != "did step one, verified" {
		t.Errorf("findings_get(summary) = %v, want file content", getRes)
	}

	// complete_step (stepwise evidence acceptance) accepts the file-sourced
	// finding for the step's required "summary" key.
	steps := &fakeStepSession{}
	stepOut, stepIsErr, stepErr := completeStep(env, steps, sessionID, ticketID, wfiID, nodeID, "s1", 1, []string{"summary"})
	if stepErr != nil || stepIsErr {
		t.Fatalf("complete_step(s1) rejected file-sourced evidence: out=%q isErr=%v err=%v", stepOut, stepIsErr, stepErr)
	}

	after := env.WorkflowSvc.BuildStepCursors(wfiID)
	prog := after[nodeID]
	if prog == nil || prog.CurrentStepID != "s2" {
		t.Fatalf("progress after complete_step(s1) = %+v, want current_step_id=s2", prog)
	}
}
