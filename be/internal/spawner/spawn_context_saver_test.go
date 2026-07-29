package spawner

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
)

// TestSpawnContextSaver_NoPool verifies that spawnContextSaver returns false
// immediately when the spawner has no database pool configured.
func TestSpawnContextSaver_NoPool(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()}) // no Pool set
	proc := &processInfo{
		sessionID: "test-session-id",
		agentType: "implementor",
	}
	got := sp.spawnContextSaver(context.Background(), proc, SpawnRequest{})
	if got {
		t.Errorf("spawnContextSaver() = true, want false when no pool configured")
	}
}

// TestSpawnContextSaver_SystemAgentNotFound verifies graceful fallback when the
// context-saver system agent definition is missing from the database.
func TestSpawnContextSaver_SystemAgentNotFound(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	// Remove the seeded context-saver so the lookup returns not-found.
	if _, err := env.database.Exec("DELETE FROM system_agent_definitions WHERE id = 'context-saver'"); err != nil {
		t.Fatalf("failed to delete context-saver: %v", err)
	}

	proc := &processInfo{
		sessionID: "test-session-id",
		agentType: "implementor",
	}
	got := env.spawner.spawnContextSaver(context.Background(), proc, SpawnRequest{})
	if got {
		t.Errorf("spawnContextSaver() = true, want false when context-saver agent not found")
	}
}

// TestSpawnContextSaver_NoMessages verifies graceful fallback when the session
// has no agent messages to summarize.
func TestSpawnContextSaver_NoMessages(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	// Create a session but add no messages to agent_messages.
	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})

	proc := &processInfo{
		sessionID: sessionID,
		agentType: "implementor",
	}
	got := env.spawner.spawnContextSaver(context.Background(), proc, SpawnRequest{})
	if got {
		t.Errorf("spawnContextSaver() = true, want false when no messages exist")
	}
}

// TestCopyToResumeToTarget verifies the saver-session → target-session finding
// copy: findings_add can only write to the saver's own session, so the spawner
// must move to_resume where the relaunch reader looks for it.
func TestCopyToResumeToTarget(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	targetSID := env.createSessionWithFindings(t, map[string]interface{}{})
	saverSID := env.createSessionWithFindings(t, map[string]interface{}{
		"to_resume": "implemented X, remaining Y",
	})

	env.spawner.copyToResumeToTarget(context.Background(), saverSID, targetSID)

	findings, err := repo.NewFindingRepo(env.database, clock.Real()).GetOwn("session", targetSID)
	if err != nil {
		t.Fatalf("GetOwn(target): %v", err)
	}
	var got string
	if err := json.Unmarshal(findings["to_resume"], &got); err != nil {
		t.Fatalf("to_resume missing or not a string on target session: %v", err)
	}
	if got != "implemented X, remaining Y" {
		t.Errorf("to_resume = %q, want copied summary", got)
	}
}

// TestCopyToResumeToTarget_NoSaverSession verifies a missing saver session id
// (saver never registered) is a no-op rather than a panic or spurious write.
func TestCopyToResumeToTarget_NoSaverSession(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	targetSID := env.createSessionWithFindings(t, map[string]interface{}{})
	env.spawner.copyToResumeToTarget(context.Background(), "", targetSID)

	findings, err := repo.NewFindingRepo(env.database, clock.Real()).GetOwn("session", targetSID)
	if err != nil {
		t.Fatalf("GetOwn(target): %v", err)
	}
	if _, ok := findings["to_resume"]; ok {
		t.Errorf("to_resume unexpectedly present on target session")
	}
}
