package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

type liveTestEnv struct {
	pool  *db.Pool
	projID string
	wfiID  string
}

func setupLiveTestEnv(t *testing.T) *liveTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "live_svc_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExecLiveSvc(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('live-proj', 'P', ?, ?)`, now, now)
	mustExecLiveSvc(t, pool, `INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('live-proj', 'live-wf', '', 'project', ?, ?)`, now, now)
	mustExecLiveSvc(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES ('live-wfi', 'live-proj', '', 'live-wf', 'active', 'project', ?, ?)`, now, now)

	return &liveTestEnv{pool: pool, projID: "live-proj", wfiID: "live-wfi"}
}

func mustExecLiveSvc(t *testing.T, pool *db.Pool, q string, args ...interface{}) {
	t.Helper()
	if _, err := pool.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func insertLiveSvcSession(t *testing.T, env *liveTestEnv, id, status string, pid interface{}, startedAt time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	started := startedAt.UTC().Format(time.RFC3339Nano)
	mustExecLiveSvc(t, env.pool, `
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, pid, started_at, created_at, updated_at)
		VALUES (?, ?, '', ?, 'ph', 'ag', ?, ?, ?, ?, ?)`,
		id, env.projID, env.wfiID, status, pid, started, now, now)
}

func stubPidAliveTrue(int64) bool                                     { return true }
func stubPidAliveFalse(int64) bool                                    { return false }
func stubPidMetrics(int64) (int64, float64, int64, bool)              { return 1024, 12.5, 30, true }
func stubPidMetricsZero(int64) (int64, float64, int64, bool)          { return 0, 0, 0, false }

func TestListLive_PidAliveFalse_DropsRow(t *testing.T) {
	t.Parallel()
	env := setupLiveTestEnv(t)
	base := time.Now().UTC()
	insertLiveSvcSession(t, env, "s-drop", "running", int64(9999), base)

	clk := clock.NewTest(base.Add(time.Minute))
	svc := NewAgentSessionLogService(env.pool, clk)
	svc.pidAlive = stubPidAliveFalse
	svc.pidMetrics = stubPidMetricsZero

	sessions, err := svc.ListLive(env.projID)
	if err != nil {
		t.Fatalf("ListLive: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("len(sessions) = %d, want 0 (pidAlive=false drops row)", len(sessions))
	}
}

func TestListLive_PidAliveTrue_IncludesRow(t *testing.T) {
	t.Parallel()
	env := setupLiveTestEnv(t)
	base := time.Now().UTC()
	insertLiveSvcSession(t, env, "s-include", "running", int64(8888), base)

	clk := clock.NewTest(base.Add(time.Minute))
	svc := NewAgentSessionLogService(env.pool, clk)
	svc.pidAlive = stubPidAliveTrue
	svc.pidMetrics = stubPidMetrics

	sessions, err := svc.ListLive(env.projID)
	if err != nil {
		t.Fatalf("ListLive: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	s := sessions[0]
	if s.SessionID != "s-include" {
		t.Errorf("SessionID = %q, want s-include", s.SessionID)
	}
	if s.PID != 8888 {
		t.Errorf("PID = %d, want 8888", s.PID)
	}
}

func TestListLive_MetricsPassthrough(t *testing.T) {
	t.Parallel()
	env := setupLiveTestEnv(t)
	base := time.Now().UTC()
	insertLiveSvcSession(t, env, "s-metrics", "running", int64(7777), base)

	clk := clock.NewTest(base.Add(10 * time.Second))
	svc := NewAgentSessionLogService(env.pool, clk)
	svc.pidAlive = stubPidAliveTrue
	svc.pidMetrics = func(int64) (int64, float64, int64, bool) {
		return 2048, 55.5, 120, true
	}

	sessions, err := svc.ListLive(env.projID)
	if err != nil {
		t.Fatalf("ListLive: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	s := sessions[0]
	if s.RssKB != 2048 {
		t.Errorf("RssKB = %d, want 2048", s.RssKB)
	}
	if s.CpuPct != 55.5 {
		t.Errorf("CpuPct = %f, want 55.5", s.CpuPct)
	}
	if s.OsUptimeSec != 120 {
		t.Errorf("OsUptimeSec = %d, want 120", s.OsUptimeSec)
	}
}

func TestListLive_DurationSec_UsesTestClock(t *testing.T) {
	t.Parallel()
	env := setupLiveTestEnv(t)
	startedAt := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	insertLiveSvcSession(t, env, "s-dur", "running", int64(6666), startedAt)

	now := startedAt.Add(90 * time.Second)
	clk := clock.NewTest(now)
	svc := NewAgentSessionLogService(env.pool, clk)
	svc.pidAlive = stubPidAliveTrue
	svc.pidMetrics = stubPidMetrics

	sessions, err := svc.ListLive(env.projID)
	if err != nil {
		t.Fatalf("ListLive: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	want := 90.0
	if sessions[0].DurationSec < want-0.1 || sessions[0].DurationSec > want+0.1 {
		t.Errorf("DurationSec = %f, want ~%f", sessions[0].DurationSec, want)
	}
}

func TestListLive_Empty(t *testing.T) {
	t.Parallel()
	env := setupLiveTestEnv(t)

	svc := NewAgentSessionLogService(env.pool, clock.Real())
	svc.pidAlive = stubPidAliveTrue
	svc.pidMetrics = stubPidMetrics

	sessions, err := svc.ListLive(env.projID)
	if err != nil {
		t.Fatalf("ListLive: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("len(sessions) = %d, want 0 (empty DB)", len(sessions))
	}
}

func TestListLive_CompletedSessionExcluded(t *testing.T) {
	t.Parallel()
	env := setupLiveTestEnv(t)
	base := time.Now().UTC()
	insertLiveSvcSession(t, env, "s-done", "completed", int64(5555), base)

	svc := NewAgentSessionLogService(env.pool, clock.Real())
	svc.pidAlive = stubPidAliveTrue
	svc.pidMetrics = stubPidMetrics

	sessions, err := svc.ListLive(env.projID)
	if err != nil {
		t.Fatalf("ListLive: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("len(sessions) = %d, want 0 (completed excluded by repo)", len(sessions))
	}
}

func TestListLive_WorkflowFields(t *testing.T) {
	t.Parallel()
	env := setupLiveTestEnv(t)
	base := time.Now().UTC()
	insertLiveSvcSession(t, env, "s-wf", "running", int64(4444), base)

	clk := clock.NewTest(base.Add(5 * time.Second))
	svc := NewAgentSessionLogService(env.pool, clk)
	svc.pidAlive = stubPidAliveTrue
	svc.pidMetrics = stubPidMetrics

	sessions, err := svc.ListLive(env.projID)
	if err != nil {
		t.Fatalf("ListLive: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	s := sessions[0]
	if s.WorkflowID != "live-wf" {
		t.Errorf("WorkflowID = %q, want live-wf", s.WorkflowID)
	}
	if s.WorkflowInstanceID != env.wfiID {
		t.Errorf("WorkflowInstanceID = %q, want %q", s.WorkflowInstanceID, env.wfiID)
	}
	if s.ProjectID != env.projID {
		t.Errorf("ProjectID = %q, want %q", s.ProjectID, env.projID)
	}
}

func TestListLive_UserInteractiveIncluded(t *testing.T) {
	t.Parallel()
	env := setupLiveTestEnv(t)
	base := time.Now().UTC()
	insertLiveSvcSession(t, env, "s-ui", "user_interactive", int64(3333), base)

	svc := NewAgentSessionLogService(env.pool, clock.NewTest(base.Add(time.Second)))
	svc.pidAlive = stubPidAliveTrue
	svc.pidMetrics = stubPidMetrics

	sessions, err := svc.ListLive(env.projID)
	if err != nil {
		t.Fatalf("ListLive: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1 (user_interactive included)", len(sessions))
	}
	if sessions[0].PID != 3333 {
		t.Errorf("PID = %d, want 3333", sessions[0].PID)
	}
}
