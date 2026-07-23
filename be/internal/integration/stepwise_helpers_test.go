package integration

// Shared fixtures for the stepwise full-loop integration test
// (stepwise_test.go): real cross-layer wiring (FindingRepo, cursor repo,
// BuildStepCursors read model, WS hub) driven through the actual
// complete_step builtin handler — no mocked stepengine/read-model layer.

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/service/stepengine"
	"be/internal/types"
)

// stepwiseTwoSteps is a canonical two-step sequence: each step requires a
// nonempty_text "summary" finding, both allow rotation so rotation-path
// coverage stays available to callers that opt in via context tokens.
func stepwiseTwoSteps() []model.StepDefinition {
	return []model.StepDefinition{
		{StepID: "s1", Title: "Step One", Instruction: "do step one",
			RequiredFindings: []model.RequiredFinding{{Key: "summary", Schema: model.FindingSchemaNonemptyText}},
			RotationAllowed:  true},
		{StepID: "s2", Title: "Step Two", Instruction: "do step two",
			RequiredFindings: []model.RequiredFinding{{Key: "summary", Schema: model.FindingSchemaNonemptyText}},
			RotationAllowed:  true},
	}
}

// createStepwiseAgentDef inserts a prompt_mode='stepwise' agent def for the
// TestEnv's project/"test" workflow.
func createStepwiseAgentDef(t *testing.T, env *TestEnv, agentID string, layer int, steps []model.StepDefinition) *model.AgentDefinition {
	t.Helper()
	b, err := json.Marshal(steps)
	if err != nil {
		t.Fatalf("marshal steps: %v", err)
	}
	stepsJSON := string(b)

	def := &model.AgentDefinition{
		ID:         agentID,
		ProjectID:  env.ProjectID,
		WorkflowID: "test",
		Layer:      layer,
		Model:      "sonnet-5",
		Timeout:    3600,
		Prompt:     "stepwise agent",
		PromptMode: service.PromptModeStepwise,
		Steps:      &stepsJSON,
	}
	adRepo := repo.NewAgentDefinitionRepo(env.Pool, env.Clock)
	if err := adRepo.Create(def); err != nil {
		t.Fatalf("createStepwiseAgentDef: %v", err)
	}
	return def
}

// snapshotCursor mirrors spawner.snapshotStepCursor: idempotently creates the
// step cursor for a stepwise def before any complete_step call.
func snapshotCursor(t *testing.T, env *TestEnv, def *model.AgentDefinition, wfiID, nodeID string) {
	t.Helper()
	engine := stepengine.New(env.Pool, env.Clock, nil)
	if _, err := engine.Snapshot(wfiID, nodeID, def); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
}

// seedSummaryFinding writes a nonempty_text "summary" finding for sessionID
// via the real FindingsService.Add — the same seam the findings_add tool
// handler calls. Value is JSON-encoded as a string per the nonempty_text
// schema (matches seedSummaryFinding in tools_builtin/complete_step_test.go).
func seedSummaryFinding(t *testing.T, env *TestEnv, sessionID, value string) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	if _, err := env.FindingsSvc.Add(&types.FindingsAddRequest{
		Key: "summary", Value: string(b), SessionID: sessionID,
	}); err != nil {
		t.Fatalf("seed finding: %v", err)
	}
}

// fakeStepSession is a canned apirun.StepSession — no real spawner/CLI
// backend (Rule 4) — recording boundary/rotation calls for assertion.
type fakeStepSession struct {
	boundaryCalls    []string
	rotationRequests []string
}

func (f *fakeStepSession) RotateSignals(sessionID string) (int, int) { return 0, 0 }
func (f *fakeStepSession) NoteStepBoundary(sessionID string) {
	f.boundaryCalls = append(f.boundaryCalls, sessionID)
}
func (f *fakeStepSession) RequestStepRotation(sessionID string) {
	f.rotationRequests = append(f.rotationRequests, sessionID)
}
func (f *fakeStepSession) RunStepChecks(ctx context.Context, sessionID string, cmds []string) (int, int, string, error) {
	return -1, 0, "", nil
}
