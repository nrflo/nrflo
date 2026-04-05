package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

func newDailyStatsServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "daily_stats_handler_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("newDailyStatsServer: new pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	_, err = pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('ds-proj', 'DS', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("newDailyStatsServer: insert project: %v", err)
	}
	return &Server{pool: pool, clock: clock.Real()}
}

func TestHandleGetDailyStats_MissingProject(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/daily-stats", nil)
	rr := httptest.NewRecorder()
	s.handleGetDailyStats(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleGetDailyStats_InvalidRange(t *testing.T) {
	s := newDailyStatsServer(t)

	badValues := []string{"day", "yearly", "all-time", "TODAY", "Week", "WEEK", "invalid", "0"}
	for _, bad := range badValues {
		t.Run(bad, func(t *testing.T) {
			url := withProject("/api/v1/daily-stats?range="+bad, "ds-proj")
			req := httptest.NewRequest(http.MethodGet, url, nil)
			rr := httptest.NewRecorder()
			s.handleGetDailyStats(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("range=%q: status = %d, want 400", bad, rr.Code)
			}
			assertErrorContains(t, rr, "invalid range")
		})
	}
}

func TestHandleGetDailyStats_DefaultsToToday(t *testing.T) {
	s := newDailyStatsServer(t)
	// No ?range= param — should default to "today" and return 200.
	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/daily-stats", "ds-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleGetDailyStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["tickets_created"]; !ok {
		t.Error("response missing tickets_created field")
	}
}

func TestHandleGetDailyStats_ValidRanges(t *testing.T) {
	s := newDailyStatsServer(t)

	validRanges := []string{"today", "week", "month", "all"}
	for _, rangeVal := range validRanges {
		t.Run(rangeVal, func(t *testing.T) {
			url := withProject("/api/v1/daily-stats?range="+rangeVal, "ds-proj")
			req := httptest.NewRequest(http.MethodGet, url, nil)
			rr := httptest.NewRecorder()
			s.handleGetDailyStats(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("range=%q: status = %d, want 200", rangeVal, rr.Code)
			}
		})
	}
}

func TestHandleGetDailyStats_ResponseJSON(t *testing.T) {
	// Use a TestClock with a fixed date so inserted data matches the "today" query.
	fixedNow := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	dbPath := filepath.Join(t.TempDir(), "daily_stats_resp_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	pid := "resp-proj"
	now := fixedNow.Format(time.RFC3339Nano)
	_, err = pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'Resp', ?, ?)`, pid, now, now)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	// Insert 2 tickets created today.
	for _, id := range []string{"t1", "t2"} {
		_, err = pool.Exec(`INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by) VALUES (?, ?, 'T', 'open', 'feature', 2, ?, ?, 'test')`,
			pid+"-"+id, pid, now, now)
		if err != nil {
			t.Fatalf("insert ticket: %v", err)
		}
	}
	// Closed ticket: created yesterday (not today), closed today.
	yesterday := fixedNow.AddDate(0, 0, -1).Format(time.RFC3339Nano)
	closedAt := fixedNow.Add(1 * time.Hour).Format(time.RFC3339Nano)
	_, err = pool.Exec(`INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, closed_at, created_by) VALUES (?, ?, 'T', 'closed', 'feature', 2, ?, ?, ?, 'test')`,
		pid+"-tc", pid, yesterday, closedAt, closedAt)
	if err != nil {
		t.Fatalf("insert closed ticket: %v", err)
	}

	clk := clock.NewTest(fixedNow)
	s := &Server{pool: pool, clock: clk}

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/daily-stats?range=today", pid), nil)
	rr := httptest.NewRecorder()
	s.handleGetDailyStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ProjectID      string  `json:"project_id"`
		Date           string  `json:"date"`
		TicketsCreated int     `json:"tickets_created"`
		TicketsClosed  int     `json:"tickets_closed"`
		TokensSpent    int64   `json:"tokens_spent"`
		AgentTimeSec   float64 `json:"agent_time_sec"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TicketsCreated != 2 {
		t.Errorf("tickets_created = %d, want 2", resp.TicketsCreated)
	}
	if resp.TicketsClosed != 1 {
		t.Errorf("tickets_closed = %d, want 1", resp.TicketsClosed)
	}
	if resp.Date != "2026-03-20" {
		t.Errorf("date = %q, want 2026-03-20", resp.Date)
	}
	if !strings.HasPrefix(resp.ProjectID, "resp-") {
		t.Errorf("project_id = %q, unexpected value", resp.ProjectID)
	}
}

func TestHandleGetDailyStats_WeekAggregatesMultipleDays(t *testing.T) {
	fixedNow := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	dbPath := filepath.Join(t.TempDir(), "daily_stats_week_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	pid := "week-proj"
	now := fixedNow.Format(time.RFC3339Nano)
	_, err = pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'Week', ?, ?)`, pid, now, now)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	// 2 tickets today, 1 ticket 4 days ago (in range), 1 ticket 9 days ago (out of range).
	insert := func(id, createdAt string) {
		t.Helper()
		_, err := pool.Exec(`INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by) VALUES (?, ?, 'T', 'open', 'feature', 2, ?, ?, 'test')`,
			pid+"-"+id, pid, createdAt, createdAt)
		if err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insert("t1", now)
	insert("t2", now)
	insert("t3", fixedNow.AddDate(0, 0, -4).Format(time.RFC3339Nano))
	insert("t4", fixedNow.AddDate(0, 0, -9).Format(time.RFC3339Nano)) // outside week

	clk := clock.NewTest(fixedNow)
	s := &Server{pool: pool, clock: clk}

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/daily-stats?range=week", pid), nil)
	rr := httptest.NewRecorder()
	s.handleGetDailyStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		TicketsCreated int `json:"tickets_created"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TicketsCreated != 3 {
		t.Errorf("week tickets_created = %d, want 3 (today×2 + 4d ago, excluding 9d ago)", resp.TicketsCreated)
	}
}
