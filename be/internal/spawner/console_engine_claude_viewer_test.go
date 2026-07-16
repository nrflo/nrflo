package spawner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// grabTestPtySession returns the mock PTY session the engine started on.
func grabTestPtySession(t *testing.T, mgr *mockPtyManager, sessionID string) *mockPtySession {
	t.Helper()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	sess, ok := mgr.sessions[sessionID]
	if !ok {
		t.Fatalf("no mock pty session %q", sessionID)
	}
	return sess
}

type viewerCollector struct {
	mu  sync.Mutex
	buf strings.Builder
	got chan struct{}
}

func newViewerCollector() *viewerCollector {
	return &viewerCollector{got: make(chan struct{}, 16)}
}

func (v *viewerCollector) sink(data []byte) {
	v.mu.Lock()
	v.buf.Write(data)
	v.mu.Unlock()
	select {
	case v.got <- struct{}{}:
	default:
	}
}

func (v *viewerCollector) wait(t *testing.T) {
	t.Helper()
	select {
	case <-v.got:
	case <-time.After(2 * time.Second):
		t.Fatal("viewer never received PTY output")
	}
}

func (v *viewerCollector) text() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.buf.String()
}

// TestClaudeEngine_Viewer_ForwardsOutputAndDetaches: an attached viewer
// receives ferry output; after detach it receives nothing more; ViewerWrite
// reaches the PTY.
func TestClaudeEngine_Viewer_ForwardsOutputAndDetaches(t *testing.T) {
	e, mgr := startTestClaudeEngine(t, &testSink{}, nil, EngineSpec{SessionID: "sess-viewer-1"})
	t.Cleanup(e.Stop)
	sess := grabTestPtySession(t, mgr, "sess-viewer-1")

	v := newViewerCollector()
	detach := e.AttachViewer(v.sink)

	sess.feed("live-output")
	v.wait(t)
	if !strings.Contains(v.text(), "live-output") {
		t.Fatalf("viewer text = %q, want live-output", v.text())
	}

	detach()
	sess.feed("after-detach")
	time.Sleep(20 * time.Millisecond) // give the ferry a beat to (not) deliver
	if strings.Contains(v.text(), "after-detach") {
		t.Errorf("viewer received output after detach: %q", v.text())
	}

	if err := e.ViewerWrite([]byte("typed!")); err != nil {
		t.Fatalf("ViewerWrite: %v", err)
	}
	sess.mu.Lock()
	written := string(sess.writtenBytes)
	sess.mu.Unlock()
	if !strings.Contains(written, "typed!") {
		t.Errorf("pty written = %q, want typed!", written)
	}
}

// TestClaudeEngine_NotifyUserPrompt_EchoMatchedOnce: the engine claims only
// the exact SendUserTurn echo, exactly once — a human retyping the same text
// later (attached terminal) must persist normally.
func TestClaudeEngine_NotifyUserPrompt_EchoMatchedOnce(t *testing.T) {
	e, _ := startTestClaudeEngine(t, &testSink{}, nil, EngineSpec{SessionID: "sess-echo-1"})
	t.Cleanup(e.Stop)

	if err := e.SendUserTurn(context.Background(), "hello echo"); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}

	if own := e.NotifyUserPrompt("something else"); own {
		t.Error("unrelated prompt claimed as own echo")
	}
	if own := e.NotifyUserPrompt("hello echo"); !own {
		t.Error("SendUserTurn echo not claimed")
	}
	if own := e.NotifyUserPrompt("hello echo"); own {
		t.Error("echo claimed twice — a human retyping the same text would be suppressed")
	}
}
