package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupWorkflowSeedTestEnv creates an isolated DB for seed-findings service tests.
func setupWorkflowSeedTestEnv(t *testing.T) (*WorkflowService, *db.Pool) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "wf_seed_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	clk := clock.NewTest(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))
	return NewWorkflowService(pool, clk), pool
}

// seedSvcProject inserts a minimal project row via ProjectService.
func seedSvcProject(t *testing.T, pool *db.Pool, projectID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seedSvcProject(%q): %v", projectID, err)
	}
}

// seedTicketSvc inserts a minimal ticket row.
func seedTicketSvc(t *testing.T, pool *db.Pool, projectID, ticketID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT OR IGNORE INTO tickets (id, project_id, title, status, priority, issue_type, created_at, updated_at, created_by)
		 VALUES (?, ?, 'Test ticket', 'open', 2, 'task', ?, ?, 'test')`,
		ticketID, projectID, now, now,
	); err != nil {
		t.Fatalf("seedTicketSvc(%q/%q): %v", projectID, ticketID, err)
	}
}

// seedWorkflowDef inserts a workflow definition row directly.
func seedWorkflowDef(t *testing.T, pool *db.Pool, projectID, workflowID, scopeType string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, created_at, updated_at)
		 VALUES (?, ?, '', ?, '[]', 1, ?, ?)`,
		workflowID, projectID, scopeType, now, now,
	); err != nil {
		t.Fatalf("seedWorkflowDef(%q/%q, scope=%q): %v", projectID, workflowID, scopeType, err)
	}
}

// findingsFromJSON unmarshals a JSON string into a map[string]string.
func findingsFromJSON(t *testing.T, raw string) map[string]string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal findings %q: %v", raw, err)
	}
	return m
}

func TestWorkflowService_Init_SeedFindings(t *testing.T) {
	t.Parallel()
	svc, pool := setupWorkflowSeedTestEnv(t)

	seedSvcProject(t, pool, "proj-seed-ticket")
	seedTicketSvc(t, pool, "proj-seed-ticket", "tkt-seed")
	seedWorkflowDef(t, pool, "proj-seed-ticket", "feature", "ticket")

	req := &types.WorkflowInitRequest{
		Workflow:     "feature",
		SeedFindings: map[string]string{"foo": "bar", "spec_url": "https://example.com"},
	}

	wi, err := svc.Init("proj-seed-ticket", "tkt-seed", req)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	findings := findingsFromJSON(t, wi.Findings)
	if findings["foo"] != "bar" {
		t.Errorf("findings[foo] = %q, want %q", findings["foo"], "bar")
	}
	if findings["spec_url"] != "https://example.com" {
		t.Errorf("findings[spec_url] = %q, want %q", findings["spec_url"], "https://example.com")
	}
}

func TestWorkflowService_Init_EmptySeedFindings(t *testing.T) {
	t.Parallel()
	svc, pool := setupWorkflowSeedTestEnv(t)

	seedSvcProject(t, pool, "proj-empty-seed")
	seedTicketSvc(t, pool, "proj-empty-seed", "tkt-empty")
	seedWorkflowDef(t, pool, "proj-empty-seed", "feature", "ticket")

	cases := []struct {
		name string
		seed map[string]string
	}{
		{"nil seed", nil},
		{"empty map", map[string]string{}},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			// Use distinct tickets per sub-test to avoid conflicts
			ticketID := "tkt-empty-" + c.name
			seedTicketSvc(t, pool, "proj-empty-seed", ticketID)

			wi, err := svc.Init("proj-empty-seed", ticketID, &types.WorkflowInitRequest{
				Workflow:     "feature",
				SeedFindings: c.seed,
			})
			if err != nil {
				t.Fatalf("Init(%s): %v", c.name, err)
			}
			if wi.Findings != "{}" {
				t.Errorf("Findings = %q, want %q", wi.Findings, "{}")
			}
		})
	}
}

func TestWorkflowService_InitProjectWorkflow_SeedFindings(t *testing.T) {
	t.Parallel()
	svc, pool := setupWorkflowSeedTestEnv(t)

	seedSvcProject(t, pool, "proj-seed-proj")
	seedWorkflowDef(t, pool, "proj-seed-proj", "analysis", "project")

	req := &types.ProjectWorkflowRunRequest{
		Workflow:     "analysis",
		SeedFindings: map[string]string{"import_id": "spec-42", "source": "github"},
	}

	wi, err := svc.InitProjectWorkflow("proj-seed-proj", req)
	if err != nil {
		t.Fatalf("InitProjectWorkflow: %v", err)
	}

	if wi.ScopeType != "project" {
		t.Errorf("ScopeType = %q, want %q", wi.ScopeType, "project")
	}

	findings := findingsFromJSON(t, wi.Findings)
	if findings["import_id"] != "spec-42" {
		t.Errorf("findings[import_id] = %q, want %q", findings["import_id"], "spec-42")
	}
	if findings["source"] != "github" {
		t.Errorf("findings[source] = %q, want %q", findings["source"], "github")
	}
}

func TestWorkflowService_InitProjectWorkflow_EmptySeedFindings(t *testing.T) {
	t.Parallel()
	svc, pool := setupWorkflowSeedTestEnv(t)

	seedSvcProject(t, pool, "proj-proj-empty")
	seedWorkflowDef(t, pool, "proj-proj-empty", "analysis", "project")

	wi, err := svc.InitProjectWorkflow("proj-proj-empty", &types.ProjectWorkflowRunRequest{
		Workflow:     "analysis",
		SeedFindings: nil,
	})
	if err != nil {
		t.Fatalf("InitProjectWorkflow: %v", err)
	}
	if wi.Findings != "{}" {
		t.Errorf("Findings = %q, want %q", wi.Findings, "{}")
	}
}

func TestWorkflowService_InitProjectWorkflow_RejectsTicketScope(t *testing.T) {
	t.Parallel()
	svc, pool := setupWorkflowSeedTestEnv(t)

	seedSvcProject(t, pool, "proj-scope-check")
	seedWorkflowDef(t, pool, "proj-scope-check", "bugfix", "ticket")

	_, err := svc.InitProjectWorkflow("proj-scope-check", &types.ProjectWorkflowRunRequest{
		Workflow: "bugfix",
	})
	if err == nil {
		t.Errorf("InitProjectWorkflow with ticket-scoped workflow: expected error, got nil")
	}
}
