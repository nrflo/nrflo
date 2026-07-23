package spawner

import (
	"context"
	"os"
	"testing"
	"time"
)

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
