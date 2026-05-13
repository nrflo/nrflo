package spawner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCodexAdapter_PostStart_Batch_PopulatesContextLeft verifies that PostStart
// launches the rollout JSONL tailer and that a token_count record (50% of
// context used) causes Sink.UpdateContextLeft to be called with pct==50.
// This exercises the cli-batch path wired from cliBackend.Start in backend_cli.go.
func TestCodexAdapter_PostStart_Batch_PopulatesContextLeft(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Resolve symlinks on workDir so the tailer's internal EvalSymlinks call
	// produces a path that matches the session_meta.payload.cwd we write below.
	rawWorkDir := t.TempDir()
	workDir, err := filepath.EvalSymlinks(rawWorkDir)
	if err != nil {
		workDir = rawWorkDir
	}

	// Write the rollout JSONL file before PostStart so the tailer discovers
	// it on the first 250 ms poll cycle.
	dir := filepath.Join(tmp, ".codex", "sessions", "2026", "05", "12")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	rolloutPath := filepath.Join(dir, "rollout-2026-05-12T00-00-00-batch.jsonl")
	// session_meta identifies our session; token_count encodes 50% used → 50% left.
	content := `{"type":"session_meta","payload":{"cwd":"` + workDir + `"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100000},"model_context_window":200000}}}` + "\n"
	if err := os.WriteFile(rolloutPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}

	sink := &codexJSONLTestSink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := &CodexAdapter{}
	cleanup, err := adapter.PostStart(ctx, PostStartOptions{
		SessionID: "sess-post-start-batch",
		WorkDir:   workDir,
		Sink:      sink,
		StartedAt: time.Now().Add(-time.Second),
	})
	if err != nil {
		t.Fatalf("PostStart: %v", err)
	}
	defer cleanup()

	// Poll until the token_count record is dispatched (max 1.5 s).
	// The tailer's internal poll cadence is 250 ms; one full cycle is sufficient.
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		sink.mu.Lock()
		updates := append([]int{}, sink.contextUpdates...)
		sink.mu.Unlock()
		for _, pct := range updates {
			if pct == 50 {
				return // success
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	sink.mu.Lock()
	got := append([]int{}, sink.contextUpdates...)
	sink.mu.Unlock()
	t.Errorf("contextUpdates = %v after 1.5s; want entry with pct==50", got)
}

// TestCodexAdapter_PostStart_EmptyWorkDir_ReturnsError verifies that PostStart
// rejects an empty WorkDir immediately, before launching any goroutine.
func TestCodexAdapter_PostStart_EmptyWorkDir_ReturnsError(t *testing.T) {
	t.Parallel()
	sink := &codexJSONLTestSink{}
	ctx := context.Background()
	adapter := &CodexAdapter{}
	_, err := adapter.PostStart(ctx, PostStartOptions{
		SessionID: "sess-empty-wd",
		WorkDir:   "",
		Sink:      sink,
	})
	if err == nil {
		t.Error("PostStart(emptyWorkDir) returned nil error, want error")
	}
}
