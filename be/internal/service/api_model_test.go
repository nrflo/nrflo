package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

func setupAPIModelTestEnv(t *testing.T) (*APIModelService, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "api_model_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	svc := NewAPIModelService(pool, clock.Real())
	return svc, func() { pool.Close() }
}

// --- List ---

func TestAPIModel_List(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	models, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(models) != 20 {
		t.Fatalf("List len = %d, want 20 (8 anthropic + 12 openai)", len(models))
	}

	// ORDER BY id ascending — first and last entries.
	if models[0].ID != "gpt53_codex_high" {
		t.Errorf("List[0].ID = %q, want %q", models[0].ID, "gpt53_codex_high")
	}
	if models[len(models)-1].ID != "sonnet" {
		t.Errorf("List[last].ID = %q, want %q", models[len(models)-1].ID, "sonnet")
	}

	// All seeded models are read-only.
	for _, m := range models {
		if !m.ReadOnly {
			t.Errorf("seeded model %q: ReadOnly = false, want true", m.ID)
		}
	}
}

// --- Get ---

func TestAPIModel_Get(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	// Anthropic row
	m, err := svc.Get("opus_4_7")
	if err != nil {
		t.Fatalf("Get opus_4_7: %v", err)
	}
	if m.ID != "opus_4_7" {
		t.Errorf("ID = %q, want opus_4_7", m.ID)
	}
	if m.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", m.Provider)
	}
	if m.MappedModel != "claude-opus-4-7" {
		t.Errorf("MappedModel = %q, want claude-opus-4-7", m.MappedModel)
	}
	if m.ContextLength != 1000000 {
		t.Errorf("ContextLength = %d, want 1000000 (opus 4.7 is 1M-native on the API)", m.ContextLength)
	}
	if !m.ReadOnly {
		t.Error("ReadOnly = false, want true")
	}

	// OpenAI row
	gpt, err := svc.Get("gpt54_high")
	if err != nil {
		t.Fatalf("Get gpt54_high: %v", err)
	}
	if gpt.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", gpt.Provider)
	}
	if !gpt.ReadOnly {
		t.Error("openai row ReadOnly = false, want true")
	}
}

func TestAPIModel_GetNotFound(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
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

func TestAPIModel_Create(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Create(types.APIModelCreateRequest{
		ID:            "my-api-model",
		Provider:      "anthropic",
		DisplayName:   "My API Model",
		MappedModel:   "claude-3-5-sonnet",
		ContextLength: 100000,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.ID != "my-api-model" {
		t.Errorf("ID = %q, want my-api-model", m.ID)
	}
	if m.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", m.Provider)
	}
	if m.DisplayName != "My API Model" {
		t.Errorf("DisplayName = %q, want My API Model", m.DisplayName)
	}
	if m.MappedModel != "claude-3-5-sonnet" {
		t.Errorf("MappedModel = %q, want claude-3-5-sonnet", m.MappedModel)
	}
	if m.ContextLength != 100000 {
		t.Errorf("ContextLength = %d, want 100000", m.ContextLength)
	}
	if m.ReadOnly {
		t.Error("ReadOnly = true, want false for user-created model")
	}
	if !m.Enabled {
		t.Error("Enabled = false, want true for new model")
	}
	if m.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}

	got, err := svc.Get("my-api-model")
	if err != nil {
		t.Fatalf("Get after Create: %v", err)
	}
	if got.ReadOnly {
		t.Error("Get ReadOnly = true, want false")
	}
}

func TestAPIModel_CreateInvalidProvider(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	_, err := svc.Create(types.APIModelCreateRequest{
		ID:          "bad-provider",
		Provider:    "azure",
		DisplayName: "Bad",
		MappedModel: "gpt-4",
	})
	if err == nil {
		t.Fatal("expected error for invalid provider, got nil")
	}
	if !strings.Contains(err.Error(), "invalid provider") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "invalid provider")
	}
}

func TestAPIModel_CreateMissingID(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	_, err := svc.Create(types.APIModelCreateRequest{
		Provider:    "anthropic",
		DisplayName: "No ID",
		MappedModel: "sonnet",
	})
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %q, want to contain required", err.Error())
	}
}

func TestAPIModel_CreateMissingDisplayName(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	_, err := svc.Create(types.APIModelCreateRequest{
		ID:          "nodisplay",
		Provider:    "anthropic",
		MappedModel: "sonnet",
	})
	if err == nil {
		t.Fatal("expected error for missing display_name, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %q, want to contain required", err.Error())
	}
}

func TestAPIModel_CreateMissingMappedModel(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	_, err := svc.Create(types.APIModelCreateRequest{
		ID:          "nomap",
		Provider:    "anthropic",
		DisplayName: "No Map",
	})
	if err == nil {
		t.Fatal("expected error for missing mapped_model, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %q, want to contain required", err.Error())
	}
}

func TestAPIModel_CreateDuplicate(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	req := types.APIModelCreateRequest{
		ID:          "dup-api-model",
		Provider:    "openai",
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
		t.Errorf("error = %q, want to contain already exists", err.Error())
	}
}

func TestAPIModel_CreateContextLengthDefault(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Create(types.APIModelCreateRequest{
		ID:          "default-ctx-api",
		Provider:    "openai",
		DisplayName: "Default Context",
		MappedModel: "gpt-4",
		// ContextLength = 0 → should default to 200000
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.ContextLength != 200000 {
		t.Errorf("ContextLength = %d, want 200000 (default)", m.ContextLength)
	}
}

func TestAPIModel_CreateIDNormalized(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Create(types.APIModelCreateRequest{
		ID:          "MyAPIModel",
		Provider:    "anthropic",
		DisplayName: "Custom",
		MappedModel: "claude-3-5-sonnet",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.ID != "myapimodel" {
		t.Errorf("ID = %q, want %q (lowercased)", m.ID, "myapimodel")
	}
}
