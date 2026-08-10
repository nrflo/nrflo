package spawner

import (
	"context"
	"errors"
	"testing"

	"be/internal/types"
)

func TestEnsureRefineryFoldHostInstance_MintsThenReuses(t *testing.T) {
	env := setupRefineryFoldTestEnv(t)
	defer env.cleanup()

	first, err := env.spawner.ensureRefineryFoldHostInstance(env.pool, env.projectID, "console-sess-1")
	if err != nil {
		t.Fatalf("ensureRefineryFoldHostInstance: %v", err)
	}
	if first == "" {
		t.Fatal("ensureRefineryFoldHostInstance returned empty instance id")
	}

	second, err := env.spawner.ensureRefineryFoldHostInstance(env.pool, env.projectID, "console-sess-1")
	if err != nil {
		t.Fatalf("ensureRefineryFoldHostInstance (reuse): %v", err)
	}
	if second != first {
		t.Errorf("same session minted a second instance: %q != %q", second, first)
	}

	other, err := env.spawner.ensureRefineryFoldHostInstance(env.pool, env.projectID, "console-sess-2")
	if err != nil {
		t.Fatalf("ensureRefineryFoldHostInstance (other session): %v", err)
	}
	if other == first {
		t.Error("different sessions must not share a host instance")
	}

	var workflowID, origin, originSID string
	if err := env.database.QueryRow(
		`SELECT workflow_id, origin, origin_session_id FROM workflow_instances WHERE id = ?`, first,
	).Scan(&workflowID, &origin, &originSID); err != nil {
		t.Fatalf("read minted instance: %v", err)
	}
	if workflowID != refineryFoldHiddenWorkflow || origin != "console" || originSID != "console-sess-1" {
		t.Errorf("minted instance = (%q, %q, %q), want (%q, console, console-sess-1)",
			workflowID, origin, originSID, refineryFoldHiddenWorkflow)
	}
}

// A console fold carries no WorkflowInstanceID; RunRefineryFold must back it
// with a hidden host instance instead of failing Spawn's "project workflow
// not initialized" gate. The build-time model rejection (empty cli_model)
// proves the spawn got past instance resolution.
func TestRunRefineryFold_NoWorkflowInstance_UsesHostInstance(t *testing.T) {
	env := setupRefineryFoldTestEnv(t)
	defer env.cleanup()

	_, err := env.spawner.RunRefineryFold(context.Background(), types.RefineryFoldRequest{
		ProjectID: env.projectID,
		SessionID: "console-sess-1",
		ModelID:   "gpt-5.3-codex",
		Provider:  "openai",
		UserText:  "digest this",
	})
	if err == nil {
		t.Fatal("RunRefineryFold() returned nil error; want a provider-build error")
	}
	if !errors.Is(err, types.ErrRefineryFoldProviderBuild) {
		t.Errorf("error = %v, want errors.Is(err, types.ErrRefineryFoldProviderBuild) — an instance-gate failure would not wrap it", err)
	}
}
