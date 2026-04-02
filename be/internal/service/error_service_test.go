package service

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/ws"
)

func setupErrorServiceEnv(t *testing.T) (*ErrorService, *db.Pool, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "error_svc_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	_, err = pool.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return NewErrorService(pool, clock.Real(), nil), pool, "proj-1"
}

func TestErrorService_RecordError_InsertsToDb(t *testing.T) {
	svc, pool, projectID := setupErrorServiceEnv(t)

	if err := svc.RecordError(projectID, "agent", "sess-123", "implementor: timeout"); err != nil {
		t.Fatalf("RecordError: %v", err)
	}

	var count int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM errors WHERE project_id = ?`, projectID).Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 1 {
		t.Errorf("errors count = %d, want 1", count)
	}
}

func TestErrorService_RecordError_GeneratesUUID(t *testing.T) {
	svc, pool, projectID := setupErrorServiceEnv(t)

	if err := svc.RecordError(projectID, "agent", "sess-1", "error msg"); err != nil {
		t.Fatalf("RecordError: %v", err)
	}

	var id string
	if err := pool.QueryRow(`SELECT id FROM errors WHERE project_id = ?`, projectID).Scan(&id); err != nil {
		t.Fatalf("query id: %v", err)
	}
	if id == "" {
		t.Errorf("ID should be non-empty UUID, got empty string")
	}
	if len(id) != 36 {
		t.Errorf("ID length = %d, want 36 (UUID format)", len(id))
	}
}

func TestErrorService_RecordError_SetsTimestampFromClock(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ts_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	_, err = pool.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'T', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	fixedTime := time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	svc := NewErrorService(pool, clk, nil)

	if err := svc.RecordError("p1", "workflow", "wfi-abc", "workflow failed"); err != nil {
		t.Fatalf("RecordError: %v", err)
	}

	var createdAt string
	if err := pool.QueryRow(`SELECT created_at FROM errors WHERE project_id = 'p1'`).Scan(&createdAt); err != nil {
		t.Fatalf("query created_at: %v", err)
	}
	want := fixedTime.UTC().Format(time.RFC3339Nano)
	if createdAt != want {
		t.Errorf("created_at = %q, want %q", createdAt, want)
	}
}

func TestErrorService_RecordError_NilHub_NoPanic(t *testing.T) {
	svc, _, projectID := setupErrorServiceEnv(t)
	// svc has nil hub — should not panic
	if err := svc.RecordError(projectID, "system", "wfi-99", "merge conflict"); err != nil {
		t.Fatalf("RecordError with nil hub: %v", err)
	}
}

func TestErrorService_RecordError_BroadcastsWSEvent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ws_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	_, err = pool.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-ws', 'T', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(hub.Stop)

	client, ch := ws.NewTestClient(hub, "test-client")
	hub.Register(client)
	hub.Subscribe(client, "proj-ws", "")

	svc := NewErrorService(pool, clock.Real(), hub)
	if err := svc.RecordError("proj-ws", "agent", "sess-42", "qa-verifier: exit_code"); err != nil {
		t.Fatalf("RecordError: %v", err)
	}

	select {
	case msg := <-ch:
		var event struct {
			Type string                 `json:"type"`
			Data map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventErrorCreated {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventErrorCreated)
		}
		if event.Data["error_type"] != "agent" {
			t.Errorf("data.error_type = %v, want %q", event.Data["error_type"], "agent")
		}
		if event.Data["instance_id"] != "sess-42" {
			t.Errorf("data.instance_id = %v, want %q", event.Data["instance_id"], "sess-42")
		}
		if event.Data["message"] != "qa-verifier: exit_code" {
			t.Errorf("data.message = %v, want %q", event.Data["message"], "qa-verifier: exit_code")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for WS broadcast")
	}
}

func TestErrorService_ListErrors_EmptyResult(t *testing.T) {
	svc, _, projectID := setupErrorServiceEnv(t)

	errors, total, err := svc.ListErrors(projectID, "", 1, 20)
	if err != nil {
		t.Fatalf("ListErrors: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(errors) != 0 {
		t.Errorf("errors count = %d, want 0", len(errors))
	}
}

func TestErrorService_ListErrors_Pagination(t *testing.T) {
	svc, pool, projectID := setupErrorServiceEnv(t)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		ts := base.Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339Nano)
		_, err := pool.Exec(
			`INSERT INTO errors (id, project_id, error_type, instance_id, message, created_at) VALUES (?,?,?,?,?,?)`,
			fmt.Sprintf("err-%d", i), projectID, "agent", fmt.Sprintf("s%d", i), "msg", ts,
		)
		if err != nil {
			t.Fatalf("insert error %d: %v", i, err)
		}
	}

	page1, total, err := svc.ListErrors(projectID, "", 1, 2)
	if err != nil {
		t.Fatalf("ListErrors page1: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(page1) != 2 {
		t.Errorf("page1 count = %d, want 2", len(page1))
	}

	page3, _, err := svc.ListErrors(projectID, "", 3, 2)
	if err != nil {
		t.Fatalf("ListErrors page3: %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("page3 count = %d, want 1 (last record)", len(page3))
	}
}

func TestErrorService_ListErrors_TypeFilter(t *testing.T) {
	svc, pool, projectID := setupErrorServiceEnv(t)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, typ := range []string{"agent", "agent", "workflow"} {
		ts := base.Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339Nano)
		_, err := pool.Exec(
			`INSERT INTO errors (id, project_id, error_type, instance_id, message, created_at) VALUES (?,?,?,?,?,?)`,
			fmt.Sprintf("e%d", i), projectID, typ, fmt.Sprintf("inst-%d", i), "msg", ts,
		)
		if err != nil {
			t.Fatalf("insert error %d: %v", i, err)
		}
	}

	agentErrs, total, err := svc.ListErrors(projectID, "agent", 1, 100)
	if err != nil {
		t.Fatalf("ListErrors agent: %v", err)
	}
	if total != 2 {
		t.Errorf("agent total = %d, want 2", total)
	}
	if len(agentErrs) != 2 {
		t.Errorf("agent count = %d, want 2", len(agentErrs))
	}
	for _, e := range agentErrs {
		if e.ErrorType != model.ErrorTypeAgent {
			t.Errorf("ErrorType = %q, want %q", e.ErrorType, model.ErrorTypeAgent)
		}
	}

	wfErrs, wfTotal, err := svc.ListErrors(projectID, "workflow", 1, 100)
	if err != nil {
		t.Fatalf("ListErrors workflow: %v", err)
	}
	if wfTotal != 1 {
		t.Errorf("workflow total = %d, want 1", wfTotal)
	}
	if len(wfErrs) != 1 {
		t.Errorf("workflow count = %d, want 1", len(wfErrs))
	}
}

func TestErrorService_RecordError_StoresAllFields(t *testing.T) {
	svc, pool, projectID := setupErrorServiceEnv(t)

	if err := svc.RecordError(projectID, "system", "wfi-999", "merge conflict: branch main"); err != nil {
		t.Fatalf("RecordError: %v", err)
	}

	var errType, instanceID, message string
	row := pool.QueryRow(`SELECT error_type, instance_id, message FROM errors WHERE project_id = ?`, projectID)
	if err := row.Scan(&errType, &instanceID, &message); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if errType != "system" {
		t.Errorf("error_type = %q, want %q", errType, "system")
	}
	if instanceID != "wfi-999" {
		t.Errorf("instance_id = %q, want %q", instanceID, "wfi-999")
	}
	if message != "merge conflict: branch main" {
		t.Errorf("message = %q, want %q", message, "merge conflict: branch main")
	}
}
