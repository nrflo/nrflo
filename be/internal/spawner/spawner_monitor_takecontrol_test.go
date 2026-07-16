package spawner

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ptyPkg "be/internal/pty"

	"be/internal/clock"
	"be/internal/ws"
)

// ── canResumeTakeControl ─────────────────────────────────────────────────────

func TestCanResumeTakeControl(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	resumableBackend := newCLIInteractiveBackend(&ClaudeAdapter{}, sp, newMockPtyManager())
	nonResumableBackend := newAPIBackend(sp)

	cases := []struct {
		name string
		proc *processInfo
		want bool
	}{
		{"nil backend", &processInfo{backend: nil, adapter: &ClaudeAdapter{}, sessionID: "s1"}, false},
		{"backend does not support resume", &processInfo{backend: nonResumableBackend, adapter: &ClaudeAdapter{}, sessionID: "s1"}, false},
		{"nil adapter", &processInfo{backend: resumableBackend, adapter: nil, sessionID: "s1"}, false},
		{"claude adapter, empty session ID", &processInfo{backend: resumableBackend, adapter: &ClaudeAdapter{}, sessionID: ""}, false},
		{"claude adapter, session ID set", &processInfo{backend: resumableBackend, adapter: &ClaudeAdapter{}, sessionID: "s1"}, true},
		{"codex adapter, empty external session ID", &processInfo{backend: resumableBackend, adapter: &CodexAdapter{}, externalSessionID: ""}, false},
		{"codex adapter, external session ID set", &processInfo{backend: resumableBackend, adapter: &CodexAdapter{}, externalSessionID: "ext-1"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canResumeTakeControl(tc.proc); got != tc.want {
				t.Errorf("canResumeTakeControl() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ── rejectTakeControl ─────────────────────────────────────────────────────────

// TestRejectTakeControl_BroadcastsAndUnblocksReadiness verifies rejectTakeControl
// broadcasts EventAgentTakeControlRejected with the given reason and closes the
// WaitForTakeControlReady channel for the session.
func TestRejectTakeControl_BroadcastsAndUnblocksReadiness(t *testing.T) {
	t.Parallel()
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "ws-tc-reject")
	hub.Register(client)
	hub.Subscribe(client, "proj-reject", "T-1")

	sp := New(Config{Clock: clock.Real(), WSHub: hub})
	sp.RequestTakeControl("sess-reject")

	// Grab the readiness channel directly (white-box: same package) so the
	// assertion isn't racing rejectTakeControl's own map lookup/delete.
	sp.takeControlReadiesMu.Lock()
	readyCh, ok := sp.takeControlReadies["sess-reject"]
	sp.takeControlReadiesMu.Unlock()
	if !ok {
		t.Fatal("RequestTakeControl did not register a readiness channel")
	}

	proc := &processInfo{sessionID: "sess-reject", agentType: "implementor", modelID: "claude:sonnet-5"}
	req := SpawnRequest{ProjectID: "proj-reject", TicketID: "T-1", WorkflowName: "feature"}

	sp.rejectTakeControl(req, proc, "sess-reject", "resume_unsupported")

	select {
	case <-readyCh:
		// expected — closed by rejectTakeControl
	case <-time.After(2 * time.Second):
		t.Fatal("readiness channel not closed after rejectTakeControl")
	}

	sp.takeControlReadiesMu.Lock()
	_, stillPresent := sp.takeControlReadies["sess-reject"]
	sp.takeControlReadiesMu.Unlock()
	if stillPresent {
		t.Error("readiness entry still present in map after rejectTakeControl")
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			var event ws.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				continue
			}
			if event.Type != ws.EventAgentTakeControlRejected {
				continue
			}
			if reason, _ := event.Data["reason"].(string); reason != "resume_unsupported" {
				t.Errorf("reason = %q, want resume_unsupported", reason)
			}
			if sessID, _ := event.Data["session_id"].(string); sessID != "sess-reject" {
				t.Errorf("session_id = %q, want sess-reject", sessID)
			}
			return
		case <-deadline:
			t.Fatal("timeout waiting for EventAgentTakeControlRejected")
		}
	}
}

// ── registerTakeControlResumeLaunch ───────────────────────────────────────────

// TestRegisterTakeControlResumeLaunch_NilPTYManager_NoPanic verifies the
// early-return when Config.PTYManager is nil.
func TestRegisterTakeControlResumeLaunch_NilPTYManager_NoPanic(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	proc := &processInfo{sessionID: "sess-no-mgr", adapter: &ClaudeAdapter{}, modelID: "claude:sonnet-5"}
	sp.registerTakeControlResumeLaunch(proc) // must not panic
}

// TestRegisterTakeControlResumeLaunch_RegistersClaudeResumeLaunch verifies the
// adapter-built resume command is registered with the PTY manager under the
// session ID, using the DB-sourced mapped model when present.
func TestRegisterTakeControlResumeLaunch_RegistersClaudeResumeLaunch(t *testing.T) {
	t.Parallel()
	ptyMgr := ptyPkg.NewManager()
	sp := New(Config{
		Clock:      clock.Real(),
		PTYManager: ptyMgr,
		ModelConfigs: map[string]ModelConfig{
			"sonnet-5": {Provider: "anthropic", CLIModel: "claude-sonnet-mapped"},
		},
	})
	proc := &processInfo{
		sessionID: "sess-resume-1",
		adapter:   &ClaudeAdapter{},
		modelID:   "claude:sonnet-5",
		workDir:   "/tmp/resume-workdir",
	}

	sp.registerTakeControlResumeLaunch(proc)

	// Inspect the pending launch rather than Create()-ing it: Create would fork
	// a real claude CLI in a PTY, which the test suite forbids.
	launch, ok := ptyMgr.PendingLaunch("sess-resume-1")
	if !ok {
		t.Fatal("registerTakeControlResumeLaunch did not register a launch")
	}
	// exec.Command resolves a PATH lookup to an absolute path.
	if filepath.Base(launch.Command) != "claude" {
		t.Errorf("launch.Command = %q, want claude", launch.Command)
	}

	args := strings.Join(launch.Args, " ")
	if !strings.Contains(args, "--resume sess-resume-1") {
		t.Errorf("launch args missing --resume for the session: %s", args)
	}
	if !strings.Contains(args, "--model claude-sonnet-mapped") {
		t.Errorf("launch args missing the DB-sourced mapped model: %s", args)
	}
	// The CLI rejects --session-id alongside --resume.
	if strings.Contains(args, "--session-id") {
		t.Errorf("resume launch must not pass --session-id: %s", args)
	}
	if launch.Dir != "/tmp/resume-workdir" {
		t.Errorf("launch.Dir = %q, want /tmp/resume-workdir", launch.Dir)
	}
}
