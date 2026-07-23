package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// fakeTranscriptTailer wraps fakeBackend with a TranscriptPath method so tests
// can control transcriptTailer independently of SupportsResume() — the two
// are deliberately decoupled after this refactor (the codex app-server
// backend has SupportsResume()==true but implements no TranscriptPath).
type fakeTranscriptTailer struct {
	fakeBackend
	path string
}

func (f fakeTranscriptTailer) TranscriptPath(proc *processInfo) string { return f.path }

// TestUpdateLedgerFromTranscript_GatedByTranscriptTailer verifies
// updateLedgerFromTranscript (and by extension tailClaudeLedgers) is gated on
// the transcriptTailer sub-interface, NOT SupportsResume() — a capability
// check, not a name-check (root CLAUDE.md rule 6). A backend without
// TranscriptPath is a no-op regardless of SupportsResume(); one that
// implements it but returns "" is also a no-op; one returning a real path
// ingests. The codex app-server backend is pinned as the regression case this
// refactor exists to prevent: SupportsResume()==true but it implements no
// transcriptTailer, so it must never be tailed for a claude-shaped transcript.
func TestUpdateLedgerFromTranscript_GatedByTranscriptTailer(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})

	t.Run("nil backend is a no-op", func(t *testing.T) {
		proc := &processInfo{sessionID: "sess-nil-backend"}
		s.updateLedgerFromTranscript(proc) // must not panic
		if _, ok := globalLedgerStore.snapshot("sess-nil-backend"); ok {
			t.Errorf("ledger created for a nil-backend proc, want none")
		}
	})

	t.Run("backend without TranscriptPath is a no-op even with SupportsResume=true", func(t *testing.T) {
		sessionID := "sess-no-tailer"
		t.Cleanup(func() { globalLedgerStore.drop(sessionID) })
		proc := &processInfo{sessionID: sessionID, backend: fakeBackend{name: "codex", supportsResume: true}}
		s.updateLedgerFromTranscript(proc)
		if _, ok := globalLedgerStore.snapshot(sessionID); ok {
			t.Errorf("ledger created for a backend with no transcriptTailer implementation")
		}
	})

	t.Run("codex app-server backend: SupportsResume=true but never tailed", func(t *testing.T) {
		sessionID := "sess-codex-appserver-no-tail"
		t.Cleanup(func() { globalLedgerStore.drop(sessionID) })
		backend := newCodexAppServerBackend(s)
		if !backend.SupportsResume() {
			t.Fatal("codexAppServerBackend.SupportsResume() = false, want true (test premise)")
		}
		if _, ok := any(backend).(transcriptTailer); ok {
			t.Fatal("codexAppServerBackend implements transcriptTailer; it must not (regression this refactor prevents)")
		}
		proc := &processInfo{sessionID: sessionID, backend: backend}
		s.updateLedgerFromTranscript(proc)
		if _, ok := globalLedgerStore.snapshot(sessionID); ok {
			t.Errorf("ledger created for the codex app-server backend despite no transcriptTailer")
		}
	})

	t.Run("TranscriptPath returning empty is a no-op", func(t *testing.T) {
		sessionID := "sess-empty-tailer-path"
		t.Cleanup(func() { globalLedgerStore.drop(sessionID) })
		proc := &processInfo{
			sessionID: sessionID,
			backend:   fakeTranscriptTailer{fakeBackend: fakeBackend{name: "claude"}, path: ""},
		}
		s.updateLedgerFromTranscript(proc)
		if _, ok := globalLedgerStore.snapshot(sessionID); ok {
			t.Errorf("ledger created despite an empty TranscriptPath")
		}
	})

	t.Run("TranscriptPath returning a real path ingests", func(t *testing.T) {
		sessionID := "sess-resume-tail"
		t.Cleanup(func() { globalLedgerStore.drop(sessionID) })
		workDir := t.TempDir()
		env := []string{"CLAUDE_CONFIG_DIR=" + t.TempDir()}
		path := claudeTranscriptPath(env, workDir, sessionID)
		writeRawTranscript(t, path, ledgerCliAssistantLine(`{"type":"text","text":"resume-capable read"}`))

		proc := &processInfo{
			sessionID: sessionID,
			workDir:   workDir,
			env:       env,
			backend:   fakeTranscriptTailer{fakeBackend: fakeBackend{name: "claude", supportsResume: true}, path: path},
		}
		s.updateLedgerFromTranscript(proc)

		snap, ok := globalLedgerStore.snapshot(sessionID)
		if !ok || len(snap.Entries) != 1 {
			t.Fatalf("snapshot=%+v ok=%v, want 1 entry", snap, ok)
		}
	})
}
