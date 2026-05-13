package spawner

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestOpencodeAdapter_PostStart_EmptyWorkDir_ReturnsError verifies that PostStart
// rejects an empty WorkDir immediately without launching any goroutine.
func TestOpencodeAdapter_PostStart_EmptyWorkDir_ReturnsError(t *testing.T) {
	t.Parallel()
	adapter := &OpencodeAdapter{}
	sink := &opencodeTestSink{}
	ctx := context.Background()

	_, err := adapter.PostStart(ctx, PostStartOptions{
		SessionID: "sess-empty-wd",
		WorkDir:   "",
		Sink:      sink,
	})
	if err == nil {
		t.Error("PostStart(emptyWorkDir) = nil error, want non-nil")
	}
}

// TestOpencodeAdapter_PostStart_ValidWorkDir_PopulatesContextLeft verifies that
// PostStart launches the SQLite tailer and that a 50%-used assistant message
// causes Sink.UpdateContextLeft(50) to be called within 1.5s.
func TestOpencodeAdapter_PostStart_ValidWorkDir_PopulatesContextLeft(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db := setupOpencodeTestDB(t, dbPath)

	rawWorkDir := t.TempDir()
	workDir, err := filepath.EvalSymlinks(rawWorkDir)
	if err != nil {
		workDir = rawWorkDir
	}

	const maxCtx = 200000
	const tokensUsed = 100000 // → pct=50

	now := time.Now()
	insertOpencodeSession(t, db, "sess-ps", workDir, now.UnixMilli())
	insertOpencodeAssistantMsg(t, db, "msg-ps-1", "sess-ps", tokensUsed, 0)

	t.Setenv("OPENCODE_DB", dbPath)
	sink := &opencodeTestSink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := &OpencodeAdapter{}
	cleanup, err := adapter.PostStart(ctx, PostStartOptions{
		SessionID:  "sess-ps",
		WorkDir:    workDir,
		StartedAt:  now.Add(-time.Second),
		MaxContext: maxCtx,
		Sink:       sink,
	})
	if err != nil {
		t.Fatalf("PostStart: %v", err)
	}
	if cleanup == nil {
		t.Fatal("PostStart returned nil cleanup func")
	}
	defer cleanup()

	if !pollForPct(sink, 50, 1500*time.Millisecond) {
		sink.mu.Lock()
		got := append([]int{}, sink.contextUpdates...)
		sink.mu.Unlock()
		t.Errorf("contextUpdates = %v after 1.5s; want entry with pct==50", got)
	}
}
