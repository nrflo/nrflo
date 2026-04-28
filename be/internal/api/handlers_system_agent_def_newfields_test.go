package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// newSystemAgentServerWithSeeds creates a Server with all migration-seeded rows intact.
// Unlike newSystemAgentServer, this does NOT delete the seeded system agent definitions.
func newSystemAgentServerWithSeeds(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sys_agent_handler_seeds_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

// findDefByID scans a slice of definitions and returns the first matching ID, or nil.
func findDefByID(defs []*model.SystemAgentDefinition, id string) *model.SystemAgentDefinition {
	for _, d := range defs {
		if d.ID == id {
			return d
		}
	}
	return nil
}

// TestHandleListSystemAgentDefs_IncludesAPIRow verifies the seeded context-saver-api row
// is present in the list response with role/execution_mode/tools/api_max_iterations populated.
func TestHandleListSystemAgentDefs_IncludesAPIRow(t *testing.T) {
	s := newSystemAgentServerWithSeeds(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentDefs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	defs := decodeSystemAgentDefList(t, rr)
	if len(defs) == 0 {
		t.Fatal("expected seeded rows in list, got empty")
	}

	apiRow := findDefByID(defs, "context-saver-api")
	if apiRow == nil {
		t.Fatal("context-saver-api not found in list")
	}
	if apiRow.Role != "context-saver" {
		t.Errorf("context-saver-api Role = %q, want %q", apiRow.Role, "context-saver")
	}
	if apiRow.ExecutionMode != "api" {
		t.Errorf("context-saver-api ExecutionMode = %q, want api", apiRow.ExecutionMode)
	}
	if apiRow.Tools != "findings_add" {
		t.Errorf("context-saver-api Tools = %q, want %q", apiRow.Tools, "findings_add")
	}
	if apiRow.APIMaxIterations == nil || *apiRow.APIMaxIterations != 8 {
		t.Errorf("context-saver-api APIMaxIterations = %v, want 8", apiRow.APIMaxIterations)
	}

	// Verify that the CLI row also has role/execution_mode populated (backfill check).
	cliRow := findDefByID(defs, "context-saver")
	if cliRow == nil {
		t.Fatal("context-saver not found in list")
	}
	if cliRow.Role != "context-saver" {
		t.Errorf("context-saver Role = %q, want %q", cliRow.Role, "context-saver")
	}
	if cliRow.ExecutionMode != "cli" {
		t.Errorf("context-saver ExecutionMode = %q, want cli", cliRow.ExecutionMode)
	}
}

// TestHandleUpdateSystemAgentDef_NewFields verifies PATCH correctly persists tools and api_max_iterations.
func TestHandleUpdateSystemAgentDef_NewFields(t *testing.T) {
	s := newSystemAgentServer(t)

	// Create an API-mode row.
	createBody := `{"id":"context-saver-api","role":"context-saver","execution_mode":"api","prompt":"save it","tools":"findings_add","api_max_iterations":8}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(createBody))
	createRR := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", createRR.Code, createRR.Body.String())
	}

	// PATCH with updated tools.
	patchBody := `{"tools":"findings_add,findings_get","api_max_iterations":16}`
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/system-agents/context-saver-api", strings.NewReader(patchBody))
	patchReq.SetPathValue("id", "context-saver-api")
	patchRR := httptest.NewRecorder()
	s.handleUpdateSystemAgentDef(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200; body: %s", patchRR.Code, patchRR.Body.String())
	}

	// Verify via Get that the update persisted.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents/context-saver-api", nil)
	getReq.SetPathValue("id", "context-saver-api")
	getRR := httptest.NewRecorder()
	s.handleGetSystemAgentDef(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200; body: %s", getRR.Code, getRR.Body.String())
	}
	def := decodeSystemAgentDef(t, getRR)

	if def.Tools != "findings_add,findings_get" {
		t.Errorf("Tools = %q, want %q", def.Tools, "findings_add,findings_get")
	}
	if def.APIMaxIterations == nil || *def.APIMaxIterations != 16 {
		t.Errorf("APIMaxIterations = %v, want 16", def.APIMaxIterations)
	}
	// Prompt unchanged.
	if def.Prompt != "save it" {
		t.Errorf("Prompt = %q, want %q (should be unchanged)", def.Prompt, "save it")
	}
	// Role unchanged.
	if def.Role != "context-saver" {
		t.Errorf("Role = %q, want %q (should be unchanged)", def.Role, "context-saver")
	}
}

// TestHandleCreate_InvalidExecutionMode_400 verifies invalid execution_mode returns HTTP 400.
func TestHandleCreate_InvalidExecutionMode_400(t *testing.T) {
	s := newSystemAgentServer(t)

	body := `{"id":"bad-mode","prompt":"p","execution_mode":"grpc"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "invalid execution_mode")
}

// TestHandleUpdate_InvalidExecutionMode_400 verifies invalid execution_mode on PATCH returns HTTP 400.
func TestHandleUpdate_InvalidExecutionMode_400(t *testing.T) {
	s := newSystemAgentServer(t)

	// Create a valid agent first.
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents",
		strings.NewReader(`{"id":"mode-agent","prompt":"p"}`))
	s.handleCreateSystemAgentDef(httptest.NewRecorder(), createReq)

	// PATCH with invalid mode.
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/system-agents/mode-agent",
		strings.NewReader(`{"execution_mode":"socket"}`))
	patchReq.SetPathValue("id", "mode-agent")
	rr := httptest.NewRecorder()
	s.handleUpdateSystemAgentDef(rr, patchReq)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "invalid execution_mode")
}

// TestHandleCreate_DuplicateRoleMode_Conflict verifies the (role, execution_mode) unique index
// causes a 409 when a second agent shares the same role+mode pair.
func TestHandleCreate_DuplicateRoleMode_Conflict(t *testing.T) {
	s := newSystemAgentServer(t)

	// First row with role=shared-role, execution_mode=cli.
	body1 := `{"id":"role-agent-1","role":"shared-role","execution_mode":"cli","prompt":"p"}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(body1))
	rr1 := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(rr1, req1)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201; body: %s", rr1.Code, rr1.Body.String())
	}

	// Second row with different id but same role+mode → unique index violation → 409.
	body2 := `{"id":"role-agent-2","role":"shared-role","execution_mode":"cli","prompt":"p"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(body2))
	rr2 := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(rr2, req2)
	if rr2.Code != http.StatusConflict {
		t.Errorf("duplicate role+mode status = %d, want 409; body: %s", rr2.Code, rr2.Body.String())
	}
	assertErrorContains(t, rr2, "already exists")
}
