package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// TestSystemAgentDef_ListForAPI_ExcludesAPIMode verifies that ListForAPI(false)
// hides execution_mode='api' rows (e.g., context-saver-api) while still returning
// cli-mode rows.
func TestSystemAgentDef_ListForAPI_ExcludesAPIMode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "listforapi_exclude.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	svc := NewSystemAgentDefinitionService(pool, clock.Real())

	defs, err := svc.ListForAPI(false)
	if err != nil {
		t.Fatalf("ListForAPI(false): %v", err)
	}

	// No execution_mode='api' rows should appear.
	for _, d := range defs {
		if d.ExecutionMode == "api" {
			t.Errorf("ListForAPI(false) returned api-mode row: id=%q execution_mode=%q", d.ID, d.ExecutionMode)
		}
	}

	// The seeded CLI context-saver must still be present.
	var foundCLI bool
	for _, d := range defs {
		if d.ID == "context-saver" {
			foundCLI = true
			break
		}
	}
	if !foundCLI {
		t.Errorf("ListForAPI(false) excluded cli context-saver; got %d defs", len(defs))
	}
}

// TestSystemAgentDef_ListForAPI_IncludesAPIMode verifies that ListForAPI(true)
// returns all rows including execution_mode='api', so the seeded context-saver-api
// row is visible when the server is in api-mode.
func TestSystemAgentDef_ListForAPI_IncludesAPIMode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "listforapi_include.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	svc := NewSystemAgentDefinitionService(pool, clock.Real())

	defs, err := svc.ListForAPI(true)
	if err != nil {
		t.Fatalf("ListForAPI(true): %v", err)
	}

	var foundCLI, foundAPI bool
	for _, d := range defs {
		switch d.ID {
		case "context-saver":
			foundCLI = true
		case "context-saver-api":
			foundAPI = true
		}
	}
	if !foundCLI {
		t.Errorf("ListForAPI(true) missing cli context-saver; got %d defs", len(defs))
	}
	if !foundAPI {
		t.Errorf("ListForAPI(true) missing api context-saver-api; got %d defs", len(defs))
	}
}
