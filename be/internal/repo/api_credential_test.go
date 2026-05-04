package repo

import (
	"database/sql"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

func setupAPICredRepo(t *testing.T) *APICredentialRepo {
	t.Helper()
	pool := newTestPool(t)
	return NewAPICredentialRepo(pool, clock.Real())
}

func mkCred(id, provider, secretRef string, projectID *string) *model.APICredential {
	return &model.APICredential{
		ID:        id,
		Provider:  provider,
		ProjectID: projectID,
		SecretRef: secretRef,
	}
}

// TestAPICredentialRepo_CreateGet round-trips a global credential.
func TestAPICredentialRepo_CreateGet(t *testing.T) {
	t.Parallel()
	r := setupAPICredRepo(t)

	c := mkCred("c1", "anthropic", "env:ANTHROPIC_API_KEY", nil)
	if err := r.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := r.Get("c1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", got.Provider)
	}
	if got.SecretRef != "env:ANTHROPIC_API_KEY" {
		t.Errorf("SecretRef = %q, want env:ANTHROPIC_API_KEY", got.SecretRef)
	}
	if got.ProjectID != nil {
		t.Errorf("ProjectID = %v, want nil for global", got.ProjectID)
	}
}

// TestAPICredentialRepo_List orders rows.
func TestAPICredentialRepo_List(t *testing.T) {
	t.Parallel()
	r := setupAPICredRepo(t)
	pid := "p1"
	if err := r.Create(mkCred("c1", "anthropic", "env:KEY", nil)); err != nil {
		t.Fatalf("Create c1: %v", err)
	}
	if err := r.Create(mkCred("c2", "anthropic", "literal:sk-xx", &pid)); err != nil {
		t.Fatalf("Create c2: %v", err)
	}
	got, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

// TestAPICredentialRepo_Update mutates fields.
func TestAPICredentialRepo_Update(t *testing.T) {
	t.Parallel()
	r := setupAPICredRepo(t)
	if err := r.Create(mkCred("c1", "anthropic", "env:OLD", nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	v := "literal:sk-new"
	if err := r.Update("c1", &APICredentialUpdateFields{SecretRef: &v}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := r.Get("c1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.SecretRef != v {
		t.Errorf("SecretRef = %q, want %q", got.SecretRef, v)
	}
}

func TestAPICredentialRepo_Delete(t *testing.T) {
	t.Parallel()
	r := setupAPICredRepo(t)
	if err := r.Create(mkCred("c1", "anthropic", "env:K", nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.Delete("c1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := r.Delete("c1"); err == nil {
		t.Fatal("Delete twice: want error")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found'", err)
	}
}

// TestAPICredentialRepo_Resolve_PerProjectBeatsGlobal verifies the precedence rule.
func TestAPICredentialRepo_Resolve_PerProjectBeatsGlobal(t *testing.T) {
	t.Parallel()
	r := setupAPICredRepo(t)
	pid := "proj-a"
	if err := r.Create(mkCred("g", "anthropic", "env:GLOBAL", nil)); err != nil {
		t.Fatalf("Create global: %v", err)
	}
	if err := r.Create(mkCred("p", "anthropic", "literal:sk-proj", &pid)); err != nil {
		t.Fatalf("Create per-project: %v", err)
	}

	got, err := r.Resolve("anthropic", "proj-a")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ID != "p" {
		t.Errorf("Resolve(proj-a).ID = %q, want %q (per-project wins)", got.ID, "p")
	}
	if got.SecretRef != "literal:sk-proj" {
		t.Errorf("SecretRef = %q, want literal:sk-proj", got.SecretRef)
	}
}

// TestAPICredentialRepo_Resolve_FallbackToGlobal verifies fallback when no per-project row.
func TestAPICredentialRepo_Resolve_FallbackToGlobal(t *testing.T) {
	t.Parallel()
	r := setupAPICredRepo(t)
	if err := r.Create(mkCred("g", "anthropic", "env:GLOBAL", nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Resolve("anthropic", "proj-x")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ID != "g" {
		t.Errorf("Resolve(proj-x).ID = %q, want %q (global fallback)", got.ID, "g")
	}
	if got.ProjectID != nil {
		t.Errorf("expected global ProjectID nil, got %v", got.ProjectID)
	}
}

// TestAPICredentialRepo_Resolve_NoMatch returns sql.ErrNoRows when neither exists.
func TestAPICredentialRepo_Resolve_NoMatch(t *testing.T) {
	t.Parallel()
	r := setupAPICredRepo(t)
	_, err := r.Resolve("anthropic", "")
	if err != sql.ErrNoRows {
		t.Errorf("Resolve empty: err = %v, want sql.ErrNoRows", err)
	}
	_, err = r.Resolve("anthropic", "proj-x")
	if err != sql.ErrNoRows {
		t.Errorf("Resolve no-match: err = %v, want sql.ErrNoRows", err)
	}
}

// TestAPICredentialRepo_Resolve_EmptyProjectIDUsesGlobalOnly verifies that an
// empty project_id only returns a global row, never a per-project one.
func TestAPICredentialRepo_Resolve_EmptyProjectIDUsesGlobalOnly(t *testing.T) {
	t.Parallel()
	r := setupAPICredRepo(t)
	pid := "proj-a"
	if err := r.Create(mkCred("p", "anthropic", "literal:sk-proj", &pid)); err != nil {
		t.Fatalf("Create per-project: %v", err)
	}
	_, err := r.Resolve("anthropic", "")
	if err != sql.ErrNoRows {
		t.Errorf("Resolve(\"\") err = %v, want sql.ErrNoRows (global only)", err)
	}
}
