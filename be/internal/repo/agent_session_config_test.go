package repo

import (
	"database/sql"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func setupConfigTestDB(t *testing.T) (*db.DB, *AgentSessionRepo, string) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "config_test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj', 'Test Project', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	_, err = database.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES ('proj', 'wf1', 'Workflow', 'ticket', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	wfiID := "wfi-config-test"
	_, err = database.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
		VALUES (?, 'proj', 'TKT-1', 'wf1', 'active', 'ticket', '{}', datetime('now'), datetime('now'))`, wfiID)
	if err != nil {
		t.Fatalf("insert workflow_instance: %v", err)
	}

	r := NewAgentSessionRepo(database, clock.Real())
	return database, r, wfiID
}

func makeSession(id, wfiID, config string) *model.AgentSession {
	return &model.AgentSession{
		ID:                 id,
		ProjectID:          "proj",
		TicketID:           "TKT-1",
		WorkflowInstanceID: wfiID,
		Phase:              "phase0",
		AgentType:          "test-agent",
		ModelID:            sql.NullString{String: "sonnet", Valid: true},
		Status:             model.AgentSessionRunning,
		Config:             config,
	}
}

// TestAgentSessionCreate_ConfigPersisted verifies Config is written to DB.
func TestAgentSessionCreate_ConfigPersisted(t *testing.T) {
	_, r, wfiID := setupConfigTestDB(t)

	configJSON := `{"allowedTools":["bash"],"permissions":{"allow":["*"]}}`
	sess := makeSession("sess-cfg-1", wfiID, configJSON)

	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("sess-cfg-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Config != configJSON {
		t.Errorf("Config = %q, want %q", got.Config, configJSON)
	}
}

// TestAgentSessionCreate_EmptyConfigDefault verifies Config defaults to empty string.
func TestAgentSessionCreate_EmptyConfigDefault(t *testing.T) {
	_, r, wfiID := setupConfigTestDB(t)

	sess := makeSession("sess-cfg-empty", wfiID, "")

	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("sess-cfg-empty")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Config != "" {
		t.Errorf("Config = %q, want empty string", got.Config)
	}
}

// TestAgentSessionCreate_ConfigRoundTrip verifies multiple sessions retain their
// own Config values.
func TestAgentSessionCreate_ConfigRoundTrip(t *testing.T) {
	_, r, wfiID := setupConfigTestDB(t)

	cases := []struct {
		id     string
		config string
	}{
		{"sess-a", `{"model":"sonnet"}`},
		{"sess-b", `{"model":"haiku","safety":true}`},
		{"sess-c", ""},
	}

	for _, tc := range cases {
		sess := makeSession(tc.id, wfiID, tc.config)
		if err := r.Create(sess); err != nil {
			t.Fatalf("Create(%s): %v", tc.id, err)
		}
	}

	for _, tc := range cases {
		got, err := r.Get(tc.id)
		if err != nil {
			t.Fatalf("Get(%s): %v", tc.id, err)
		}
		if got.Config != tc.config {
			t.Errorf("Get(%s).Config = %q, want %q", tc.id, got.Config, tc.config)
		}
	}
}

// TestAgentSessionGetRunning_ConfigIncluded verifies GetRunning returns the Config field.
func TestAgentSessionGetRunning_ConfigIncluded(t *testing.T) {
	_, r, wfiID := setupConfigTestDB(t)

	configJSON := `{"permissions":{"allow":["read"]}}`
	sess := makeSession("sess-running-cfg", wfiID, configJSON)
	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	sessions, err := r.GetRunning(10)
	if err != nil {
		t.Fatalf("GetRunning: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatal("GetRunning returned no sessions")
	}

	var found *model.AgentSession
	for _, s := range sessions {
		if s.ID == "sess-running-cfg" {
			found = s
			break
		}
	}
	if found == nil {
		t.Fatal("session not found in GetRunning results")
	}
	if found.Config != configJSON {
		t.Errorf("GetRunning Config = %q, want %q", found.Config, configJSON)
	}
}

// TestAgentSessionInsertWithoutConfig_DefaultsToEmpty ensures legacy inserts
// (without explicit config column) don't break when read back.
func TestAgentSessionInsertWithoutConfig_DefaultsToEmpty(t *testing.T) {
	database, r, wfiID := setupConfigTestDB(t)

	now := "2026-01-01T00:00:00Z"
	_, err := database.Exec(`
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, 'proj', 'TKT-1', ?, 'phase0', 'test-agent', 'sonnet', 'completed', ?, ?)`,
		"sess-legacy", wfiID, now, now)
	if err != nil {
		t.Fatalf("legacy insert: %v", err)
	}

	got, err := r.Get("sess-legacy")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Config != "" {
		t.Errorf("legacy session Config = %q, want empty string", got.Config)
	}
}
