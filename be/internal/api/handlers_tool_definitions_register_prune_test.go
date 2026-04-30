package api

import (
	"net/http"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
)

// mustSeedAgentDef inserts project, workflow, and agent_definition rows with the given tools CSV.
// Uses INSERT OR IGNORE for project/workflow so repeated calls within the same test are safe.
func mustSeedAgentDef(t *testing.T, s *Server, projectID, workflowID, id, toolsCSV string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err := s.pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?,?,?,?,?)`,
		projectID, "test", "/tmp", now, now,
	); err != nil {
		t.Fatalf("mustSeedAgentDef: insert project: %v", err)
	}
	if _, err := s.pool.Exec(
		`INSERT OR IGNORE INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		projectID, workflowID, "", "ticket", now, now,
	); err != nil {
		t.Fatalf("mustSeedAgentDef: insert workflow: %v", err)
	}

	r := repo.NewAgentDefinitionRepo(s.pool, s.clock)
	if err := r.Create(&model.AgentDefinition{
		ID:         id,
		ProjectID:  projectID,
		WorkflowID: workflowID,
		Model:      "sonnet",
		Timeout:    300,
		Layer:      0,
		Tools:      toolsCSV,
	}); err != nil {
		t.Fatalf("mustSeedAgentDef: %v", err)
	}
}

// TestHandleRegisterToolDefinitions_Validation covers all entry-level validation failures.
func TestHandleRegisterToolDefinitions_Validation(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)

	cases := []struct {
		name string
		body string
	}{
		{
			"missing_name",
			`{"tools":[{"endpoint":"http://x","input_schema":{"type":"object"}}]}`,
		},
		{
			"missing_endpoint",
			`{"tools":[{"name":"t","input_schema":{"type":"object"}}]}`,
		},
		{
			"missing_input_schema",
			`{"tools":[{"name":"t","endpoint":"http://x"}]}`,
		},
		{
			"null_input_schema",
			`{"tools":[{"name":"t","endpoint":"http://x","input_schema":null}]}`,
		},
		{
			"invalid_auth_method",
			`{"tools":[{"name":"t","endpoint":"http://x","input_schema":{},"auth_method":"magic"}]}`,
		},
		{
			"bearer_env_without_auth_ref",
			`{"tools":[{"name":"t","endpoint":"http://x","input_schema":{},"auth_method":"bearer_env"}]}`,
		},
		{
			"bearer_secret_ref_without_auth_ref",
			`{"tools":[{"name":"t","endpoint":"http://x","input_schema":{},"auth_method":"bearer_secret_ref"}]}`,
		},
		{
			"negative_timeout_sec",
			`{"tools":[{"name":"t","endpoint":"http://x","input_schema":{},"timeout_sec":-1}]}`,
		},
		{
			"duplicate_name",
			`{"tools":[` +
				`{"name":"dup","endpoint":"http://x/1","input_schema":{}},` +
				`{"name":"dup","endpoint":"http://x/2","input_schema":{}}` +
				`]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := doRegister(t, s, "Bearer secret", tc.body)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("[%s] status = %d, want 400; body=%s", tc.name, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestHandleRegisterToolDefinitions_Prune verifies tools absent from the payload are deleted.
func TestHandleRegisterToolDefinitions_Prune(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)

	seed := `{"tools":[
		{"name":"alpha","endpoint":"http://x/a","input_schema":{"type":"object"}},
		{"name":"beta","endpoint":"http://x/b","input_schema":{"type":"object"}}
	]}`
	if rr := doRegister(t, s, "Bearer secret", seed); rr.Code != http.StatusOK {
		t.Fatalf("seed register status = %d; body=%s", rr.Code, rr.Body.String())
	}

	// Re-register with only alpha — beta should be pruned.
	body := `{"tools":[{"name":"alpha","endpoint":"http://x/a","input_schema":{"type":"object"}}]}`
	rr := doRegister(t, s, "Bearer secret", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("register status = %d; body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeRegisterResp(t, rr)
	if resp.ToolsPruned != 1 {
		t.Errorf("tools_pruned = %d, want 1 (beta pruned)", resp.ToolsPruned)
	}
	if len(resp.ToolsSkippedInUse) != 0 {
		t.Errorf("tools_skipped_in_use = %v, want []", resp.ToolsSkippedInUse)
	}

	// Confirm beta is no longer in the global list.
	toolDefRepo := repo.NewToolDefinitionRepo(s.pool, s.clock)
	globals, err := toolDefRepo.ListGlobalRegistered()
	if err != nil {
		t.Fatalf("ListGlobalRegistered: %v", err)
	}
	for _, d := range globals {
		if d.Name == "beta" {
			t.Errorf("beta was pruned but still present in global list")
		}
	}
}

// TestHandleRegisterToolDefinitions_PruneProtect_Literal verifies a literal tool name in
// agent_definitions.tools prevents the tool from being pruned.
func TestHandleRegisterToolDefinitions_PruneProtect_Literal(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)

	// Seed "alpha" as a global tool.
	if rr := doRegister(t, s, "Bearer secret",
		`{"tools":[{"name":"alpha","endpoint":"http://x/alpha","input_schema":{"type":"object"}}]}`); rr.Code != http.StatusOK {
		t.Fatalf("seed status = %d; body=%s", rr.Code, rr.Body.String())
	}

	// Agent definition has tools="alpha" — exact literal match.
	mustSeedAgentDef(t, s, "proj", "wf", "impl", "alpha")

	// Register "beta" only; alpha should be skipped (protected), not pruned.
	rr := doRegister(t, s, "Bearer secret",
		`{"tools":[{"name":"beta","endpoint":"http://x/beta","input_schema":{"type":"object"}}]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("register status = %d; body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeRegisterResp(t, rr)
	if resp.ToolsPruned != 0 {
		t.Errorf("tools_pruned = %d, want 0 (alpha protected by literal pattern)", resp.ToolsPruned)
	}
	if len(resp.ToolsSkippedInUse) != 1 || resp.ToolsSkippedInUse[0] != "alpha" {
		t.Errorf("tools_skipped_in_use = %v, want [alpha]", resp.ToolsSkippedInUse)
	}
}

// TestHandleRegisterToolDefinitions_PruneProtect_Glob verifies a prefix glob pattern in
// agent_definitions.tools prevents matching global tools from being pruned.
func TestHandleRegisterToolDefinitions_PruneProtect_Glob(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)

	// Seed "git_commit" as a global tool.
	if rr := doRegister(t, s, "Bearer secret",
		`{"tools":[{"name":"git_commit","endpoint":"http://x/git_commit","input_schema":{"type":"object"}}]}`); rr.Code != http.StatusOK {
		t.Fatalf("seed status = %d; body=%s", rr.Code, rr.Body.String())
	}

	// Agent definition uses glob pattern "git_*".
	mustSeedAgentDef(t, s, "proj", "wf", "impl", "git_*")

	// Register "other_tool" only; git_commit matches "git_*" and should be protected.
	rr := doRegister(t, s, "Bearer secret",
		`{"tools":[{"name":"other_tool","endpoint":"http://x/other","input_schema":{"type":"object"}}]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("register status = %d; body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeRegisterResp(t, rr)
	if resp.ToolsPruned != 0 {
		t.Errorf("tools_pruned = %d, want 0 (git_commit protected by git_*)", resp.ToolsPruned)
	}
	found := false
	for _, name := range resp.ToolsSkippedInUse {
		if name == "git_commit" {
			found = true
		}
	}
	if !found {
		t.Errorf("tools_skipped_in_use = %v, want git_commit listed", resp.ToolsSkippedInUse)
	}
}

// TestHandleRegisterToolDefinitions_PruneProtect_Wildcard verifies the "*" pattern in
// agent_definitions.tools prevents all global tools from being pruned.
func TestHandleRegisterToolDefinitions_PruneProtect_Wildcard(t *testing.T) {
	t.Setenv("NRFLO_REGISTER_TOKEN", "secret")
	s := newToolDefServer(t)

	// Seed "existing_tool" globally.
	if rr := doRegister(t, s, "Bearer secret",
		`{"tools":[{"name":"existing_tool","endpoint":"http://x/et","input_schema":{"type":"object"}}]}`); rr.Code != http.StatusOK {
		t.Fatalf("seed status = %d; body=%s", rr.Code, rr.Body.String())
	}

	// Agent definition uses wildcard "*" — matches everything.
	mustSeedAgentDef(t, s, "proj", "wf", "impl", "*")

	// Register "new_tool" only; existing_tool should be protected by "*".
	rr := doRegister(t, s, "Bearer secret",
		`{"tools":[{"name":"new_tool","endpoint":"http://x/new","input_schema":{"type":"object"}}]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("register status = %d; body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeRegisterResp(t, rr)
	if resp.ToolsPruned != 0 {
		t.Errorf("tools_pruned = %d, want 0 (existing_tool protected by *)", resp.ToolsPruned)
	}
	if len(resp.ToolsSkippedInUse) != 1 || resp.ToolsSkippedInUse[0] != "existing_tool" {
		t.Errorf("tools_skipped_in_use = %v, want [existing_tool]", resp.ToolsSkippedInUse)
	}
}
