package spawner

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// newResumeTestBackend builds a codexAppServerBackend + Spawner wired to a
// migrated test DB (so expandInjectable("crash-resume", ...) renders the real
// seeded template, not a warning-logged "").
func newResumeTestBackend(t *testing.T) *codexAppServerBackend {
	t.Helper()
	pool := setupTestDB(t)
	s := New(Config{Clock: clock.NewTest(time.Now()), Pool: pool})
	return newCodexAppServerBackend(s)
}

type turnStartInput struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func decodeTurnStartText(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var p struct {
		Input []turnStartInput `json:"input"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode turn/start params: %v", err)
	}
	if len(p.Input) != 1 {
		t.Fatalf("turn/start input blocks = %d, want 1", len(p.Input))
	}
	return p.Input[0].Text
}

// TestCodexAppServerRun_FreshSpawn_NoHandoff verifies the wire sequence for a
// fresh spawn (no resumeHandoff armed): initialize -> thread/start ->
// turn/start, turn/start carrying prep.prompt verbatim, and no thread/resume
// call anywhere on the wire.
func TestCodexAppServerRun_FreshSpawn_NoHandoff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	backend := newResumeTestBackend(t)
	f := newFakeAppServer(t)
	f.install(t)

	proc := &processInfo{sessionID: "sess-fresh-1", workDir: t.TempDir(), maxContext: 100000, doneCh: make(chan struct{})}
	prep := &prepResult{opts: SpawnOptions{MappedModel: "gpt-5.6-sol", ReasoningEffort: "medium"}, prompt: "FULL ORIGINAL PROMPT BODY"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := backend.Start(ctx, proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var methods []string
	deadline := time.After(2 * time.Second)
	for len(methods) < 4 {
		select {
		case env := <-f.outbound:
			methods = append(methods, env.Method)
			if env.Method == "turn/start" {
				if got := decodeTurnStartText(t, env.Params); got != prep.prompt {
					t.Errorf("turn/start text = %q, want prep.prompt %q", got, prep.prompt)
				}
			}
		case <-deadline:
			t.Fatalf("only observed %v before timeout", methods)
		}
	}
	want := []string{"initialize", "initialized", "thread/start", "turn/start"}
	for i, m := range want {
		if methods[i] != m {
			t.Errorf("methods[%d] = %q, want %q (all: %v)", i, methods[i], m, methods)
		}
	}
	for _, m := range methods {
		if m == "thread/resume" {
			t.Fatalf("fresh spawn must never call thread/resume; wire=%v", methods)
		}
	}
	if proc.externalSessionID == "" {
		t.Error("externalSessionID not set after fresh spawn")
	}

	cancel()
	select {
	case <-proc.doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("proc.doneCh did not close after ctx cancel")
	}
}

// TestCodexAppServerRun_ArmedHandoff_ResumesThread verifies an armed
// resumeHandoff drives thread/resume (not thread/start), with threadId/cwd/
// sandbox/model in the params, and that the first turn's text is the rendered
// crash-resume injectable rather than prep.prompt.
func TestCodexAppServerRun_ArmedHandoff_ResumesThread(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	backend := newResumeTestBackend(t)
	f := newFakeAppServer(t)
	f.install(t)

	paramsCh := make(chan json.RawMessage, 1)
	f.setOverride("thread/resume", func(f *fakeAppServer, env rpcEnvelope) {
		paramsCh <- env.Params
		f.replyResult(*env.ID, `{"thread":{"id":"thread-new-1"}}`)
	})

	oldDir := t.TempDir()
	workDir := t.TempDir()
	proc := &processInfo{
		sessionID:     "sess-resume-armed",
		workDir:       workDir,
		maxContext:    100000,
		doneCh:        make(chan struct{}),
		resumeHandoff: &codexThreadHandoff{threadID: "thread-old-1", profileDir: oldDir},
	}
	prep := &prepResult{opts: SpawnOptions{MappedModel: "gpt-5.6-sol", ReasoningEffort: "medium"}, prompt: "FULL ORIGINAL PROMPT BODY"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := backend.Start(ctx, proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	raw := mustRecvParams(t, paramsCh)
	var p struct {
		ThreadID string `json:"threadId"`
		Model    string `json:"model"`
		Cwd      string `json:"cwd"`
		Sandbox  string `json:"sandbox"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode thread/resume params: %v", err)
	}
	if p.ThreadID != "thread-old-1" {
		t.Errorf("thread/resume threadId = %q, want thread-old-1", p.ThreadID)
	}
	if p.Cwd != workDir {
		t.Errorf("thread/resume cwd = %q, want %q", p.Cwd, workDir)
	}
	if p.Sandbox != model.SandboxDangerFullAccess {
		t.Errorf("thread/resume sandbox = %q, want %q", p.Sandbox, model.SandboxDangerFullAccess)
	}
	if p.Model != "gpt-5.6-sol" {
		t.Errorf("thread/resume model = %q, want gpt-5.6-sol", p.Model)
	}

	turnEnv := waitForOutbound(t, f, "turn/start", 2*time.Second)
	gotText := decodeTurnStartText(t, turnEnv.Params)
	if gotText == prep.prompt {
		t.Error("turn/start text equals prep.prompt on the resume path; want the crash-resume injectable body")
	}
	wantText := backend.s.expandInjectable("crash-resume", map[string]string{"RESTART_REASON": "fail_restart"})
	if wantText == "" {
		t.Fatal("crash-resume injectable rendered empty; migration 000199 seed missing from test DB")
	}
	if gotText != wantText {
		t.Errorf("turn/start text = %q, want rendered crash-resume injectable %q", gotText, wantText)
	}

	if proc.externalSessionID != "thread-new-1" {
		t.Errorf("externalSessionID = %q, want thread-new-1 (from thread/resume response)", proc.externalSessionID)
	}

	// No thread/start must ever have been sent on this path.
	select {
	case env := <-f.outbound:
		if env.Method == "thread/start" {
			t.Error("armed handoff must not call thread/start")
		}
	default:
	}

	cancel()
	select {
	case <-proc.doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("proc.doneCh did not close after ctx cancel")
	}

	// Ownership of oldDir moved to the (new) handoff — it must still exist.
	if _, err := os.Stat(oldDir); err != nil {
		t.Errorf("profile dir removed after a successful resume run: %v", err)
	}
}

// TestCodexAppServerRun_ResumeRejected_FallsBackToThreadStart verifies a
// thread/resume rpc error (e.g. "no rollout found for thread id ...") falls
// through to thread/start with the FULL prep.prompt, and the session does not
// fail.
func TestCodexAppServerRun_ResumeRejected_FallsBackToThreadStart(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	backend := newResumeTestBackend(t)
	f := newFakeAppServer(t)
	f.install(t)
	f.threadID = "thread-fallback-1"

	f.setOverride("thread/resume", func(f *fakeAppServer, env rpcEnvelope) {
		f.replyError(*env.ID, -32600, "no rollout found for thread id thread-dead-1")
	})

	oldDir := t.TempDir()
	proc := &processInfo{
		sessionID:     "sess-resume-rejected",
		workDir:       t.TempDir(),
		maxContext:    100000,
		doneCh:        make(chan struct{}),
		resumeHandoff: &codexThreadHandoff{threadID: "thread-dead-1", profileDir: oldDir},
	}
	prep := &prepResult{opts: SpawnOptions{MappedModel: "gpt-5.6-sol", ReasoningEffort: "medium"}, prompt: "FULL ORIGINAL PROMPT BODY"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := backend.Start(ctx, proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Fallback must still reach thread/start.
	waitForOutbound(t, f, "thread/start", 2*time.Second)
	turnEnv := waitForOutbound(t, f, "turn/start", 2*time.Second)
	if got := decodeTurnStartText(t, turnEnv.Params); got != prep.prompt {
		t.Errorf("turn/start text = %q, want the FULL prep.prompt %q (fallback path)", got, prep.prompt)
	}
	if proc.externalSessionID != "thread-fallback-1" {
		t.Errorf("externalSessionID = %q, want thread-fallback-1 (fresh thread/start id)", proc.externalSessionID)
	}
	if proc.waitErr != nil {
		t.Errorf("proc.waitErr = %v, want nil — a rejected resume must not fail the session", proc.waitErr)
	}

	cancel()
	select {
	case <-proc.doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("proc.doneCh did not close after ctx cancel")
	}
}

// TestCodexAppServerRun_ProfileDir_RemovedOnlyWhenNoHandoffArmed is the
// ownership-transfer regression guard: a run that never arms a handoff (fails
// before reaching that point) must clean up its temp profile dir, while a run
// that arms one (the normal case, fresh or resumed) must leave the dir for the
// handoff's owner.
func TestCodexAppServerRun_ProfileDir_RemovedOnlyWhenNoHandoffArmed(t *testing.T) {
	t.Run("handoff armed: dir survives", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		backend := newResumeTestBackend(t)
		f := newFakeAppServer(t)
		f.install(t)

		proc := &processInfo{sessionID: "sess-dir-survive", workDir: t.TempDir(), maxContext: 100000, doneCh: make(chan struct{})}
		prep := &prepResult{opts: SpawnOptions{MappedModel: "gpt-5.6-sol", ReasoningEffort: "medium"}, prompt: "prompt"}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := backend.Start(ctx, proc, prep); err != nil {
			t.Fatalf("Start: %v", err)
		}
		waitForOutbound(t, f, "turn/start", 2*time.Second)
		dir := backend.profileDir

		cancel()
		select {
		case <-proc.doneCh:
		case <-time.After(2 * time.Second):
			t.Fatal("proc.doneCh did not close")
		}
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("profile dir removed despite an armed handoff: %v", err)
		}
	})

	t.Run("no handoff armed (thread/start fails): dir removed", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		backend := newResumeTestBackend(t)
		f := newFakeAppServer(t)
		f.install(t)
		// resumeHandoff is armed right before turn/start (see run()), so a
		// turn/start failure alone leaves ownership already transferred; to
		// exercise the "never armed" branch the failure must happen in
		// startOrResumeThread itself (thread/start, since there is no
		// resumeHandoff to attempt a resume on).
		f.setOverride("thread/start", func(f *fakeAppServer, env rpcEnvelope) {
			f.replyError(*env.ID, -32000, "boom")
		})

		proc := &processInfo{sessionID: "sess-dir-removed", workDir: t.TempDir(), maxContext: 100000, doneCh: make(chan struct{})}
		prep := &prepResult{opts: SpawnOptions{MappedModel: "gpt-5.6-sol", ReasoningEffort: "medium"}, prompt: "prompt"}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := backend.Start(ctx, proc, prep); err != nil {
			t.Fatalf("Start: %v", err)
		}
		select {
		case <-proc.doneCh:
		case <-time.After(2 * time.Second):
			t.Fatal("proc.doneCh did not close after thread/start failure")
		}
		dir := backend.profileDir
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("profile dir still exists after thread/start failure with no handoff armed: err=%v", err)
		}
		if proc.waitErr == nil {
			t.Error("proc.waitErr = nil, want a thread/start failure error")
		}
		if proc.resumeHandoff != nil {
			t.Error("resumeHandoff set despite thread/start failing before it is armed")
		}
	})
}
