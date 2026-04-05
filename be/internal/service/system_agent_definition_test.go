package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupSysAgentDefTestEnv creates an isolated DB for system agent definition tests.
func setupSysAgentDefTestEnv(t *testing.T) (*SystemAgentDefinitionService, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sys_agent_def_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	svc := NewSystemAgentDefinitionService(pool, clock.Real())
	return svc, func() { pool.Close() }
}

// intPtr is a convenience for creating *int values in tests.
func intPtr(v int) *int { return &v }

// --- Create + Get roundtrip ---

func TestSystemAgentDef_CreateAndGet(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	// Remove seeded conflict-resolver from migration so we can test Create.
	_ = svc.Delete("conflict-resolver")

	rt := intPtr(25)
	mfr := intPtr(3)
	sst := intPtr(60)
	srt := intPtr(300)

	def, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:                     "conflict-resolver",
		Model:                  "opus",
		Timeout:                30,
		Prompt:                 "resolve conflicts",
		RestartThreshold:       rt,
		MaxFailRestarts:        mfr,
		StallStartTimeoutSec:   sst,
		StallRunningTimeoutSec: srt,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if def.ID != "conflict-resolver" {
		t.Errorf("ID = %q, want %q", def.ID, "conflict-resolver")
	}
	if def.Model != "opus" {
		t.Errorf("Model = %q, want %q", def.Model, "opus")
	}
	if def.Timeout != 30 {
		t.Errorf("Timeout = %d, want 30", def.Timeout)
	}
	if def.Prompt != "resolve conflicts" {
		t.Errorf("Prompt = %q, want %q", def.Prompt, "resolve conflicts")
	}
	if def.RestartThreshold == nil || *def.RestartThreshold != 25 {
		t.Errorf("RestartThreshold = %v, want 25", def.RestartThreshold)
	}
	if def.MaxFailRestarts == nil || *def.MaxFailRestarts != 3 {
		t.Errorf("MaxFailRestarts = %v, want 3", def.MaxFailRestarts)
	}
	if def.StallStartTimeoutSec == nil || *def.StallStartTimeoutSec != 60 {
		t.Errorf("StallStartTimeoutSec = %v, want 60", def.StallStartTimeoutSec)
	}
	if def.StallRunningTimeoutSec == nil || *def.StallRunningTimeoutSec != 300 {
		t.Errorf("StallRunningTimeoutSec = %v, want 300", def.StallRunningTimeoutSec)
	}
	if def.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if def.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}

	// Get round-trips all fields.
	got, err := svc.Get("conflict-resolver")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != def.ID {
		t.Errorf("Get ID = %q, want %q", got.ID, def.ID)
	}
	if got.Model != def.Model {
		t.Errorf("Get Model = %q, want %q", got.Model, def.Model)
	}
	if got.Timeout != def.Timeout {
		t.Errorf("Get Timeout = %d, want %d", got.Timeout, def.Timeout)
	}
	if got.RestartThreshold == nil || *got.RestartThreshold != 25 {
		t.Errorf("Get RestartThreshold = %v, want 25", got.RestartThreshold)
	}
	if got.MaxFailRestarts == nil || *got.MaxFailRestarts != 3 {
		t.Errorf("Get MaxFailRestarts = %v, want 3", got.MaxFailRestarts)
	}
}

// --- Default values ---

func TestSystemAgentDef_CreateWithDefaults(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	def, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:     "defaults-agent",
		Prompt: "do something",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if def.Model != "sonnet" {
		t.Errorf("default Model = %q, want %q", def.Model, "sonnet")
	}
	if def.Timeout != 20 {
		t.Errorf("default Timeout = %d, want 20", def.Timeout)
	}
	if def.RestartThreshold != nil {
		t.Errorf("RestartThreshold = %v, want nil", def.RestartThreshold)
	}
	if def.MaxFailRestarts != nil {
		t.Errorf("MaxFailRestarts = %v, want nil", def.MaxFailRestarts)
	}
	if def.StallStartTimeoutSec != nil {
		t.Errorf("StallStartTimeoutSec = %v, want nil", def.StallStartTimeoutSec)
	}
	if def.StallRunningTimeoutSec != nil {
		t.Errorf("StallRunningTimeoutSec = %v, want nil", def.StallRunningTimeoutSec)
	}
}

// --- Create with missing ID ---

func TestSystemAgentDef_CreateMissingID(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	_, err := svc.Create(&types.SystemAgentDefCreateRequest{
		Prompt: "do something",
	})
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "required")
	}
}

// --- Duplicate create ---

func TestSystemAgentDef_CreateDuplicate(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	req := &types.SystemAgentDefCreateRequest{ID: "dup-agent", Prompt: "p"}
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

// --- Get not found ---

func TestSystemAgentDef_GetNotFound(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	_, err := svc.Get("nonexistent-agent")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

// --- List ---

func TestSystemAgentDef_List(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	// Delete seeded data so test starts from a known empty state.
	_ = svc.Delete("conflict-resolver")
	_ = svc.Delete("context-saver")

	// Initially empty.
	defs, err := svc.List()
	if err != nil {
		t.Fatalf("List (empty): %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("initial List len = %d, want 0", len(defs))
	}

	ids := []string{"agent-b", "agent-a", "agent-c"}
	for _, id := range ids {
		if _, err := svc.Create(&types.SystemAgentDefCreateRequest{ID: id, Prompt: "p"}); err != nil {
			t.Fatalf("Create %q: %v", id, err)
		}
	}

	defs, err = svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(defs) != 3 {
		t.Fatalf("List len = %d, want 3", len(defs))
	}

	// Verify ORDER BY id ascending.
	wantOrder := []string{"agent-a", "agent-b", "agent-c"}
	for i, want := range wantOrder {
		if defs[i].ID != want {
			t.Errorf("List[%d].ID = %q, want %q", i, defs[i].ID, want)
		}
	}
}

// --- Partial update ---

func TestSystemAgentDef_Update_PartialUpdate(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID: "upd-agent", Model: "haiku", Timeout: 10, Prompt: "original",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newModel := "opus"
	if err := svc.Update("upd-agent", &types.SystemAgentDefUpdateRequest{
		Model: &newModel,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := svc.Get("upd-agent")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Model != "opus" {
		t.Errorf("after update Model = %q, want %q", got.Model, "opus")
	}
	// Unmodified fields preserved.
	if got.Timeout != 10 {
		t.Errorf("after update Timeout = %d, want 10", got.Timeout)
	}
	if got.Prompt != "original" {
		t.Errorf("after update Prompt = %q, want %q", got.Prompt, "original")
	}
}

func TestSystemAgentDef_Update_PointerFields(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID: "ptr-agent", Prompt: "p",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rt := intPtr(30)
	mfr := intPtr(5)
	if err := svc.Update("ptr-agent", &types.SystemAgentDefUpdateRequest{
		RestartThreshold: rt,
		MaxFailRestarts:  mfr,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := svc.Get("ptr-agent")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.RestartThreshold == nil || *got.RestartThreshold != 30 {
		t.Errorf("RestartThreshold = %v, want 30", got.RestartThreshold)
	}
	if got.MaxFailRestarts == nil || *got.MaxFailRestarts != 5 {
		t.Errorf("MaxFailRestarts = %v, want 5", got.MaxFailRestarts)
	}
}

func TestSystemAgentDef_Update_NoFieldsIsNoOp(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID: "noop-agent", Prompt: "p", Model: "haiku",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Empty update should succeed without error.
	if err := svc.Update("noop-agent", &types.SystemAgentDefUpdateRequest{}); err != nil {
		t.Fatalf("empty Update: %v", err)
	}

	got, err := svc.Get("noop-agent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Model != "haiku" {
		t.Errorf("Model = %q after no-op update, want %q", got.Model, "haiku")
	}
}

// --- Update not found ---

func TestSystemAgentDef_Update_NotFound(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	newModel := "opus"
	err := svc.Update("no-such-agent", &types.SystemAgentDefUpdateRequest{Model: &newModel})
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

// --- Delete ---

func TestSystemAgentDef_Delete(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.SystemAgentDefCreateRequest{ID: "del-agent", Prompt: "p"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete("del-agent"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Subsequent Get returns not-found.
	_, err := svc.Get("del-agent")
	if err == nil {
		t.Fatal("expected not-found after Delete, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

func TestSystemAgentDef_Delete_NotFound(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	err := svc.Delete("no-such-agent")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

// --- Case-insensitive ID lookup ---

func TestSystemAgentDef_CaseInsensitiveID(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	// ID is lowercased on create.
	def, err := svc.Create(&types.SystemAgentDefCreateRequest{ID: "MyAgent", Prompt: "p"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if def.ID != "myagent" {
		t.Errorf("ID = %q, want %q", def.ID, "myagent")
	}

	// Get with different case works.
	got, err := svc.Get("MYAGENT")
	if err != nil {
		t.Fatalf("Get with uppercase: %v", err)
	}
	if got.ID != "myagent" {
		t.Errorf("Get ID = %q, want %q", got.ID, "myagent")
	}
}
