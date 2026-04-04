package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupCLIModelTestEnv creates an isolated DB copy for CLI model tests.
func setupCLIModelTestEnv(t *testing.T) (*CLIModelService, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "cli_model_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	svc := NewCLIModelService(pool, clock.Real())
	return svc, func() { pool.Close() }
}

// strPtr is a convenience for creating *string values in tests.
func strPtr(v string) *string { return &v }

// --- List ---

func TestCLIModel_List(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	models, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(models) != 11 {
		t.Fatalf("List len = %d, want 11", len(models))
	}

	// Verify ORDER BY id ascending — first and last entries.
	if models[0].ID != "codex_gpt54_high" {
		t.Errorf("List[0].ID = %q, want %q", models[0].ID, "codex_gpt54_high")
	}
	if models[10].ID != "sonnet" {
		t.Errorf("List[10].ID = %q, want %q", models[10].ID, "sonnet")
	}

	// All seeded models are read-only.
	for _, m := range models {
		if !m.ReadOnly {
			t.Errorf("seeded model %q: ReadOnly = false, want true", m.ID)
		}
	}
}

// --- Get ---

func TestCLIModel_Get(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Get("opus")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if m.ID != "opus" {
		t.Errorf("ID = %q, want %q", m.ID, "opus")
	}
	if m.CLIType != "claude" {
		t.Errorf("CLIType = %q, want %q", m.CLIType, "claude")
	}
	if m.MappedModel != "opus" {
		t.Errorf("MappedModel = %q, want %q", m.MappedModel, "opus")
	}
	if m.ContextLength != 200000 {
		t.Errorf("ContextLength = %d, want 200000", m.ContextLength)
	}
	if !m.ReadOnly {
		t.Error("ReadOnly = false, want true")
	}
}

func TestCLIModel_GetCaseInsensitive(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Get("OPUS")
	if err != nil {
		t.Fatalf("Get with uppercase: %v", err)
	}
	if m.ID != "opus" {
		t.Errorf("ID = %q, want %q", m.ID, "opus")
	}
}

func TestCLIModel_GetNotFound(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	_, err := svc.Get("nonexistent-model")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

// --- Create ---

func TestCLIModel_Create(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Create(types.CLIModelCreateRequest{
		ID:              "my-model",
		CLIType:         "claude",
		DisplayName:     "My Model",
		MappedModel:     "claude-3-5-sonnet",
		ReasoningEffort: "",
		ContextLength:   100000,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if m.ID != "my-model" {
		t.Errorf("ID = %q, want %q", m.ID, "my-model")
	}
	if m.CLIType != "claude" {
		t.Errorf("CLIType = %q, want %q", m.CLIType, "claude")
	}
	if m.DisplayName != "My Model" {
		t.Errorf("DisplayName = %q, want %q", m.DisplayName, "My Model")
	}
	if m.MappedModel != "claude-3-5-sonnet" {
		t.Errorf("MappedModel = %q, want %q", m.MappedModel, "claude-3-5-sonnet")
	}
	if m.ContextLength != 100000 {
		t.Errorf("ContextLength = %d, want 100000", m.ContextLength)
	}
	if m.ReadOnly {
		t.Error("ReadOnly = true, want false for user-created model")
	}
	if m.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if m.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}

	// Round-trip via Get.
	got, err := svc.Get("my-model")
	if err != nil {
		t.Fatalf("Get after Create: %v", err)
	}
	if got.ReadOnly {
		t.Error("Get ReadOnly = true, want false")
	}
}

func TestCLIModel_CreateInvalidCLIType(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	_, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "bad-type",
		CLIType:     "invalid",
		DisplayName: "Bad",
		MappedModel: "something",
	})
	if err == nil {
		t.Fatal("expected error for invalid cli_type, got nil")
	}
	if !strings.Contains(err.Error(), "invalid cli_type") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "invalid cli_type")
	}
}

func TestCLIModel_CreateMissingID(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	_, err := svc.Create(types.CLIModelCreateRequest{
		CLIType:     "claude",
		DisplayName: "No ID",
		MappedModel: "sonnet",
	})
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "required")
	}
}

func TestCLIModel_CreateMissingDisplayName(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	_, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "nodisplay",
		CLIType:     "claude",
		MappedModel: "sonnet",
	})
	if err == nil {
		t.Fatal("expected error for missing display_name, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "required")
	}
}

func TestCLIModel_CreateMissingMappedModel(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	_, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "nomap",
		CLIType:     "claude",
		DisplayName: "No Map",
	})
	if err == nil {
		t.Fatal("expected error for missing mapped_model, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "required")
	}
}

func TestCLIModel_CreateDuplicate(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	req := types.CLIModelCreateRequest{
		ID:          "dup-model",
		CLIType:     "codex",
		DisplayName: "Dup",
		MappedModel: "gpt-4",
	}
	if _, err := svc.Create(req); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(req)
	if err == nil {
		t.Fatal("expected duplicate error, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "already exists")
	}
}

func TestCLIModel_CreateContextLengthDefault(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "default-ctx",
		CLIType:     "opencode",
		DisplayName: "Default Context",
		MappedModel: "openai/gpt-4",
		// ContextLength = 0 → should default to 200000
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.ContextLength != 200000 {
		t.Errorf("ContextLength = %d, want 200000 (default)", m.ContextLength)
	}
}

func TestCLIModel_CreateIDNormalized(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "MyCustomModel",
		CLIType:     "claude",
		DisplayName: "Custom",
		MappedModel: "sonnet",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.ID != "mycustommodel" {
		t.Errorf("ID = %q, want %q (lowercased)", m.ID, "mycustommodel")
	}
}

// --- Update ---

func TestCLIModel_Update(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "upd-model",
		CLIType:     "claude",
		DisplayName: "Original Name",
		MappedModel: "original-model",
		ContextLength: 50000,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newName := "Updated Name"
	updated, err := svc.Update("upd-model", types.CLIModelUpdateRequest{
		DisplayName: &newName,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.DisplayName != "Updated Name" {
		t.Errorf("DisplayName = %q, want %q", updated.DisplayName, "Updated Name")
	}
	// Other fields unchanged.
	if updated.MappedModel != "original-model" {
		t.Errorf("MappedModel = %q after partial update, want %q", updated.MappedModel, "original-model")
	}
	if updated.ContextLength != 50000 {
		t.Errorf("ContextLength = %d after partial update, want 50000", updated.ContextLength)
	}
}

func TestCLIModel_UpdateMultipleFields(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "multi-upd",
		CLIType:     "codex",
		DisplayName: "Old Name",
		MappedModel: "old-model",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newName := "New Name"
	newModel := "new-model"
	newCtx := 500000
	updated, err := svc.Update("multi-upd", types.CLIModelUpdateRequest{
		DisplayName:   &newName,
		MappedModel:   &newModel,
		ContextLength: &newCtx,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.DisplayName != "New Name" {
		t.Errorf("DisplayName = %q, want %q", updated.DisplayName, "New Name")
	}
	if updated.MappedModel != "new-model" {
		t.Errorf("MappedModel = %q, want %q", updated.MappedModel, "new-model")
	}
	if updated.ContextLength != 500000 {
		t.Errorf("ContextLength = %d, want 500000", updated.ContextLength)
	}
}

func TestCLIModel_UpdateNotFound(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	newName := "Whatever"
	_, err := svc.Update("nonexistent-model", types.CLIModelUpdateRequest{
		DisplayName: &newName,
	})
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

func TestCLIModel_UpdateReadonly(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	// Readonly models CAN be updated (only delete is blocked).
	newName := "My Opus"
	updated, err := svc.Update("opus", types.CLIModelUpdateRequest{
		DisplayName: &newName,
	})
	if err != nil {
		t.Fatalf("Update readonly model: %v", err)
	}
	if updated.DisplayName != "My Opus" {
		t.Errorf("DisplayName = %q, want %q", updated.DisplayName, "My Opus")
	}
	// Still read-only.
	if !updated.ReadOnly {
		t.Error("ReadOnly = false after update, want true")
	}
}

func TestCLIModel_UpdateNoFields(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "noop-model",
		CLIType:     "claude",
		DisplayName: "Noop",
		MappedModel: "sonnet",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Empty update should succeed and return current model.
	got, err := svc.Update("noop-model", types.CLIModelUpdateRequest{})
	if err != nil {
		t.Fatalf("empty Update: %v", err)
	}
	if got.DisplayName != "Noop" {
		t.Errorf("DisplayName = %q after no-op update, want %q", got.DisplayName, "Noop")
	}
}
