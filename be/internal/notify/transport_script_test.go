package notify

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

func skipIfNoPython3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
}

// withScriptRuntime sets scriptRuntime and restores the previous value on cleanup.
func withScriptRuntime(t *testing.T, rt *ScriptRuntime) {
	t.Helper()
	old := scriptRuntime
	scriptRuntime = rt
	t.Cleanup(func() { scriptRuntime = old })
}

func TestScriptTransport_Registered(t *testing.T) {
	tr := Get("script")
	if tr == nil {
		t.Fatal("script transport not registered")
	}
	if tr.Kind() != "script" {
		t.Errorf("Kind() = %q, want script", tr.Kind())
	}
}

func TestScriptTransport_RuntimeNotInitialized_ReturnsError(t *testing.T) {
	withScriptRuntime(t, nil)
	tr := Get("script")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"script_code": "print(1)"},
	})
	if err == nil {
		t.Fatal("Send with no runtime: expected error, got nil")
	}
}

func TestScriptTransport_EmptyScriptCode_ReturnsError(t *testing.T) {
	withScriptRuntime(t, &ScriptRuntime{NrfloHome: t.TempDir(), Clock: clock.Real()})
	tr := Get("script")
	err := tr.Send(&Notification{
		ChannelID: "ch-empty",
		Config:    map[string]interface{}{"script_code": ""},
	})
	if err == nil {
		t.Fatal("Send with empty script_code: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "script_code") {
		t.Errorf("error = %q, want to contain 'script_code'", err.Error())
	}
}

func TestScriptTransport_Success_ExitsZero(t *testing.T) {
	skipIfNoPython3(t)
	withScriptRuntime(t, &ScriptRuntime{NrfloHome: t.TempDir(), Clock: clock.Real()})

	tr := Get("script")
	err := tr.Send(&Notification{
		ChannelID:  "ch-ok",
		DeliveryID: "del-ok",
		Config:     map[string]interface{}{"script_code": "import sys; sys.stderr.write('sentinel\\n')"},
		ProjectID:  "proj-ok",
	})
	if err != nil {
		t.Errorf("Send(exit 0) = %v, want nil", err)
	}
}

func TestScriptTransport_NonZeroExit_ReturnsErrorWithStderr(t *testing.T) {
	skipIfNoPython3(t)
	withScriptRuntime(t, &ScriptRuntime{NrfloHome: t.TempDir(), Clock: clock.Real()})

	code := `import sys
sys.stderr.write("line1\n")
sys.stderr.write("line2\n")
sys.stderr.write("last_line\n")
sys.exit(1)`

	tr := Get("script")
	err := tr.Send(&Notification{
		ChannelID:  "ch-fail",
		DeliveryID: "del-fail",
		Config:     map[string]interface{}{"script_code": code},
		ProjectID:  "proj-fail",
	})
	if err == nil {
		t.Fatal("Send(exit 1): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "last_line") {
		t.Errorf("error = %q, want to contain 'last_line'", err.Error())
	}
}

func TestScriptTransport_NoSession_EmptyInstanceID(t *testing.T) {
	skipIfNoPython3(t)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-nosess', 'Test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}

	clk := clock.Real()
	withScriptRuntime(t, &ScriptRuntime{
		NrfloHome:   t.TempDir(),
		Clock:       clk,
		SessionRepo: repo.NewAgentSessionRepo(pool, clk),
	})

	tr := Get("script")
	sendErr := tr.Send(&Notification{
		ChannelID:  "ch-nosess",
		DeliveryID: "del-nosess",
		Config:     map[string]interface{}{"script_code": "import sys; sys.stderr.write('ran\\n')"},
		ProjectID:  "proj-nosess",
		InstanceID: "", // empty → StartNotifySession is a no-op
	})
	if sendErr != nil {
		t.Errorf("Send(no instanceID): %v", sendErr)
	}

	var count int
	row := pool.QueryRow(`SELECT COUNT(*) FROM agent_sessions WHERE agent_type = '_notification'`)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 0 {
		t.Errorf("_notification sessions = %d, want 0 (empty instanceID skips session)", count)
	}
}

func TestScriptTransport_WithSession_CreatesAndCompletes(t *testing.T) {
	skipIfNoPython3(t)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, q := range []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-sess', 'Test', datetime('now'), datetime('now'))`,
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('wf-sess', 'proj-sess', '', datetime('now'), datetime('now'))`,
		`INSERT INTO workflow_instances (id, project_id, workflow_id, scope_type, status, created_at, updated_at)
		 VALUES ('wfi-sess', 'proj-sess', 'wf-sess', 'project', 'active', datetime('now'), datetime('now'))`,
	} {
		if _, err := pool.Exec(q); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	clk := clock.Real()
	withScriptRuntime(t, &ScriptRuntime{
		NrfloHome:   t.TempDir(),
		Clock:       clk,
		SessionRepo: repo.NewAgentSessionRepo(pool, clk),
	})

	tr := Get("script")
	sendErr := tr.Send(&Notification{
		ChannelID:  "ch-sess",
		DeliveryID: "del-sess",
		Config:     map[string]interface{}{"script_code": "import sys; sys.stderr.write('script-ran\\n')"},
		ProjectID:  "proj-sess",
		InstanceID: "wfi-sess",
	})
	if sendErr != nil {
		t.Errorf("Send with session: %v", sendErr)
	}

	var status string
	row := pool.QueryRow(`SELECT status FROM agent_sessions WHERE agent_type = '_notification' LIMIT 1`)
	if err := row.Scan(&status); err != nil {
		t.Fatalf("scan session: %v (session not created?)", err)
	}
	if status != string(model.AgentSessionCompleted) {
		t.Errorf("session status = %q, want %q", status, model.AgentSessionCompleted)
	}
}
