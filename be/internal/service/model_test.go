package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

func strPtr(v string) *string { return &v }

func setupModelService(t *testing.T) *ModelService {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "models.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewModelService(pool, clock.Real())
}

func TestModelServiceSeedAndModeValidation(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	m, err := svc.Get("gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	if m.CLIModel != "gpt-5.6-sol" || m.APIModel != "gpt-5.6-sol" || m.DefaultEffort != "low" {
		t.Fatalf("unexpected seeded model: %+v", m)
	}
	for _, mode := range []string{"cli", "api"} {
		valid, err := svc.IsValidModelForMode(m.ID, mode)
		if err != nil || !valid {
			t.Fatalf("IsValidModelForMode(%s): valid=%v err=%v", mode, valid, err)
		}
	}
	valid, err := svc.IsValidModelForMode("gpt-5.6-terra", "api")
	if err != nil || !valid {
		t.Fatalf("api-only check: valid=%v err=%v", valid, err)
	}
}

func TestModelServiceCreateUpdateDelete(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	created, err := svc.Create(types.ModelCreateRequest{
		ID:             "CUSTOM-MERGED",
		Provider:       "anthropic",
		DisplayName:    "Custom",
		CLIModel:       "custom-cli",
		APIModel:       "custom-api",
		CLIEfforts:     []string{"high", "low"},
		APIEfforts:     []string{"low", "high"},
		FallbackModels: " one, two ",
		DefaultEffort:  "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "custom-merged" || created.CLIContext != 200000 || created.FallbackModels != "one,two" {
		t.Fatalf("unexpected created model: %+v", created)
	}
	empty := ""
	updated, err := svc.Update(created.ID, types.ModelUpdateRequest{APIModel: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if updated.APIModel != "" {
		t.Fatalf("APIModel = %q", updated.APIModel)
	}
	if err := svc.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(created.ID); err == nil {
		t.Fatal("deleted model still resolves")
	}
}

func TestModelServiceRejectsInvalidModesAndDefaults(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	_, err := svc.Create(types.ModelCreateRequest{
		ID: "no-mode", Provider: "openai", DisplayName: "No mode",
	})
	if err == nil {
		t.Fatal("expected missing mode error")
	}
	_, err = svc.Create(types.ModelCreateRequest{
		ID: "bad-default", Provider: "openai", DisplayName: "Bad", CLIModel: "bad",
		CLIEfforts: []string{"low"}, DefaultEffort: "high",
	})
	if err == nil {
		t.Fatal("expected default_effort validation error")
	}
}

func TestModelServiceBuiltInUpdateRestrictions(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	high := "high"
	updated, err := svc.Update("gpt-5.4", types.ModelUpdateRequest{DefaultEffort: &high})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DefaultEffort != "high" {
		t.Fatalf("DefaultEffort = %q", updated.DefaultEffort)
	}
	name := "Renamed"
	if _, err := svc.Update("gpt-5.4", types.ModelUpdateRequest{DisplayName: &name}); err == nil {
		t.Fatal("built-in display_name update succeeded")
	}
	if err := svc.Delete("gpt-5.4"); err == nil {
		t.Fatal("built-in delete succeeded")
	}
}

func TestModelServiceInUseBlocksDisableAndDelete(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	created, err := svc.Create(types.ModelCreateRequest{
		ID: "in-use", Provider: "openai", DisplayName: "In use", CLIModel: "in-use",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := svc.clock.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('model-use', 'Use', '/tmp', ?, ?)`, []any{now, now}},
		{`INSERT INTO workflows (project_id, id, scope_type, created_at, updated_at) VALUES ('model-use', 'wf', 'project', ?, ?)`, []any{now, now}},
		{`INSERT INTO agent_definitions (project_id, workflow_id, id, model, prompt, created_at, updated_at) VALUES ('model-use', 'wf', 'agent', ?, 'p', ?, ?)`, []any{created.ID, now, now}},
	}
	for _, statement := range statements {
		if _, err := svc.pool.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed usage: %v", err)
		}
	}
	disabled := false
	if _, err := svc.Update(created.ID, types.ModelUpdateRequest{Enabled: &disabled}); err == nil {
		t.Fatal("disable of in-use model succeeded")
	}
	if err := svc.Delete(created.ID); err == nil {
		t.Fatal("delete of in-use model succeeded")
	}
}

func TestModelServiceProviderFallbackAndModeInvariant(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	_, err := svc.Create(types.ModelCreateRequest{
		ID: "bad-fallback", Provider: "openai", DisplayName: "Bad", CLIModel: "bad",
		FallbackModels: "fallback",
	})
	if err == nil {
		t.Fatal("openai fallback_models accepted")
	}
	created, err := svc.Create(types.ModelCreateRequest{
		ID: "one-mode", Provider: "anthropic", DisplayName: "One", CLIModel: "one",
	})
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	if _, err := svc.Update(created.ID, types.ModelUpdateRequest{CLIModel: &empty}); err == nil {
		t.Fatal("update removed the final supported mode")
	}
}
