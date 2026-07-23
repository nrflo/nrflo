package spawner

// Shared fixtures for the stepwise prompt-assembly tests
// (template_stepwise_test.go, spawner_prepare_stepwise_test.go,
// template_stepwise_resume_test.go, context_save_stepwise_test.go).

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// threeSteps returns a canonical 3-step stepwise sequence: distinct titles
// and instructions per step so tests can assert on exact per-step content
// and on the "cannot see the future" guarantee.
func threeSteps() []model.StepDefinition {
	return []model.StepDefinition{
		{StepID: "step-one", Title: "Title One", Instruction: "Instruction body one."},
		{StepID: "step-two", Title: "Title Two", Instruction: "Instruction body two."},
		{StepID: "step-three", Title: "Title Three", Instruction: "Instruction body three."},
	}
}

// createStepwiseAgentDef inserts a prompt_mode='stepwise' agent def under the
// spawnerTestEnv's project/"test" workflow.
func createStepwiseAgentDef(t *testing.T, env *spawnerTestEnv, agentID string, steps []model.StepDefinition) {
	t.Helper()
	b, err := json.Marshal(steps)
	if err != nil {
		t.Fatalf("marshal steps: %v", err)
	}
	stepsJSON := string(b)

	adRepo := repo.NewAgentDefinitionRepo(env.pool, clock.Real())
	if err := adRepo.Create(&model.AgentDefinition{
		ID:         agentID,
		ProjectID:  env.project,
		WorkflowID: "test",
		Model:      "sonnet-5",
		Timeout:    3600,
		Prompt:     "Main prompt body",
		PromptMode: service.PromptModeStepwise,
		Steps:      &stepsJSON,
	}); err != nil {
		t.Fatalf("createStepwiseAgentDef: %v", err)
	}
}

// createStepwiseAgentDefInContextEnv is the contextSaveTestEnv counterpart,
// under the "feature" workflow, for the prepareSpawn/context-save tests that
// share that fixture.
func createStepwiseAgentDefInContextEnv(t *testing.T, env *contextSaveTestEnv, agentID, modelID string, steps []model.StepDefinition) {
	t.Helper()
	b, err := json.Marshal(steps)
	if err != nil {
		t.Fatalf("marshal steps: %v", err)
	}
	stepsJSON := string(b)

	adRepo := repo.NewAgentDefinitionRepo(env.database, clock.Real())
	if err := adRepo.Create(&model.AgentDefinition{
		ID:         agentID,
		ProjectID:  env.projectID,
		WorkflowID: "feature",
		Model:      modelID,
		Timeout:    3600,
		Prompt:     "Main prompt body",
		PromptMode: service.PromptModeStepwise,
		Steps:      &stepsJSON,
	}); err != nil {
		t.Fatalf("createStepwiseAgentDefInContextEnv: %v", err)
	}
}
