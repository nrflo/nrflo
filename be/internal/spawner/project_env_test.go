package spawner

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestPrepareScriptSpawn_ProjectEnvAppended verifies that Config.ProjectEnv entries
// are present in opts.Env after prepareScriptSpawn.
func TestPrepareScriptSpawn_ProjectEnvAppended(t *testing.T) {
	t.Parallel()
	env := setupScriptSpawnEnv(t)
	t.Cleanup(env.cleanup)

	env.spawner.config.ProjectEnv = []string{"MY_CUSTOM_VAR=hello", "ANOTHER_VAR=world"}

	agentDef := makeMinimalAgentDef(env.scriptID)
	_, prep, err := env.spawner.prepareScriptSpawn(
		context.Background(),
		SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
		"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
		agentDef,
	)
	if err != nil {
		t.Fatalf("prepareScriptSpawn() error: %v", err)
	}

	if !projEnvContains(prep.opts.Env, "MY_CUSTOM_VAR=hello") {
		t.Errorf("opts.Env missing MY_CUSTOM_VAR=hello; env=%v", prep.opts.Env)
	}
	if !projEnvContains(prep.opts.Env, "ANOTHER_VAR=world") {
		t.Errorf("opts.Env missing ANOTHER_VAR=world; env=%v", prep.opts.Env)
	}
}

// TestPrepareScriptSpawn_EmptyProjectEnvIsNoOp verifies that nil Config.ProjectEnv
// does not alter the nrflo-controlled env entries.
func TestPrepareScriptSpawn_EmptyProjectEnvIsNoOp(t *testing.T) {
	t.Parallel()
	env := setupScriptSpawnEnv(t)
	t.Cleanup(env.cleanup)

	// ProjectEnv is nil by default in setupScriptSpawnEnv.
	agentDef := makeMinimalAgentDef(env.scriptID)
	_, prep, err := env.spawner.prepareScriptSpawn(
		context.Background(),
		SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
		"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
		agentDef,
	)
	if err != nil {
		t.Fatalf("prepareScriptSpawn() error: %v", err)
	}

	nrfloKeys := []string{"NRFLO_PROJECT", "NRF_WORKFLOW_INSTANCE_ID", "NRF_SESSION_ID", "NRFLO_AGENT_TOKEN", "NRF_SPAWNED"}
	for _, key := range nrfloKeys {
		if !projEnvHasKey(prep.opts.Env, key) {
			t.Errorf("opts.Env missing nrflo-controlled key %s; env=%v", key, prep.opts.Env)
		}
	}
	// No phantom custom entries from a nil ProjectEnv.
	if projEnvContains(prep.opts.Env, "MY_CUSTOM_VAR=hello") {
		t.Errorf("opts.Env unexpectedly contains MY_CUSTOM_VAR=hello with nil ProjectEnv")
	}
}

// TestPrepareScriptSpawn_ProjectEnvAppearsAfterNrfloVars verifies that
// Config.ProjectEnv entries appear AFTER nrflo-controlled vars in opts.Env.
// This documents the slice ordering so a regression that flips it is caught;
// the service-layer reserved-name validator is the primary shadowing defense.
func TestPrepareScriptSpawn_ProjectEnvAppearsAfterNrfloVars(t *testing.T) {
	t.Parallel()
	env := setupScriptSpawnEnv(t)
	t.Cleanup(env.cleanup)

	// Use a name that looks like a nrflo-reserved var to verify ordering.
	// In production the service validator prevents storing reserved names;
	// here we set Config.ProjectEnv directly to test slice position only.
	env.spawner.config.ProjectEnv = []string{"NRF_SPAWNED=project-override"}

	agentDef := makeMinimalAgentDef(env.scriptID)
	_, prep, err := env.spawner.prepareScriptSpawn(
		context.Background(),
		SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
		"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
		agentDef,
	)
	if err != nil {
		t.Fatalf("prepareScriptSpawn() error: %v", err)
	}

	nrfloIdx := -1
	projectIdx := -1
	for i, e := range prep.opts.Env {
		if e == "NRF_SPAWNED=1" {
			nrfloIdx = i
		}
		if e == "NRF_SPAWNED=project-override" {
			projectIdx = i
		}
	}
	if nrfloIdx == -1 {
		t.Fatalf("NRF_SPAWNED=1 not found in opts.Env; env=%v", prep.opts.Env)
	}
	if projectIdx == -1 {
		t.Fatalf("NRF_SPAWNED=project-override not found in opts.Env; env=%v", prep.opts.Env)
	}
	if nrfloIdx >= projectIdx {
		t.Errorf("nrflo NRF_SPAWNED=1 at idx=%d must come before project NRF_SPAWNED=project-override at idx=%d",
			nrfloIdx, projectIdx)
	}
}

// projEnvContains checks if the exact "KEY=VALUE" entry exists in the env slice.
func projEnvContains(env []string, entry string) bool {
	for _, e := range env {
		if e == entry {
			return true
		}
	}
	return false
}

// projEnvHasKey checks if any entry in env starts with "KEY=".
func projEnvHasKey(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}
