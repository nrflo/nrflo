package spawner

import (
	"strings"
	"testing"

	ptyPkg "be/internal/pty"

	"be/internal/clock"
)

// TestCanResumeTakeControl_CodexAppServerBackend verifies canResumeTakeControl
// against the real codexAppServerBackend (not the generic cliInteractive
// stand-in): true when externalSessionID (the codex thread id) is set, false
// when empty — the concrete regression target of the resume feature.
func TestCanResumeTakeControl_CodexAppServerBackend(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	backend := newCodexAppServerBackend(sp)

	cases := []struct {
		name              string
		externalSessionID string
		want              bool
	}{
		{"empty external session ID", "", false},
		{"external session ID set", "thread-abc-1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proc := &processInfo{backend: backend, adapter: &CodexAdapter{}, externalSessionID: tc.externalSessionID}
			if got := canResumeTakeControl(proc); got != tc.want {
				t.Errorf("canResumeTakeControl() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRegisterTakeControlResumeLaunch_CodexResumeLaunch_CarriesCodexHome is
// the regression guard for the whole feature: without CODEX_HOME the PTY
// resume launch would attach against the viewer's real ~/.codex and find
// nothing. Verifies the registered launch's argv is `resume <thread_id>
// --model <mapped> --dangerously-bypass-approvals-and-sandbox
// --dangerously-bypass-hook-trust` and its Env carries the live per-session
// CODEX_HOME dir (sourced from the backend's interactiveHandoff.TakeControlExtras()).
func TestRegisterTakeControlResumeLaunch_CodexResumeLaunch_CarriesCodexHome(t *testing.T) {
	t.Parallel()
	ptyMgr := ptyPkg.NewManager()
	liveCodexHome := "/tmp/nrflo-codex-live-profile"
	sp := New(Config{
		Clock:      clock.Real(),
		PTYManager: ptyMgr,
		ModelConfigs: map[string]ModelConfig{
			"gpt-5.6-sol": {Provider: "openai", CLIModel: "gpt-5.6-sol-mapped", DefaultEffort: "medium"},
		},
	})
	backend := &codexAppServerBackend{s: sp, profileDir: liveCodexHome}

	proc := &processInfo{
		sessionID:         "sess-codex-tc-1",
		adapter:           &CodexAdapter{},
		backend:           backend,
		modelID:           "codex:gpt-5.6-sol",
		externalSessionID: "thread-live-1",
		workDir:           "/tmp/resume-codex-workdir",
	}

	sp.registerTakeControlResumeLaunch(proc)

	launch, ok := ptyMgr.PendingLaunch("sess-codex-tc-1")
	if !ok {
		t.Fatal("registerTakeControlResumeLaunch did not register a launch")
	}

	args := strings.Join(launch.Args, " ")
	if !strings.Contains(args, "resume thread-live-1") {
		t.Errorf("launch args missing 'resume thread-live-1': %s", args)
	}
	if !strings.Contains(args, "--model gpt-5.6-sol-mapped") {
		t.Errorf("launch args missing the DB-sourced mapped model: %s", args)
	}
	if !strings.Contains(args, "--dangerously-bypass-approvals-and-sandbox") {
		t.Errorf("launch args missing --dangerously-bypass-approvals-and-sandbox: %s", args)
	}
	if !strings.Contains(args, "--dangerously-bypass-hook-trust") {
		t.Errorf("launch args missing --dangerously-bypass-hook-trust (CodexHome is set, nrflo owns the profile): %s", args)
	}

	var gotCodexHome string
	for _, e := range launch.Env {
		if strings.HasPrefix(e, "CODEX_HOME=") {
			gotCodexHome = strings.TrimPrefix(e, "CODEX_HOME=")
		}
	}
	if gotCodexHome != liveCodexHome {
		t.Errorf("launch.Env CODEX_HOME = %q, want the live profile dir %q", gotCodexHome, liveCodexHome)
	}
	if launch.Dir != "/tmp/resume-codex-workdir" {
		t.Errorf("launch.Dir = %q, want /tmp/resume-codex-workdir", launch.Dir)
	}
}

// TestRegisterTakeControlResumeLaunch_CodexResumeLaunch_NoBackendExtras_NoCodexHome
// verifies a codex resume launch registered for a backend that never armed a
// live profile (empty TakeControlExtras) carries no CODEX_HOME and therefore
// no --dangerously-bypass-hook-trust (the trust-leak guard).
func TestRegisterTakeControlResumeLaunch_CodexResumeLaunch_NoBackendExtras_NoCodexHome(t *testing.T) {
	t.Parallel()
	ptyMgr := ptyPkg.NewManager()
	sp := New(Config{Clock: clock.Real(), PTYManager: ptyMgr})
	backend := &codexAppServerBackend{s: sp} // profileDir empty -> TakeControlExtras returns ok=false

	proc := &processInfo{
		sessionID:         "sess-codex-tc-2",
		adapter:           &CodexAdapter{},
		backend:           backend,
		modelID:           "codex:gpt-5.6-sol",
		externalSessionID: "thread-live-2",
		workDir:           "/tmp",
	}

	sp.registerTakeControlResumeLaunch(proc)

	launch, ok := ptyMgr.PendingLaunch("sess-codex-tc-2")
	if !ok {
		t.Fatal("registerTakeControlResumeLaunch did not register a launch")
	}
	for _, e := range launch.Env {
		if strings.HasPrefix(e, "CODEX_HOME=") {
			t.Errorf("launch.Env carries CODEX_HOME=%q despite an empty TakeControlExtras", e)
		}
	}
	args := strings.Join(launch.Args, " ")
	if strings.Contains(args, "--dangerously-bypass-hook-trust") {
		t.Errorf("launch args must not bypass hook trust with no CodexHome: %s", args)
	}
}
