package spawner

import (
	"context"
	"testing"

	"be/internal/clock"
)

// TestSpawnContextSaver_NoPool verifies that spawnContextSaver returns false
// immediately when the spawner has no database pool configured.
func TestSpawnContextSaver_NoPool(t *testing.T) {
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

