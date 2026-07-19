package console

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/types"
)

func newSpecTestPool(t *testing.T) (*db.Pool, *clock.TestClock) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "spec_test.db")
	if err := copyConsoleTemplateDB(dbPath); err != nil {
		t.Fatalf("copyConsoleTemplateDB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("OpenPoolExisting: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return pool, clk
}

func seedSpecProject(t *testing.T, pool *db.Pool, id, rootPath string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, id, rootPath, now, now)
}

func TestBuildChatEngineSpec_NoModel_PassesThroughEmpty(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)
	seedSpecProject(t, pool, "proj-spec-nomodel", "/work/proj")

	spec, err := buildChatEngineSpec(pool, clk, chatSpecParams{
		SessionID: "s1", ProjectID: "proj-spec-nomodel", Engine: "codex", ModelID: "", SpawnToken: "tok", ServerURL: "http://x",
	})
	if err != nil {
		t.Fatalf("buildChatEngineSpec: %v", err)
	}
	if spec.Model != "" {
		t.Errorf("Model = %q, want empty", spec.Model)
	}
	if spec.WorkDir != "/work/proj" {
		t.Errorf("WorkDir = %q, want /work/proj", spec.WorkDir)
	}
}

func TestBuildChatEngineSpec_UnknownModelID_PassesThroughRaw(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)
	seedSpecProject(t, pool, "proj-spec-unknown", "")

	spec, err := buildChatEngineSpec(pool, clk, chatSpecParams{
		SessionID: "s1", ProjectID: "proj-spec-unknown", Engine: "codex", ModelID: "gpt-raw-name", SpawnToken: "tok",
	})
	if err != nil {
		t.Fatalf("buildChatEngineSpec: %v", err)
	}
	if spec.Model != "gpt-raw-name" {
		t.Errorf("Model = %q, want raw passthrough %q", spec.Model, "gpt-raw-name")
	}
	if spec.WorkDir != "" {
		t.Errorf("WorkDir = %q, want empty for a project with no root_path", spec.WorkDir)
	}
}

func TestBuildChatEngineSpec_KnownEnabledModel_ResolvesRegistryFields(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)
	seedSpecProject(t, pool, "proj-spec-known", "")

	modelSvc := service.NewModelService(pool, clk)
	if _, err := modelSvc.Create(types.ModelCreateRequest{
		ID: "codex-fast", Provider: "openai", DisplayName: "Codex Fast", CLIModel: "gpt-5.5",
		CLIEfforts: []string{"high"}, DefaultEffort: "high", CLIContext: 200000,
	}); err != nil {
		t.Fatalf("seed models row: %v", err)
	}

	spec, err := buildChatEngineSpec(pool, clk, chatSpecParams{
		SessionID: "s1", ProjectID: "proj-spec-known", Engine: "codex", ModelID: "codex-fast", SpawnToken: "tok",
	})
	if err != nil {
		t.Fatalf("buildChatEngineSpec: %v", err)
	}
	if spec.Model != "gpt-5.5" {
		t.Errorf("Model = %q, want mapped_model gpt-5.5", spec.Model)
	}
	if spec.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q, want high", spec.ReasoningEffort)
	}
	if spec.MaxContext != 200000 {
		t.Errorf("MaxContext = %d, want 200000", spec.MaxContext)
	}
}

func TestBuildChatEngineSpec_ModelForDifferentCLIType_Errors(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)
	seedSpecProject(t, pool, "proj-spec-wrongcli", "")

	modelSvc := service.NewModelService(pool, clk)
	if _, err := modelSvc.Create(types.ModelCreateRequest{
		ID: "claude-only", Provider: "anthropic", DisplayName: "Claude", CLIModel: "sonnet-5",
	}); err != nil {
		t.Fatalf("seed models row: %v", err)
	}

	_, err := buildChatEngineSpec(pool, clk, chatSpecParams{
		SessionID: "s1", ProjectID: "proj-spec-wrongcli", Engine: "codex", ModelID: "claude-only", SpawnToken: "tok",
	})
	if err == nil {
		t.Fatal("buildChatEngineSpec with wrong-cli model: want error, got nil")
	}
}

func TestBuildChatEngineSpec_DisabledModel_Errors(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)
	seedSpecProject(t, pool, "proj-spec-disabled", "")

	modelSvc := service.NewModelService(pool, clk)
	if _, err := modelSvc.Create(types.ModelCreateRequest{
		ID: "codex-disabled", Provider: "openai", DisplayName: "Disabled", CLIModel: "gpt-5.5",
	}); err != nil {
		t.Fatalf("seed models row: %v", err)
	}
	disabled := false
	if _, err := modelSvc.Update("codex-disabled", types.ModelUpdateRequest{Enabled: &disabled}); err != nil {
		t.Fatalf("disable model: %v", err)
	}

	_, err := buildChatEngineSpec(pool, clk, chatSpecParams{
		SessionID: "s1", ProjectID: "proj-spec-disabled", Engine: "codex", ModelID: "codex-disabled", SpawnToken: "tok",
	})
	if err == nil {
		t.Fatal("buildChatEngineSpec with disabled model: want error, got nil")
	}
}

func TestBuildChatEngineSpec_UnknownProject_Errors(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	_, err := buildChatEngineSpec(pool, clk, chatSpecParams{
		SessionID: "s1", ProjectID: "no-such-project", Engine: "codex", SpawnToken: "tok",
	})
	if err == nil {
		t.Fatal("buildChatEngineSpec with unknown project: want error, got nil")
	}
}

// TestBuildChatEngineSpec_SystemTemplateID_RendersIntoSystemPrompt verifies a
// non-empty SystemTemplateID is rendered (via spawner.RenderInjectable) into
// spec.SystemPrompt, using the PROJECT_ID/MODEL/NODE_ID vars.
func TestBuildChatEngineSpec_SystemTemplateID_RendersIntoSystemPrompt(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)
	seedSpecProject(t, pool, "proj-spec-systempl", "")

	spec, err := buildChatEngineSpec(pool, clk, chatSpecParams{
		SessionID: "s1", ProjectID: "proj-spec-systempl", Engine: "codex", ModelID: "",
		SpawnToken: "tok", SystemTemplateID: "tier-t2-extractor",
	})
	if err != nil {
		t.Fatalf("buildChatEngineSpec: %v", err)
	}
	if spec.SystemPrompt == "" {
		t.Fatal("spec.SystemPrompt is empty, want the rendered tier-t2-extractor template")
	}
	if !strings.Contains(spec.SystemPrompt, "T2 Extractor") {
		t.Errorf("spec.SystemPrompt = %q, want it to contain the tier-t2-extractor role text", spec.SystemPrompt)
	}
}

// TestBuildChatEngineSpec_EmptySystemTemplateID_LeavesSystemPromptEmpty is the
// byte-identical regression: an unset SystemTemplateID must not touch
// spec.SystemPrompt at all, leaving each engine's own default in play.
func TestBuildChatEngineSpec_EmptySystemTemplateID_LeavesSystemPromptEmpty(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)
	seedSpecProject(t, pool, "proj-spec-nosystempl", "")

	spec, err := buildChatEngineSpec(pool, clk, chatSpecParams{
		SessionID: "s1", ProjectID: "proj-spec-nosystempl", Engine: "codex", ModelID: "", SpawnToken: "tok",
	})
	if err != nil {
		t.Fatalf("buildChatEngineSpec: %v", err)
	}
	if spec.SystemPrompt != "" {
		t.Errorf("spec.SystemPrompt = %q, want empty when SystemTemplateID is unset", spec.SystemPrompt)
	}
}

// TestBuildChatEngineSpec_SystemTemplateID_UsesResolvedModelInVars verifies
// the MODEL var fed into the injectable render is the resolved CLI model
// (mapped_model), not the raw registry id, when a known model is selected.
func TestBuildChatEngineSpec_SystemTemplateID_UsesResolvedModelInVars(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)
	seedSpecProject(t, pool, "proj-spec-systempl-model", "")

	modelSvc := service.NewModelService(pool, clk)
	if _, err := modelSvc.Create(types.ModelCreateRequest{
		ID: "codex-fast2", Provider: "openai", DisplayName: "Codex Fast 2", CLIModel: "gpt-5.5",
		CLIEfforts: []string{"high"}, DefaultEffort: "high", CLIContext: 200000,
	}); err != nil {
		t.Fatalf("seed models row: %v", err)
	}

	// Seed a custom injectable that echoes ${MODEL} so the resolved value is observable.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at) VALUES (?, ?, ?, ?, 0, 'injectable', ?, ?)`,
		"chat-spec-model-echo", "Model Echo", "MODEL=${MODEL}", "MODEL=${MODEL}", now, now)

	spec, err := buildChatEngineSpec(pool, clk, chatSpecParams{
		SessionID: "s1", ProjectID: "proj-spec-systempl-model", Engine: "codex", ModelID: "codex-fast2",
		SpawnToken: "tok", SystemTemplateID: "chat-spec-model-echo",
	})
	if err != nil {
		t.Fatalf("buildChatEngineSpec: %v", err)
	}
	if spec.SystemPrompt != "MODEL=gpt-5.5" {
		t.Errorf("spec.SystemPrompt = %q, want MODEL=gpt-5.5 (resolved CLI model)", spec.SystemPrompt)
	}
}

func TestChatMCPEnv_ContainsAdoptionFields(t *testing.T) {
	t.Parallel()
	env := chatMCPEnv("http://127.0.0.1:6587", "proj-1", "sess-1", "tok-1")
	want := map[string]string{
		"NRFLO_SERVER_URL":         "http://127.0.0.1:6587",
		"NRFLO_PROJECT":            "proj-1",
		"NRFLO_CONSOLE_TOKEN":      "tok-1",
		"NRFLO_CONSOLE_SESSION_ID": "sess-1",
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("chatMCPEnv[%q] = %q, want %q", k, env[k], v)
		}
	}
}
