package spawner

import (
	"context"
	"syscall"
	"testing"
	"time"
)

// mockBackend is a minimal ExecutionBackend stub for testing backend name detection.
type mockBackend struct {
	name string
}

func (m *mockBackend) Name() string                                                    { return m.name }
func (m *mockBackend) SupportsResume() bool                                            { return false }
func (m *mockBackend) SupportsTakeControl() bool                                       { return false }
func (m *mockBackend) Start(_ context.Context, _ *processInfo, _ *prepResult) error   { return nil }
func (m *mockBackend) Kill(_ context.Context, _ *processInfo, _ syscall.Signal) error { return nil }

// insertAgentMessage adds a single message row to agent_messages for the given session.
func (env *contextSaveTestEnv) insertAgentMessage(t *testing.T, sessionID, content string) {
	t.Helper()
	_, err := env.database.Exec(
		`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, 1, ?, ?)`,
		sessionID, content, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatalf("insert agent_message: %v", err)
	}
}

// clearContextSaverCLIPrompt sets the CLI context-saver's prompt to empty so
// that when the ephemeral spawner loads the template it fails fast with
// "empty prompt" rather than actually starting a real claude process.
func (env *contextSaveTestEnv) clearContextSaverCLIPrompt(t *testing.T) {
	t.Helper()
	_, err := env.database.Exec(
		`UPDATE system_agent_definitions SET prompt = '' WHERE id = 'context-saver'`,
	)
	if err != nil {
		t.Fatalf("clear context-saver prompt: %v", err)
	}
}

// TestSpawnContextSaver_APIBackend_MigrationSeededAPIVariant verifies that
// migration 000063 seeded a context-saver-api row with role="context-saver"
// and execution_mode="api", enabling GetForBackend to find it when backend="api".
func TestSpawnContextSaver_APIBackend_MigrationSeededAPIVariant(t *testing.T) {
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	var executionMode, role string
	err := env.database.QueryRow(
		`SELECT execution_mode, role FROM system_agent_definitions WHERE id = 'context-saver-api'`,
	).Scan(&executionMode, &role)
	if err != nil {
		t.Fatalf("context-saver-api row not found after migration: %v", err)
	}
	if executionMode != "api" {
		t.Errorf("context-saver-api execution_mode = %q, want %q", executionMode, "api")
	}
	if role != "context-saver" {
		t.Errorf("context-saver-api role = %q, want %q", role, "context-saver")
	}
}

// TestSpawnContextSaver_APIBackend_AttemptsAPIVariant verifies that when the
// process backend is "api", spawnContextSaver selects the context-saver-api
// variant (execution_mode="api") and fails at API key resolution rather than
// at the variant-lookup step. The "api mode" error prefix is the observable
// signal that the API path was entered.
func TestSpawnContextSaver_APIBackend_AttemptsAPIVariant(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "") // no credentials → spawn fails at API key resolution
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	env.insertAgentMessage(t, sessionID, "implementing the feature")

	proc := &processInfo{
		sessionID: sessionID,
		agentType: "implementor",
		backend:   &mockBackend{name: "api"},
	}
	// Returns false because API key is missing; no panic.
	got := env.spawner.spawnContextSaver(context.Background(), proc, SpawnRequest{
		ProjectID:          env.projectID,
		WorkflowInstanceID: env.wfiID,
	})
	if got {
		t.Errorf("spawnContextSaver() = true, want false when no API credentials configured")
	}
}

// TestSpawnContextSaver_FallbackOnMissingAPIRow verifies that when there is no
// context-saver-api row, spawnContextSaver falls back to the default CLI
// context-saver without crashing. The CLI row's prompt is cleared so the
// ephemeral spawn fails fast at template loading (no real claude process).
func TestSpawnContextSaver_FallbackOnMissingAPIRow(t *testing.T) {
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	// Remove the API-mode variant so GetForBackend returns sql.ErrNoRows.
	if _, err := env.database.Exec(
		`DELETE FROM system_agent_definitions WHERE id = 'context-saver-api'`,
	); err != nil {
		t.Fatalf("delete context-saver-api: %v", err)
	}
	// Clear CLI context-saver prompt so loadTemplate fails fast (avoids real claude).
	env.clearContextSaverCLIPrompt(t)

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	env.insertAgentMessage(t, sessionID, "doing work")

	proc := &processInfo{
		sessionID: sessionID,
		agentType: "implementor",
		backend:   &mockBackend{name: "api"}, // api backend, but no api variant
	}
	// Fallback selects CLI context-saver; spawn fails at template loading (empty prompt).
	got := env.spawner.spawnContextSaver(context.Background(), proc, SpawnRequest{
		ProjectID:          env.projectID,
		WorkflowInstanceID: env.wfiID,
	})
	if got {
		t.Errorf("spawnContextSaver() = true, want false when fallback spawn fails")
	}
}

// TestSpawnContextSaver_FallbackBothRowsMissing verifies that when both
// context-saver and context-saver-api rows are absent, spawnContextSaver
// returns false immediately at the lookup step without panicking.
func TestSpawnContextSaver_FallbackBothRowsMissing(t *testing.T) {
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	if _, err := env.database.Exec(
		`DELETE FROM system_agent_definitions WHERE id IN ('context-saver', 'context-saver-api')`,
	); err != nil {
		t.Fatalf("delete rows: %v", err)
	}

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	env.insertAgentMessage(t, sessionID, "doing work")

	proc := &processInfo{
		sessionID: sessionID,
		agentType: "implementor",
		backend:   &mockBackend{name: "api"},
	}
	got := env.spawner.spawnContextSaver(context.Background(), proc, SpawnRequest{
		ProjectID:          env.projectID,
		WorkflowInstanceID: env.wfiID,
	})
	if got {
		t.Errorf("spawnContextSaver() = true, want false when no agent rows found")
	}
}

// TestSpawnContextSaver_CLIBackend_Regression verifies the CLI backend path:
// GetForBackend("context-saver","cli") selects the CLI row and the ephemeral
// spawn fails fast at template loading (empty prompt prevents real claude launch).
func TestSpawnContextSaver_CLIBackend_Regression(t *testing.T) {
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	// Clear CLI prompt so loadTemplate fails fast without spawning a real process.
	env.clearContextSaverCLIPrompt(t)

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	env.insertAgentMessage(t, sessionID, "implementing feature X")

	proc := &processInfo{
		sessionID: sessionID,
		agentType: "implementor",
		backend:   &mockBackend{name: "cli"},
	}
	// CLI path: variant found, spawn fails at empty prompt (not at missing binary).
	got := env.spawner.spawnContextSaver(context.Background(), proc, SpawnRequest{
		ProjectID:          env.projectID,
		WorkflowInstanceID: env.wfiID,
	})
	if got {
		t.Errorf("spawnContextSaver() = true, want false when CLI spawn fails")
	}
}

// TestSpawnContextSaver_NilBackend_DefaultsCLI verifies that a nil backend
// defaults to the "cli" name and selects the CLI context-saver row.
// The empty CLI prompt causes fast failure at template loading.
func TestSpawnContextSaver_NilBackend_DefaultsCLI(t *testing.T) {
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	env.clearContextSaverCLIPrompt(t)

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	env.insertAgentMessage(t, sessionID, "some work done")

	proc := &processInfo{
		sessionID: sessionID,
		agentType: "implementor",
		backend:   nil, // nil → defaults to "cli"
	}
	got := env.spawner.spawnContextSaver(context.Background(), proc, SpawnRequest{
		ProjectID:          env.projectID,
		WorkflowInstanceID: env.wfiID,
	})
	if got {
		t.Errorf("spawnContextSaver() = true, want false when CLI spawn fails")
	}
}
