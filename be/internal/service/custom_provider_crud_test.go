package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// TestCustomProviderGetEnabled verifies GetEnabled returns the row when
// enabled and errors when disabled or missing.
func TestCustomProviderGetEnabled(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	created, err := svc.Create(validCreateReq())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.GetEnabled(created.Name); err != nil {
		t.Errorf("GetEnabled(enabled row): %v", err)
	}

	enabled := false
	if _, err := svc.Update(created.Name, types.CustomProviderUpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatalf("Update disable: %v", err)
	}
	if _, err := svc.GetEnabled(created.Name); err == nil {
		t.Error("GetEnabled(disabled row) succeeded, want error")
	}

	if _, err := svc.GetEnabled("does-not-exist"); err == nil {
		t.Error("GetEnabled(missing row) succeeded, want error")
	}
}

// TestCustomProviderExists verifies Exists reports presence regardless of
// enabled state, and false for a missing name.
func TestCustomProviderExists(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	created, err := svc.Create(validCreateReq())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	exists, err := svc.Exists(created.Name)
	if err != nil || !exists {
		t.Errorf("Exists(created) = %v, %v; want true, nil", exists, err)
	}

	enabled := false
	if _, err := svc.Update(created.Name, types.CustomProviderUpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatalf("Update disable: %v", err)
	}
	exists, err = svc.Exists(created.Name)
	if err != nil || !exists {
		t.Errorf("Exists(disabled) = %v, %v; want true, nil (Exists ignores enabled)", exists, err)
	}

	exists, err = svc.Exists("does-not-exist")
	if err != nil || exists {
		t.Errorf("Exists(missing) = %v, %v; want false, nil", exists, err)
	}
}

// TestCustomProviderListEnabled_FiltersDisabled verifies ListEnabled excludes
// disabled rows while List returns all.
func TestCustomProviderListEnabled_FiltersDisabled(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	if _, err := svc.Create(validCreateReq()); err != nil {
		t.Fatalf("Create enabled: %v", err)
	}
	req2 := validCreateReq()
	req2.Name = "local-lmstudio"
	if _, err := svc.Create(req2); err != nil {
		t.Fatalf("Create second: %v", err)
	}
	enabled := false
	if _, err := svc.Update("local-lmstudio", types.CustomProviderUpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatalf("Update disable: %v", err)
	}

	all, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List len = %d, want 2", len(all))
	}

	onlyEnabled, err := svc.ListEnabled()
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(onlyEnabled) != 1 || onlyEnabled[0].Name != "local-ollama" {
		t.Fatalf("ListEnabled = %+v, want only local-ollama", onlyEnabled)
	}
}

// TestCustomProviderDelete_InUse_Rejected verifies Delete is blocked while a
// models row references the provider name, and succeeds once unreferenced.
func TestCustomProviderDelete_InUse_Rejected(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	created, err := svc.Create(validCreateReq())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	modelSvc := NewModelService(svc.pool, svc.clock)
	if _, err := modelSvc.Create(types.ModelCreateRequest{
		ID: "local-model-1", Provider: created.Name, DisplayName: "Local Model",
		APIModel: "local-model",
	}); err != nil {
		t.Fatalf("seed model referencing provider: %v", err)
	}

	if err := svc.Delete(created.Name); err == nil {
		t.Fatal("Delete(in-use provider) succeeded, want error")
	} else if !strings.Contains(err.Error(), "in use by") {
		t.Errorf("error = %v, want mention of in use by", err)
	}

	if err := modelSvc.Delete("local-model-1"); err != nil {
		t.Fatalf("cleanup delete model: %v", err)
	}
	if err := svc.Delete(created.Name); err != nil {
		t.Errorf("Delete(unreferenced provider): %v", err)
	}
}

// TestCustomProviderUpdate_Disable_InUse_Rejected mirrors the delete guard
// for the enabled=false path.
func TestCustomProviderUpdate_Disable_InUse_Rejected(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	created, err := svc.Create(validCreateReq())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	modelSvc := NewModelService(svc.pool, svc.clock)
	if _, err := modelSvc.Create(types.ModelCreateRequest{
		ID: "local-model-2", Provider: created.Name, DisplayName: "Local Model 2",
		APIModel: "local-model-2",
	}); err != nil {
		t.Fatalf("seed model: %v", err)
	}

	enabled := false
	if _, err := svc.Update(created.Name, types.CustomProviderUpdateRequest{Enabled: &enabled}); err == nil {
		t.Fatal("Update(disable in-use) succeeded, want error")
	}
}

// TestCustomProviderUpdate_PartialFields verifies pointer-semantics update:
// only supplied fields change, others are preserved.
func TestCustomProviderUpdate_PartialFields(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	created, err := svc.Create(validCreateReq())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	newKey := "sk-new-key"
	updated, err := svc.Update(created.Name, types.CustomProviderUpdateRequest{APIKey: &newKey})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.APIKey != "sk-new-key" {
		t.Errorf("APIKey = %q, want sk-new-key", updated.APIKey)
	}
	if updated.BaseURL != created.BaseURL {
		t.Errorf("BaseURL changed unexpectedly: %q -> %q", created.BaseURL, updated.BaseURL)
	}
}

// TestCustomProviderUpdate_InvalidBaseURL_Rejected verifies Update validates
// base_url the same way Create does.
func TestCustomProviderUpdate_InvalidBaseURL_Rejected(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	created, err := svc.Create(validCreateReq())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	bad := "not-a-url"
	if _, err := svc.Update(created.Name, types.CustomProviderUpdateRequest{BaseURL: &bad}); err == nil {
		t.Fatal("Update(invalid base_url) succeeded, want error")
	}
}

// TestCustomProviderUpdate_NotFound verifies Update on a missing name errors.
func TestCustomProviderUpdate_NotFound(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	newKey := "sk-x"
	if _, err := svc.Update("missing-provider", types.CustomProviderUpdateRequest{APIKey: &newKey}); err == nil {
		t.Fatal("Update(missing) succeeded, want error")
	}
}

// TestCustomProviderDelete_NotFound verifies Delete on a missing name errors.
func TestCustomProviderDelete_NotFound(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	if err := svc.Delete("missing-provider"); err == nil {
		t.Fatal("Delete(missing) succeeded, want error")
	}
}
