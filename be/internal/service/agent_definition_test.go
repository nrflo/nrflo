package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupAgentDefTestEnv creates an isolated DB with a project and workflow for agent def tests.
// Returns pool, AgentDefinitionService, and workflowID.
func setupAgentDefTestEnv(t *testing.T, groups []string) (*db.Pool, *AgentDefinitionService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "agent_def_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P', '/tmp', ?, ?)`,
		"proj1", now, now); err != nil {
		t.Fatalf("project insert: %v", err)
	}

	// Build groups JSON for the workflow
	wfSvc := NewWorkflowService(pool, clock.Real())
	_, err = wfSvc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf1",
		Groups: groups,
	})
	if err != nil {
		t.Fatalf("workflow create: %v", err)
	}

	cliModelSvc := NewCLIModelService(pool, clock.Real())
	svc := NewAgentDefinitionService(pool, clock.Real(), cliModelSvc)
	return pool, svc, "wf1"
}

// --- CreateAgentDef tag validation ---

func TestCreateAgentDef_WithValidTag(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, []string{"be", "fe"})

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent1",
		Prompt: "do stuff",
		Tag:    "be",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with valid tag: %v", err)
	}
	if def.Tag != "be" {
		t.Errorf("Tag = %q, want %q", def.Tag, "be")
	}
}

func TestCreateAgentDef_WithEmptyTag(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, []string{"be", "fe"})

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-notag",
		Prompt: "do stuff",
		Tag:    "", // empty tag is optional
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with empty tag: %v", err)
	}
	if def.Tag != "" {
		t.Errorf("Tag = %q, want empty", def.Tag)
	}
}

func TestCreateAgentDef_TagNotInGroups(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, []string{"be", "fe"})

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-badtag",
		Prompt: "do stuff",
		Tag:    "docs", // not in groups
	})
	if err == nil {
		t.Fatal("expected error for tag not in groups, got nil")
	}
}

func TestCreateAgentDef_TagWithEmptyWorkflowGroups(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, []string{}) // no groups defined

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-notag2",
		Prompt: "do stuff",
		Tag:    "be", // workflow has no groups
	})
	if err == nil {
		t.Fatal("expected error when workflow has no groups and tag is set, got nil")
	}
}

func TestCreateAgentDef_NoTagWorkflowHasGroups(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, []string{"be", "fe"})

	// No tag required — agent can opt out of tagging
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "untagged-agent",
		Prompt: "do stuff",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef without tag when workflow has groups: %v", err)
	}
}

// --- GetAgentDef returns tag ---

func TestGetAgentDef_ReturnsTag(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, []string{"be", "fe"})

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "tagged-agent",
		Prompt: "do stuff",
		Tag:    "fe",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "tagged-agent")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.Tag != "fe" {
		t.Errorf("GetAgentDef Tag = %q, want %q", def.Tag, "fe")
	}
}

// --- ListAgentDefs returns tag ---

func TestListAgentDefs_ReturnsTag(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, []string{"be", "fe"})

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "list-agent",
		Prompt: "do stuff",
		Tag:    "be",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	defs, err := svc.ListAgentDefs("proj1", wfID)
	if err != nil {
		t.Fatalf("ListAgentDefs: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 agent def, got %d", len(defs))
	}
	if defs[0].Tag != "be" {
		t.Errorf("ListAgentDefs[0].Tag = %q, want %q", defs[0].Tag, "be")
	}
}

// --- UpdateAgentDef tag validation ---

func TestUpdateAgentDef_UpdatesTag(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, []string{"be", "fe"})

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "upd-agent",
		Prompt: "do stuff",
		Tag:    "be",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newTag := "fe"
	if err := svc.UpdateAgentDef("proj1", wfID, "upd-agent", &types.AgentDefUpdateRequest{
		Tag: &newTag,
	}); err != nil {
		t.Fatalf("UpdateAgentDef with valid tag: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "upd-agent")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	if def.Tag != "fe" {
		t.Errorf("after update Tag = %q, want %q", def.Tag, "fe")
	}
}

func TestUpdateAgentDef_InvalidTag(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, []string{"be", "fe"})

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "upd-agent2",
		Prompt: "do stuff",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	badTag := "docs"
	if err := svc.UpdateAgentDef("proj1", wfID, "upd-agent2", &types.AgentDefUpdateRequest{
		Tag: &badTag,
	}); err == nil {
		t.Fatal("expected error for invalid tag in update, got nil")
	}
}

func TestUpdateAgentDef_ClearsTag(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, []string{"be", "fe"})

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "upd-agent3",
		Prompt: "do stuff",
		Tag:    "be",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	emptyTag := ""
	if err := svc.UpdateAgentDef("proj1", wfID, "upd-agent3", &types.AgentDefUpdateRequest{
		Tag: &emptyTag,
	}); err != nil {
		t.Fatalf("UpdateAgentDef clearing tag: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "upd-agent3")
	if err != nil {
		t.Fatalf("GetAgentDef after clear: %v", err)
	}
	if def.Tag != "" {
		t.Errorf("after clear Tag = %q, want empty", def.Tag)
	}
}
