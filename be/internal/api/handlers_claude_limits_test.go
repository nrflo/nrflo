package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
)

// newClaudeLimitsServer creates a Server with a temp DB for claude-limits handler tests.
func newClaudeLimitsServer(t *testing.T) (*Server, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "claude_limits_handler_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	s := &Server{dataPath: dbPath, pool: pool, clock: clock.Real()}
	return s, dbPath
}

// TestHandleGetClaudeLimits_EmptyDB verifies all fields are null when no data has been recorded.
func TestHandleGetClaudeLimits_EmptyDB(t *testing.T) {
	s, _ := newClaudeLimitsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/claude-limits", nil)
	rr := httptest.NewRecorder()
	s.handleGetClaudeLimits(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	nullFields := []string{
		"five_hour_used_pct",
		"five_hour_resets_at",
		"seven_day_used_pct",
		"seven_day_resets_at",
		"updated_at",
	}
	for _, field := range nullFields {
		val, exists := resp[field]
		if !exists {
			t.Errorf("response missing field %q", field)
			continue
		}
		if val != nil {
			t.Errorf("resp[%q] = %v, want nil (fresh DB)", field, val)
		}
	}
}

// TestHandleGetClaudeLimits_AfterUpdate verifies correct snake_case payload after data is set.
func TestHandleGetClaudeLimits_AfterUpdate(t *testing.T) {
	s, _ := newClaudeLimitsServer(t)

	// Write limits via service (same as socket handler would).
	fixedTime := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	testClock := clock.NewTest(fixedTime)
	svc := service.NewClaudeLimitsService(s.pool, testClock)
	if err := svc.Update(service.ClaudeLimits{
		FiveHourUsedPct:  42.5,
		FiveHourResetsAt: "2026-05-11T05:00:00Z",
		SevenDayUsedPct:  75.0,
		SevenDayResetsAt: "2026-05-18T05:00:00Z",
	}); err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/claude-limits", nil)
	rr := httptest.NewRecorder()
	s.handleGetClaudeLimits(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if pct, ok := resp["five_hour_used_pct"].(float64); !ok || pct != 42.5 {
		t.Errorf("five_hour_used_pct = %v, want 42.5", resp["five_hour_used_pct"])
	}
	if v, ok := resp["five_hour_resets_at"].(string); !ok || v != "2026-05-11T05:00:00Z" {
		t.Errorf("five_hour_resets_at = %v, want 2026-05-11T05:00:00Z", resp["five_hour_resets_at"])
	}
	if pct, ok := resp["seven_day_used_pct"].(float64); !ok || pct != 75.0 {
		t.Errorf("seven_day_used_pct = %v, want 75.0", resp["seven_day_used_pct"])
	}
	if v, ok := resp["seven_day_resets_at"].(string); !ok || v != "2026-05-18T05:00:00Z" {
		t.Errorf("seven_day_resets_at = %v, want 2026-05-18T05:00:00Z", resp["seven_day_resets_at"])
	}
	wantUpdatedAt := fixedTime.UTC().Format(time.RFC3339)
	if v, ok := resp["updated_at"].(string); !ok || v != wantUpdatedAt {
		t.Errorf("updated_at = %v, want %q", resp["updated_at"], wantUpdatedAt)
	}
}

// TestHandleGetClaudeLimits_Persistence verifies data survives Server re-creation (same DB path).
// This exercises acceptance criterion 3: data persisted across server restarts.
func TestHandleGetClaudeLimits_Persistence(t *testing.T) {
	s1, dbPath := newClaudeLimitsServer(t)

	// Write limits through the first server's service.
	svc := service.NewClaudeLimitsService(s1.pool, clock.Real())
	if err := svc.Update(service.ClaudeLimits{
		FiveHourUsedPct:  88.0,
		FiveHourResetsAt: "2026-05-11T06:00:00Z",
		SevenDayUsedPct:  66.0,
		SevenDayResetsAt: "2026-05-18T06:00:00Z",
	}); err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	// Simulate server restart by opening a new pool on the same DB.
	pool2, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open second pool: %v", err)
	}
	t.Cleanup(func() { pool2.Close() })
	s2 := &Server{dataPath: dbPath, pool: pool2, clock: clock.Real()}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/claude-limits", nil)
	rr := httptest.NewRecorder()
	s2.handleGetClaudeLimits(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 after server restart simulation", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if pct, ok := resp["five_hour_used_pct"].(float64); !ok || pct != 88.0 {
		t.Errorf("five_hour_used_pct = %v, want 88.0 after persistence", resp["five_hour_used_pct"])
	}
	if pct, ok := resp["seven_day_used_pct"].(float64); !ok || pct != 66.0 {
		t.Errorf("seven_day_used_pct = %v, want 66.0 after persistence", resp["seven_day_used_pct"])
	}
	if resp["updated_at"] == nil {
		t.Error("updated_at should be non-null after persistence")
	}
}

// TestHandleGetClaudeLimits_ContentType verifies JSON content-type header.
func TestHandleGetClaudeLimits_ContentType(t *testing.T) {
	s, _ := newClaudeLimitsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/claude-limits", nil)
	rr := httptest.NewRecorder()
	s.handleGetClaudeLimits(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct == "" {
		t.Error("Content-Type header is empty")
	}
}
