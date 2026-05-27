package spawner

import (
	"context"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner/apirun/tools_builtin"
)

// TestBuildAPIRegistry_LifecycleBaseline verifies that forceLifecycleBaseline
// merges the agent_* lifecycle tools regardless of a restrictive tools CSV, and
// that the flag is a no-op when false.
func TestBuildAPIRegistry_LifecycleBaseline(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	s := New(Config{
		DataPath:         env.dbPath,
		Pool:             db.WrapAsPool(env.database),
		Clock:            clock.Real(),
		PythonScriptRepo: repo.NewPythonScriptRepo(env.database, clock.Real()),
	})

	proc := &processInfo{sessionID: "sess-x"}
	req := SpawnRequest{ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	agentDef := &model.AgentDefinition{Tools: "findings_add"}

	// forceLifecycleBaseline=false: only the explicitly-selected tool resolves.
	_, handlers, _, err := s.buildAPIRegistry(context.Background(), req, env.wfiID, agentDef, proc, "findings_add", false)
	if err != nil {
		t.Fatalf("buildAPIRegistry(false): %v", err)
	}
	if _, ok := handlers["agent_finished"]; ok {
		t.Errorf("agent_finished present without forceLifecycleBaseline")
	}

	// forceLifecycleBaseline=true: lifecycle tools are force-merged in.
	specs, handlers, _, err := s.buildAPIRegistry(context.Background(), req, env.wfiID, agentDef, proc, "findings_add", true)
	if err != nil {
		t.Fatalf("buildAPIRegistry(true): %v", err)
	}
	for _, n := range tools_builtin.LifecycleToolNames() {
		if _, ok := handlers[n]; !ok {
			t.Errorf("baseline tool %q missing with forceLifecycleBaseline=true", n)
		}
	}
	if _, ok := handlers["findings_add"]; !ok {
		t.Errorf("findings_add dropped by baseline merge")
	}
	if len(specs) != len(handlers) {
		t.Errorf("specs len %d != handlers len %d", len(specs), len(handlers))
	}
}
