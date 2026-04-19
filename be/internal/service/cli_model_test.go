package service

import (
	"fmt"
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
	if len(models) != 13 {
		t.Fatalf("List len = %d, want 13", len(models))
	}

	// Verify ORDER BY id ascending — first and last entries.
	if models[0].ID != "codex_gpt54_high" {
		t.Errorf("List[0].ID = %q, want %q", models[0].ID, "codex_gpt54_high")
	}
	if models[12].ID != "sonnet" {
		t.Errorf("List[12].ID = %q, want %q", models[12].ID, "sonnet")
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

	m, err := svc.Get("opus_4_7")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if m.ID != "opus_4_7" {
		t.Errorf("ID = %q, want %q", m.ID, "opus_4_7")
	}
	if m.CLIType != "claude" {
		t.Errorf("CLIType = %q, want %q", m.CLIType, "claude")
	}
	if m.MappedModel != "claude-opus-4-7" {
		t.Errorf("MappedModel = %q, want %q", m.MappedModel, "claude-opus-4-7")
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

	m, err := svc.Get("OPUS_4_7")
	if err != nil {
		t.Fatalf("Get with uppercase: %v", err)
	}
	if m.ID != "opus_4_7" {
		t.Errorf("ID = %q, want %q", m.ID, "opus_4_7")
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

	// On read_only rows, only reasoning_effort may be updated — all other fields are rejected.
	newName := "My Opus"
	_, err := svc.Update("opus_4_7", types.CLIModelUpdateRequest{
		DisplayName: &newName,
	})
	if err == nil {
		t.Fatal("expected error updating display_name on read_only row, got nil")
	}
	if !strings.Contains(err.Error(), "only reasoning_effort can be updated on built-in models") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "only reasoning_effort can be updated on built-in models")
	}
	// TODO(test-writer): cover each locked field individually (mapped_model, context_length, enabled=false)
	// and happy-path reasoning_effort updates on read_only rows.
}

// --- ReasoningEffort validation ---

func TestCLIModel_CreateReasoningEffort(t *testing.T) {
	tests := []struct {
		name        string
		cliType     string
		mappedModel string
		effort      string
		wantErr     string // substring; "" means success expected
	}{
		{name: "empty effort claude sonnet", cliType: "claude", mappedModel: "claude-sonnet-4-5", effort: ""},
		{name: "low effort claude sonnet", cliType: "claude", mappedModel: "claude-sonnet-4-5", effort: "low"},
		{name: "medium effort claude opus 4.6", cliType: "claude", mappedModel: "claude-opus-4-6", effort: "medium"},
		{name: "high effort claude opus 4.7", cliType: "claude", mappedModel: "claude-opus-4-7", effort: "high"},
		{name: "max effort claude sonnet", cliType: "claude", mappedModel: "claude-sonnet-4-5", effort: "max"},
		{name: "xhigh effort claude opus 4.7", cliType: "claude", mappedModel: "claude-opus-4-7", effort: "xhigh"},
		{name: "xhigh effort claude opus 4.7 1M", cliType: "claude", mappedModel: "claude-opus-4-7[1m]", effort: "xhigh"},
		{name: "xhigh effort opencode ok", cliType: "opencode", mappedModel: "openai/gpt-5.4", effort: "xhigh"},
		{name: "xhigh effort codex ok", cliType: "codex", mappedModel: "gpt-5.3-codex", effort: "xhigh"},

		{name: "nonsense rejected", cliType: "claude", mappedModel: "claude-opus-4-7", effort: "nonsense", wantErr: "must be one of low, medium, high, xhigh, max"},
		{name: "uppercase rejected", cliType: "claude", mappedModel: "claude-opus-4-7", effort: "HIGH", wantErr: "invalid reasoning_effort"},
		{name: "xhigh on sonnet rejected", cliType: "claude", mappedModel: "claude-sonnet-4-5", effort: "xhigh", wantErr: "only supported on Opus 4.7"},
		{name: "xhigh on opus 4.6 rejected", cliType: "claude", mappedModel: "claude-opus-4-6", effort: "xhigh", wantErr: "only supported on Opus 4.7"},
		{name: "xhigh on opus 4.6 1M rejected", cliType: "claude", mappedModel: "claude-opus-4-6[1m]", effort: "xhigh", wantErr: "only supported on Opus 4.7"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, cleanup := setupCLIModelTestEnv(t)
			defer cleanup()

			req := types.CLIModelCreateRequest{
				ID:              fmt.Sprintf("re-test-%d", i),
				CLIType:         tt.cliType,
				DisplayName:     "RE Test",
				MappedModel:     tt.mappedModel,
				ReasoningEffort: tt.effort,
			}
			m, err := svc.Create(req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Create: unexpected error: %v", err)
				}
				if m.ReasoningEffort != tt.effort {
					t.Errorf("ReasoningEffort = %q, want %q", m.ReasoningEffort, tt.effort)
				}
				return
			}
			if err == nil {
				t.Fatalf("Create: expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestCLIModel_UpdateReasoningEffort_Valid(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	// Seeded row: opus_4_7 → claude + claude-opus-4-7.
	effort := "xhigh"
	updated, err := svc.Update("opus_4_7", types.CLIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %q, want %q", updated.ReasoningEffort, "xhigh")
	}
}

func TestCLIModel_UpdateReasoningEffort_XhighRejectedOnNonOpus47(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	// sonnet is seeded as claude CLI with mapped_model=sonnet.
	effort := "xhigh"
	_, err := svc.Update("sonnet", types.CLIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err == nil {
		t.Fatal("expected error for xhigh on non-Opus-4.7 model, got nil")
	}
	if !strings.Contains(err.Error(), "only supported on Opus 4.7") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "only supported on Opus 4.7")
	}
}

func TestCLIModel_UpdateReasoningEffort_InvalidValue(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	effort := "nonsense"
	_, err := svc.Update("sonnet", types.CLIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err == nil {
		t.Fatal("expected error for invalid reasoning_effort, got nil")
	}
	if !strings.Contains(err.Error(), "must be one of") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "must be one of")
	}
}

func TestCLIModel_UpdateMappedModel_InvalidatesStoredXhigh(t *testing.T) {
	// User stored xhigh on a user-owned Opus-4.7 row, then changes mapped_model to sonnet
	// without clearing effort. Overlay logic must reject the Update.
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	// Create a user-owned Opus-4.7 row (read_only rows block mapped_model edits).
	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "user-opus",
		CLIType:     "claude",
		DisplayName: "User Opus",
		MappedModel: "claude-opus-4-7",
	}); err != nil {
		t.Fatalf("Create user-owned row: %v", err)
	}

	// First set xhigh on the user-owned row (valid).
	xhigh := "xhigh"
	if _, err := svc.Update("user-opus", types.CLIModelUpdateRequest{
		ReasoningEffort: &xhigh,
	}); err != nil {
		t.Fatalf("initial Update: %v", err)
	}

	// Now try to switch mapped_model to a non-Opus-4.7 value without touching effort.
	newMapped := "claude-sonnet-4-5"
	_, err := svc.Update("user-opus", types.CLIModelUpdateRequest{
		MappedModel: &newMapped,
	})
	if err == nil {
		t.Fatal("expected error: overlay logic must reject xhigh + non-Opus-4.7 combination, got nil")
	}
	if !strings.Contains(err.Error(), "only supported on Opus 4.7") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "only supported on Opus 4.7")
	}

	// Verify state was not mutated.
	got, err := svc.Get("user-opus")
	if err != nil {
		t.Fatalf("Get after failed Update: %v", err)
	}
	if got.MappedModel != "claude-opus-4-7" {
		t.Errorf("MappedModel = %q after failed Update, want %q (unchanged)", got.MappedModel, "claude-opus-4-7")
	}
	if got.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %q after failed Update, want %q (unchanged)", got.ReasoningEffort, "xhigh")
	}
}

func TestCLIModel_UpdateMappedModel_AndClearEffort(t *testing.T) {
	// Switching mapped_model to non-Opus-4.7 WHILE also clearing effort must succeed.
	// Uses a user-owned row because read_only rows block mapped_model edits.
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "user-opus-2",
		CLIType:     "claude",
		DisplayName: "User Opus 2",
		MappedModel: "claude-opus-4-7",
	}); err != nil {
		t.Fatalf("Create user-owned row: %v", err)
	}

	// Seed xhigh.
	xhigh := "xhigh"
	if _, err := svc.Update("user-opus-2", types.CLIModelUpdateRequest{
		ReasoningEffort: &xhigh,
	}); err != nil {
		t.Fatalf("initial Update: %v", err)
	}

	// Switch mapped_model AND clear effort in same request.
	newMapped := "claude-sonnet-4-5"
	empty := ""
	updated, err := svc.Update("user-opus-2", types.CLIModelUpdateRequest{
		MappedModel:     &newMapped,
		ReasoningEffort: &empty,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.MappedModel != "claude-sonnet-4-5" {
		t.Errorf("MappedModel = %q, want %q", updated.MappedModel, "claude-sonnet-4-5")
	}
	if updated.ReasoningEffort != "" {
		t.Errorf("ReasoningEffort = %q, want empty", updated.ReasoningEffort)
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
