package spawner

import (
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

func stepwiseAgentDef(stepsJSON string) *model.AgentDefinition {
	return &model.AgentDefinition{
		Tools:      "findings_add",
		PromptMode: service.PromptModeStepwise,
		Steps:      &stepsJSON,
	}
}

const oneStepJSON = `[{"step_id":"s1","title":"t","instruction":"i"}]`

// TestBuildAPIRegistry_Stepwise_RestrictiveCSVStillGetsCompleteStep verifies
// a stepwise def's complete_step tool survives a restrictive CSV that does
// not mention it at all (force-merge), mirroring the baseline pattern.
func TestBuildAPIRegistry_Stepwise_RestrictiveCSVStillGetsCompleteStep(t *testing.T) {
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	s := New(Config{
		DataPath:         env.dbPath,
		Pool:             db.WrapAsPool(env.database),
		Clock:            clock.Real(),
		PythonScriptRepo: repo.NewPythonScriptRepo(env.database, clock.Real()),
	})

	proc := &processInfo{sessionID: "sess-step-a"}
	req := SpawnRequest{ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	agentDef := stepwiseAgentDef(oneStepJSON)

	_, handlers, _, err := s.buildAPIRegistry(req, env.wfiID, agentDef, proc, "findings_add", false, false, false)
	if err != nil {
		t.Fatalf("buildAPIRegistry: %v", err)
	}
	if _, ok := handlers["complete_step"]; !ok {
		t.Error("complete_step missing for a stepwise def with a restrictive CSV that omits it")
	}
}

// TestBuildAPIRegistry_Stepwise_ExplicitCSVResolvesWithoutError verifies an
// agent def whose tools CSV explicitly lists "complete_step" resolves
// cleanly (it's in the resolvable pool via StepwiseBuiltins, so
// ResolveRegistry's "no tools matched pattern" never fires for it).
func TestBuildAPIRegistry_Stepwise_ExplicitCSVResolvesWithoutError(t *testing.T) {
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	s := New(Config{
		DataPath:         env.dbPath,
		Pool:             db.WrapAsPool(env.database),
		Clock:            clock.Real(),
		PythonScriptRepo: repo.NewPythonScriptRepo(env.database, clock.Real()),
	})

	proc := &processInfo{sessionID: "sess-step-b"}
	req := SpawnRequest{ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	agentDef := stepwiseAgentDef(oneStepJSON)

	_, handlers, _, err := s.buildAPIRegistry(req, env.wfiID, agentDef, proc, "complete_step", false, false, false)
	if err != nil {
		t.Fatalf("buildAPIRegistry with explicit complete_step CSV: unexpected error (want no 'no tools matched pattern'): %v", err)
	}
	if _, ok := handlers["complete_step"]; !ok {
		t.Error("complete_step missing after an explicit CSV entry")
	}
}

// TestBuildAPIRegistry_FullMode_NeverSeesCompleteStep is the hard requirement:
// a full-mode (non-stepwise) def with tools="*" must never resolve
// complete_step, in either the specs or the handlers — it is not in
// tools_builtin.Builtins()'s pool.
func TestBuildAPIRegistry_FullMode_NeverSeesCompleteStep(t *testing.T) {
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	s := New(Config{
		DataPath:         env.dbPath,
		Pool:             db.WrapAsPool(env.database),
		Clock:            clock.Real(),
		PythonScriptRepo: repo.NewPythonScriptRepo(env.database, clock.Real()),
	})

	proc := &processInfo{sessionID: "sess-step-c"}
	req := SpawnRequest{ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	agentDef := &model.AgentDefinition{Tools: "*"} // full-mode: PromptMode zero-value, no Steps

	specs, handlers, _, err := s.buildAPIRegistry(req, env.wfiID, agentDef, proc, "*", true, false, false)
	if err != nil {
		t.Fatalf("buildAPIRegistry: %v", err)
	}
	if _, ok := handlers["complete_step"]; ok {
		t.Error("complete_step present in handlers for a full-mode def with tools=\"*\"")
	}
	for _, spec := range specs {
		if spec.Name == "complete_step" {
			t.Error("complete_step present in specs for a full-mode def with tools=\"*\"")
		}
	}
}

// TestBuildAPIRegistry_Stepwise_MissingCSVStillForceMerged verifies an empty
// tools CSV (text-only agent) still gets complete_step force-merged for a
// stepwise def — the force-merge is unconditional on isStepwiseDef, not
// contingent on any other tool resolving.
func TestBuildAPIRegistry_Stepwise_MissingCSVStillForceMerged(t *testing.T) {
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	s := New(Config{
		DataPath:         env.dbPath,
		Pool:             db.WrapAsPool(env.database),
		Clock:            clock.Real(),
		PythonScriptRepo: repo.NewPythonScriptRepo(env.database, clock.Real()),
	})

	proc := &processInfo{sessionID: "sess-step-d"}
	req := SpawnRequest{ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	agentDef := stepwiseAgentDef(oneStepJSON)

	specs, handlers, _, err := s.buildAPIRegistry(req, env.wfiID, agentDef, proc, "", false, false, false)
	if err != nil {
		t.Fatalf("buildAPIRegistry: %v", err)
	}
	if _, ok := handlers["complete_step"]; !ok {
		t.Error("complete_step missing for a stepwise def with an empty tools CSV")
	}
	if len(specs) != len(handlers) {
		t.Errorf("specs len %d != handlers len %d", len(specs), len(handlers))
	}
}
