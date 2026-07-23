package spawner

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

// TestCodexAppServerRun_ArmedHandoff_StepwiseCursorUntouched is case 22: on a
// stepwise def's fail-restart resume path, turn/start's text must be the
// rendered crash-resume injectable (never prep.prompt — the resumed thread
// already carries the prior prompt in its own history), and the
// (workflow_instance_id, node_id)-keyed cursor row must be byte-identical
// before and after the resume, since startOrResumeThread never touches
// agent_step_cursors.
func TestCodexAppServerRun_ArmedHandoff_StepwiseCursorUntouched(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	backend := newResumeTestBackend(t)
	f := newFakeAppServer(t)
	f.install(t)

	paramsCh := make(chan struct{}, 1)
	f.setOverride("thread/resume", func(f *fakeAppServer, env rpcEnvelope) {
		paramsCh <- struct{}{}
		f.replyResult(*env.ID, `{"thread":{"id":"thread-new-stepwise"}}`)
	})

	const wfiID, nodeID = "wfi-1", "impl"
	cursorRepo := repo.NewAgentStepCursorRepo(backend.s.pool(), clock.Real())
	stepsJSON := `[{"step_id":"step-one","title":"Title One","instruction":"Instruction body one."},{"step_id":"step-two","title":"Title Two","instruction":"Instruction body two."}]`
	if err := cursorRepo.Insert(&model.AgentStepCursor{
		WorkflowInstanceID: wfiID,
		NodeID:             nodeID,
		StepsSnapshot:      stepsJSON,
		Revision:           2,
		CurrentIndex:       1,
		Completed:          `[{"step_id":"step-one","completed_at":"2026-01-01T00:00:00Z"}]`,
		Rejections:         "{}",
	}); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}
	revBefore, idxBefore, completedBefore, rejBefore := readCursorRow(t, backend.s.pool(), wfiID, nodeID)

	oldDir := t.TempDir()
	workDir := t.TempDir()
	proc := &processInfo{
		sessionID:          "sess-resume-stepwise",
		workflowInstanceID: wfiID,
		nodeID:             nodeID,
		workDir:            workDir,
		maxContext:         100000,
		doneCh:             make(chan struct{}),
		resumeHandoff:      &codexThreadHandoff{threadID: "thread-old-stepwise", profileDir: oldDir},
	}
	prep := &prepResult{opts: SpawnOptions{MappedModel: "gpt-5.6-sol", ReasoningEffort: "medium"}, prompt: "FULL ORIGINAL PROMPT BODY"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := backend.Start(ctx, proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-paramsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("thread/resume was never called")
	}

	turnEnv := waitForOutbound(t, f, "turn/start", 2*time.Second)
	gotText := decodeTurnStartText(t, turnEnv.Params)
	if gotText == prep.prompt {
		t.Error("turn/start text equals prep.prompt on the resume path; want the crash-resume injectable body")
	}
	wantText := backend.s.expandInjectable("crash-resume", map[string]string{"RESTART_REASON": "fail_restart"})
	if wantText == "" {
		t.Fatal("crash-resume injectable rendered empty")
	}
	if gotText != wantText {
		t.Errorf("turn/start text = %q, want rendered crash-resume injectable %q", gotText, wantText)
	}

	cancel()
	select {
	case <-proc.doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("proc.doneCh did not close after ctx cancel")
	}

	revAfter, idxAfter, completedAfter, rejAfter := readCursorRow(t, backend.s.pool(), wfiID, nodeID)
	if revAfter != revBefore || idxAfter != idxBefore || completedAfter != completedBefore || rejAfter != rejBefore {
		t.Errorf("cursor changed across codex crash-resume: before=(%d,%d,%q,%q) after=(%d,%d,%q,%q)",
			revBefore, idxBefore, completedBefore, rejBefore, revAfter, idxAfter, completedAfter, rejAfter)
	}
}
