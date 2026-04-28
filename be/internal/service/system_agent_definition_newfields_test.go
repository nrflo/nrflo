package service

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"be/internal/types"
)

// TestGetForBackend_API verifies GetForBackend returns the api-mode row with all new fields.
func TestGetForBackend_API(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	def, err := svc.GetForBackend("context-saver", "api")
	if err != nil {
		t.Fatalf("GetForBackend(context-saver, api): %v", err)
	}

	if def.ID != "context-saver-api" {
		t.Errorf("ID = %q, want %q", def.ID, "context-saver-api")
	}
	if def.Role != "context-saver" {
		t.Errorf("Role = %q, want %q", def.Role, "context-saver")
	}
	if def.ExecutionMode != "api" {
		t.Errorf("ExecutionMode = %q, want %q", def.ExecutionMode, "api")
	}
	if def.Tools != "findings_add" {
		t.Errorf("Tools = %q, want %q", def.Tools, "findings_add")
	}
	if def.APIMaxIterations == nil || *def.APIMaxIterations != 8 {
		t.Errorf("APIMaxIterations = %v, want 8", def.APIMaxIterations)
	}
	if !strings.Contains(def.Prompt, "${TARGET_SESSION_ID}") {
		t.Error("Prompt does not contain ${TARGET_SESSION_ID}")
	}
	if !strings.Contains(def.Prompt, "findings_add") {
		t.Error("Prompt does not reference findings_add tool")
	}
}

// TestGetForBackend_CLI verifies GetForBackend returns the cli-mode context-saver.
func TestGetForBackend_CLI(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	def, err := svc.GetForBackend("context-saver", "cli")
	if err != nil {
		t.Fatalf("GetForBackend(context-saver, cli): %v", err)
	}

	if def.ExecutionMode != "cli" {
		t.Errorf("ExecutionMode = %q, want cli", def.ExecutionMode)
	}
	if def.Role != "context-saver" {
		t.Errorf("Role = %q, want context-saver", def.Role)
	}
	if def.Tools != "" {
		t.Errorf("Tools = %q, want empty for cli row", def.Tools)
	}
}

// TestGetForBackend_Miss verifies GetForBackend returns sql.ErrNoRows for unknown role+mode.
func TestGetForBackend_Miss(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	_, err := svc.GetForBackend("nonexistent-role", "cli")
	if err == nil {
		t.Fatal("expected sql.ErrNoRows, got nil")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}

// TestSystemAgentDef_BackfillRolePopulated verifies migration backfill set role=id for legacy rows.
func TestSystemAgentDef_BackfillRolePopulated(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	def, err := svc.Get("conflict-resolver")
	if err != nil {
		t.Fatalf("Get conflict-resolver: %v", err)
	}
	if def.Role != "conflict-resolver" {
		t.Errorf("Role = %q, want %q", def.Role, "conflict-resolver")
	}
	if def.ExecutionMode != "cli" {
		t.Errorf("ExecutionMode = %q, want cli (default)", def.ExecutionMode)
	}
}

// TestSystemAgentDef_CreateAndGet_NewFields verifies Role/ExecutionMode/Tools/APIMaxIterations round-trip.
func TestSystemAgentDef_CreateAndGet_NewFields(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	apiMax := intPtr(15)
	def, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:               "test-api-agent",
		Role:             "my-role",
		ExecutionMode:    "api",
		Prompt:           "do stuff",
		Tools:            "findings_add,findings_get",
		APIMaxIterations: apiMax,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if def.Role != "my-role" {
		t.Errorf("Create Role = %q, want %q", def.Role, "my-role")
	}
	if def.ExecutionMode != "api" {
		t.Errorf("Create ExecutionMode = %q, want api", def.ExecutionMode)
	}
	if def.Tools != "findings_add,findings_get" {
		t.Errorf("Create Tools = %q, want %q", def.Tools, "findings_add,findings_get")
	}
	if def.APIMaxIterations == nil || *def.APIMaxIterations != 15 {
		t.Errorf("Create APIMaxIterations = %v, want 15", def.APIMaxIterations)
	}

	got, err := svc.Get("test-api-agent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Role != "my-role" {
		t.Errorf("Get Role = %q, want %q", got.Role, "my-role")
	}
	if got.ExecutionMode != "api" {
		t.Errorf("Get ExecutionMode = %q, want api", got.ExecutionMode)
	}
	if got.Tools != "findings_add,findings_get" {
		t.Errorf("Get Tools = %q, want %q", got.Tools, "findings_add,findings_get")
	}
	if got.APIMaxIterations == nil || *got.APIMaxIterations != 15 {
		t.Errorf("Get APIMaxIterations = %v, want 15", got.APIMaxIterations)
	}
}

// TestSystemAgentDef_CreateDefaultRole verifies role defaults to id when not supplied.
func TestSystemAgentDef_CreateDefaultRole(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	def, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:     "no-role-agent",
		Prompt: "p",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if def.Role != "no-role-agent" {
		t.Errorf("Role = %q, want %q (defaults to id)", def.Role, "no-role-agent")
	}
	if def.ExecutionMode != "cli" {
		t.Errorf("ExecutionMode = %q, want cli (default)", def.ExecutionMode)
	}
}

// TestSystemAgentDef_Update_ToolsAndAPIMax verifies Update persists tools and api_max_iterations.
func TestSystemAgentDef_Update_ToolsAndAPIMax(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:     "upd-api-agent",
		Prompt: "p",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tools := "findings_add,findings_get"
	apiMax := intPtr(12)
	execMode := "api"
	if err := svc.Update("upd-api-agent", &types.SystemAgentDefUpdateRequest{
		Tools:            &tools,
		APIMaxIterations: apiMax,
		ExecutionMode:    &execMode,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := svc.Get("upd-api-agent")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Tools != "findings_add,findings_get" {
		t.Errorf("Tools = %q, want %q", got.Tools, "findings_add,findings_get")
	}
	if got.APIMaxIterations == nil || *got.APIMaxIterations != 12 {
		t.Errorf("APIMaxIterations = %v, want 12", got.APIMaxIterations)
	}
	if got.ExecutionMode != "api" {
		t.Errorf("ExecutionMode = %q, want api", got.ExecutionMode)
	}
}

// TestSystemAgentDef_CreateInvalidExecutionMode verifies invalid mode is rejected on Create.
func TestSystemAgentDef_CreateInvalidExecutionMode(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	_, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:            "bad-mode-agent",
		Prompt:        "p",
		ExecutionMode: "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid execution_mode, got nil")
	}
	if !strings.Contains(err.Error(), "invalid execution_mode") {
		t.Errorf("err = %q, want to contain 'invalid execution_mode'", err.Error())
	}
}

// TestSystemAgentDef_UpdateInvalidExecutionMode verifies invalid mode is rejected on Update.
func TestSystemAgentDef_UpdateInvalidExecutionMode(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:     "valid-agent",
		Prompt: "p",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	badMode := "invalid"
	err := svc.Update("valid-agent", &types.SystemAgentDefUpdateRequest{
		ExecutionMode: &badMode,
	})
	if err == nil {
		t.Fatal("expected error for invalid execution_mode, got nil")
	}
	if !strings.Contains(err.Error(), "invalid execution_mode") {
		t.Errorf("err = %q, want to contain 'invalid execution_mode'", err.Error())
	}
}

// TestSystemAgentDef_DuplicateRoleMode_Conflict verifies unique index on (role, execution_mode).
func TestSystemAgentDef_DuplicateRoleMode_Conflict(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:            "role-agent-1",
		Role:          "shared-role",
		ExecutionMode: "cli",
		Prompt:        "p",
	}); err != nil {
		t.Fatalf("Create first: %v", err)
	}

	_, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:            "role-agent-2",
		Role:          "shared-role",
		ExecutionMode: "cli",
		Prompt:        "p",
	})
	if err == nil {
		t.Fatal("expected unique-constraint error for duplicate role+mode, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("err = %q, want to contain 'already exists'", err.Error())
	}
}
