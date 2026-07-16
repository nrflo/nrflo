package spawner

import (
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner/apirun/tools_builtin"
)

// TestBuildAPIRegistry_BaselineForce verifies that forceBaseline merges the
// baseline tools (agent_* lifecycle group plus findings_add) regardless of a
// restrictive tools CSV, and that the flag is a no-op when false.
func TestBuildAPIRegistry_BaselineForce(t *testing.T) {
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

	// forceBaseline=false: only the explicitly-selected tool resolves.
	_, handlers, _, err := s.buildAPIRegistry(req, env.wfiID, agentDef, proc, "findings_add", false, false)
	if err != nil {
		t.Fatalf("buildAPIRegistry(false): %v", err)
	}
	if _, ok := handlers["agent_finished"]; ok {
		t.Errorf("agent_finished present without forceBaseline")
	}

	// forceBaseline=true: all baseline tools are force-merged in.
	specs, handlers, _, err := s.buildAPIRegistry(req, env.wfiID, agentDef, proc, "findings_add", true, false)
	if err != nil {
		t.Fatalf("buildAPIRegistry(true): %v", err)
	}
	for _, n := range tools_builtin.BaselineToolNames() {
		if _, ok := handlers[n]; !ok {
			t.Errorf("baseline tool %q missing with forceBaseline=true", n)
		}
	}
	if _, ok := handlers["findings_add"]; !ok {
		t.Errorf("findings_add dropped by baseline merge")
	}
	if len(specs) != len(handlers) {
		t.Errorf("specs len %d != handlers len %d", len(specs), len(handlers))
	}

	// Restrictive CSV that excludes findings_add: forceBaseline=true must
	// include findings_add; forceBaseline=false must not.
	_, handlersNoForce, _, err := s.buildAPIRegistry(req, env.wfiID, agentDef, proc, "agent_finished", false, false)
	if err != nil {
		t.Fatalf("buildAPIRegistry restrictive/false: %v", err)
	}
	if _, ok := handlersNoForce["findings_add"]; ok {
		t.Errorf("findings_add present in handlers with forceBaseline=false and restrictive CSV")
	}

	specsForce, handlersForce, _, err := s.buildAPIRegistry(req, env.wfiID, agentDef, proc, "agent_finished", true, false)
	if err != nil {
		t.Fatalf("buildAPIRegistry restrictive/true: %v", err)
	}
	if _, ok := handlersForce["findings_add"]; !ok {
		t.Errorf("findings_add absent in handlers with forceBaseline=true and restrictive CSV")
	}
	if len(specsForce) != len(handlersForce) {
		t.Errorf("specs len %d != handlers len %d after restrictive-CSV force merge", len(specsForce), len(handlersForce))
	}
}
