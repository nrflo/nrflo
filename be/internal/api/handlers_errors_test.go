package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

func newErrorServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "errors_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	_, err = pool.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('test-proj', 'Test', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return &Server{pool: pool, clock: clock.Real()}
}

func insertTestError(t *testing.T, pool *db.Pool, id, projectID, errorType, instanceID, message, createdAt string) {
	t.Helper()
	_, err := pool.Exec(
		`INSERT INTO errors (id, project_id, error_type, instance_id, message, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, projectID, errorType, instanceID, message, createdAt,
	)
	if err != nil {
		t.Fatalf("insertTestError(%s): %v", id, err)
	}
}

func TestHandleListErrors_MissingProject(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/errors", nil)
	rr := httptest.NewRecorder()
	s.handleListErrors(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project is required")
}

func TestHandleListErrors_EmptyList(t *testing.T) {
	s := newErrorServer(t)
	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/errors", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListErrors(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var resp struct {
		Errors     []interface{} `json:"errors"`
		Total      int           `json:"total"`
		Page       int           `json:"page"`
		PerPage    int           `json:"per_page"`
		TotalPages int           `json:"total_pages"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Errors == nil {
		t.Errorf("errors should not be null (want empty array)")
	}
	if len(resp.Errors) != 0 {
		t.Errorf("errors count = %d, want 0", len(resp.Errors))
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1", resp.Page)
	}
	if resp.PerPage != 20 {
		t.Errorf("per_page = %d, want 20 (default)", resp.PerPage)
	}
	if resp.TotalPages != 0 {
		t.Errorf("total_pages = %d, want 0", resp.TotalPages)
	}
}

func TestHandleListErrors_DefaultPagination(t *testing.T) {
	s := newErrorServer(t)
	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/errors", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListErrors(rr, req)

	var resp struct {
		Page    int `json:"page"`
		PerPage int `json:"per_page"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Page != 1 {
		t.Errorf("default page = %d, want 1", resp.Page)
	}
	if resp.PerPage != 20 {
		t.Errorf("default per_page = %d, want 20", resp.PerPage)
	}
}

func TestHandleListErrors_CustomPagination(t *testing.T) {
	s := newErrorServer(t)
	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/errors?page=2&per_page=5", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListErrors(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp struct {
		Page    int `json:"page"`
		PerPage int `json:"per_page"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Page != 2 {
		t.Errorf("page = %d, want 2", resp.Page)
	}
	if resp.PerPage != 5 {
		t.Errorf("per_page = %d, want 5", resp.PerPage)
	}
}

func TestHandleListErrors_PerPageCappedAt100(t *testing.T) {
	s := newErrorServer(t)
	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/errors?per_page=500", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListErrors(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp struct {
		PerPage int `json:"per_page"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.PerPage != 100 {
		t.Errorf("per_page = %d, want 100 (capped)", resp.PerPage)
	}
}

func TestHandleListErrors_WithErrors(t *testing.T) {
	s := newErrorServer(t)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		insertTestError(t, s.pool,
			fmt.Sprintf("err-%d", i), "test-proj", "agent", fmt.Sprintf("s%d", i), "error msg",
			base.Add(time.Duration(i)*time.Second).UTC().Format(time.RFC3339Nano))
	}

	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/errors?per_page=2", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListErrors(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp struct {
		Errors     []interface{} `json:"errors"`
		Total      int           `json:"total"`
		TotalPages int           `json:"total_pages"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Errors) != 2 {
		t.Errorf("errors count = %d, want 2 (per_page limit)", len(resp.Errors))
	}
	if resp.Total != 3 {
		t.Errorf("total = %d, want 3", resp.Total)
	}
	if resp.TotalPages != 2 {
		t.Errorf("total_pages = %d, want 2 (ceil(3/2))", resp.TotalPages)
	}
}

func TestHandleListErrors_TypeFilter(t *testing.T) {
	s := newErrorServer(t)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	insertTestError(t, s.pool, "e1", "test-proj", "agent", "s1", "agent err",
		base.Format(time.RFC3339Nano))
	insertTestError(t, s.pool, "e2", "test-proj", "workflow", "w1", "wf err",
		base.Add(time.Second).Format(time.RFC3339Nano))
	insertTestError(t, s.pool, "e3", "test-proj", "system", "wfi1", "sys err",
		base.Add(2*time.Second).Format(time.RFC3339Nano))

	tests := []struct {
		typ       string
		wantTotal int
	}{
		{"agent", 1},
		{"workflow", 1},
		{"system", 1},
		{"", 3},
	}

	for _, tc := range tests {
		t.Run(tc.typ, func(t *testing.T) {
			url := withProject("/api/v1/errors", "test-proj")
			if tc.typ != "" {
				url = withProject(fmt.Sprintf("/api/v1/errors?type=%s", tc.typ), "test-proj")
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			rr := httptest.NewRecorder()
			s.handleListErrors(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("[%s] status = %d, want 200", tc.typ, rr.Code)
			}
			var resp struct {
				Total int `json:"total"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("[%s] decode: %v", tc.typ, err)
			}
			if resp.Total != tc.wantTotal {
				t.Errorf("[%s] total = %d, want %d", tc.typ, resp.Total, tc.wantTotal)
			}
		})
	}
}

func TestHandleListErrors_TotalPagesComputation(t *testing.T) {
	s := newErrorServer(t)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	// 5 errors, per_page=3 → ceil(5/3)=2 pages
	for i := 0; i < 5; i++ {
		insertTestError(t, s.pool, fmt.Sprintf("err-%d", i), "test-proj", "agent",
			fmt.Sprintf("s%d", i), "msg",
			base.Add(time.Duration(i)*time.Second).UTC().Format(time.RFC3339Nano))
	}

	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/errors?per_page=3", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListErrors(rr, req)

	var resp struct {
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 5 {
		t.Errorf("total = %d, want 5", resp.Total)
	}
	if resp.TotalPages != 2 {
		t.Errorf("total_pages = %d, want 2 (ceil(5/3))", resp.TotalPages)
	}
}

func TestHandleListErrors_InvalidPageParam(t *testing.T) {
	s := newErrorServer(t)
	// Invalid page param — should fall back to default page=1
	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/errors?page=abc", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListErrors(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp struct {
		Page int `json:"page"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1 (fallback on invalid param)", resp.Page)
	}
}
