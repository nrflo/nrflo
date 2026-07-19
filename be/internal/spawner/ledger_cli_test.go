package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// ledgerCliTranscriptLine builds one assistant JSONL transcript line with the
// given content blocks (already-JSON-encoded fragments).
func ledgerCliAssistantLine(blocksJSON string) string {
	return `{"type":"assistant","message":{"role":"assistant","content":[` + blocksJSON + `]}}` + "\n"
}

func ledgerCliUserLine(blocksJSON string) string {
	return `{"type":"user","message":{"role":"user","content":[` + blocksJSON + `]}}` + "\n"
}

// TestIngestClaudeTranscriptLine_AssistantAndUserBlocks drives the
// package-level line parsers directly against a local *ledger (no global
// store, no file I/O): assistant text/thinking/tool_use(Read) followed by a
// user tool_result, asserting the resulting entries' kind/source/tokens.
func TestIngestClaudeTranscriptLine_AssistantAndUserBlocks(t *testing.T) {
	l := newLedger()

	assistantLine := ledgerCliAssistantLine(
		`{"type":"text","text":"looking at the file"},` +
			`{"type":"thinking","thinking":"hmm let me check"},` +
			`{"type":"tool_use","id":"tu1","name":"Read","input":{"file_path":"/repo/a.txt"}}`,
	)
	ingestClaudeTranscriptLine(l, []byte(assistantLine))

	userLine := ledgerCliUserLine(`{"type":"tool_result","tool_use_id":"tu1","content":"file A contents"}`)
	ingestClaudeTranscriptLine(l, []byte(userLine))

	snap := l.snapshot("s1")
	wantKinds := []LedgerKind{LedgerKindDialog, LedgerKindDialog, LedgerKindToolUse, LedgerKindFileRead}
	if len(snap.Entries) != len(wantKinds) {
		t.Fatalf("Entries = %d, want %d; entries=%+v", len(snap.Entries), len(wantKinds), snap.Entries)
	}
	for i, k := range wantKinds {
		if snap.Entries[i].Kind != k {
			t.Errorf("Entries[%d].Kind = %q, want %q", i, snap.Entries[i].Kind, k)
		}
	}

	fileRead := snap.Entries[3]
	if fileRead.Source != "/repo/a.txt" {
		t.Errorf("file_read Source = %q, want /repo/a.txt", fileRead.Source)
	}
	if !fileRead.Approx {
		t.Errorf("cli-ingested entries must be marked Approx=true (EXACT-ish tail parse)")
	}
	wantTokens := estTokens(len("file A contents"))
	if fileRead.TokensEst != wantTokens {
		t.Errorf("file_read TokensEst = %d, want %d (bytes/4 of output)", fileRead.TokensEst, wantTokens)
	}

	toolUse := snap.Entries[2]
	if toolUse.Source != "/repo/a.txt" {
		t.Errorf("tool_use Source = %q, want /repo/a.txt (path hint)", toolUse.Source)
	}
}

// TestIngestClaudeTranscriptLine_DuplicateFileReadSupersedes verifies two
// tool_result reads of the same file_path across separate transcript lines
// mark the earlier file_read entry Superseded.
func TestIngestClaudeTranscriptLine_DuplicateFileReadSupersedes(t *testing.T) {
	l := newLedger()

	ingestClaudeTranscriptLine(l, []byte(ledgerCliAssistantLine(
		`{"type":"tool_use","id":"tu1","name":"Read","input":{"file_path":"/repo/a.txt"}}`)))
	ingestClaudeTranscriptLine(l, []byte(ledgerCliUserLine(
		`{"type":"tool_result","tool_use_id":"tu1","content":"first read"}`)))

	ingestClaudeTranscriptLine(l, []byte(ledgerCliAssistantLine(
		`{"type":"tool_use","id":"tu2","name":"Read","input":{"file_path":"/repo/a.txt"}}`)))
	ingestClaudeTranscriptLine(l, []byte(ledgerCliUserLine(
		`{"type":"tool_result","tool_use_id":"tu2","content":"second read, file changed"}`)))

	snap := l.snapshot("s1")
	var fileReads []LedgerEntry
	for _, e := range snap.Entries {
		if e.Kind == LedgerKindFileRead {
			fileReads = append(fileReads, e)
		}
	}
	if len(fileReads) != 2 {
		t.Fatalf("file_read entries = %d, want 2", len(fileReads))
	}
	if !fileReads[0].Superseded {
		t.Errorf("first file_read Superseded = false, want true (duplicate path)")
	}
	if fileReads[1].Superseded {
		t.Errorf("second file_read Superseded = true, want false")
	}
}

// TestIngestClaudeTranscriptLine_NonToolResultUserBlockIgnored verifies a
// user block that isn't a tool_result (e.g. a plain text reply) produces no
// ledger entry.
func TestIngestClaudeTranscriptLine_NonToolResultUserBlockIgnored(t *testing.T) {
	l := newLedger()
	ingestClaudeTranscriptLine(l, []byte(ledgerCliUserLine(`{"type":"text","text":"not a tool result"}`)))
	if got := len(l.snapshot("s1").Entries); got != 0 {
		t.Errorf("Entries = %d, want 0", got)
	}
}

// TestIngestClaudeTranscriptLine_MalformedJSONIgnored verifies a corrupt
// line is dropped without panicking (transcript lines can be partially
// written mid-flush by the CLI in principle).
func TestIngestClaudeTranscriptLine_MalformedJSONIgnored(t *testing.T) {
	l := newLedger()
	ingestClaudeTranscriptLine(l, []byte(`{not valid json`))
	if got := len(l.snapshot("s1").Entries); got != 0 {
		t.Errorf("Entries = %d, want 0 (malformed line ignored)", got)
	}
}

// TestSpawnerIngestClaudeTranscript_IncrementalTailing exercises the full
// byte-offset tailer via (*Spawner).ingestClaudeTranscript against a real
// file, driven twice like consecutive monitorAll ticks: the second call must
// pick up only the newly appended line, leaving the first ingestion's entry
// untouched (offset advances, no re-append of already-consumed lines).
func TestSpawnerIngestClaudeTranscript_IncrementalTailing(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})
	sessionID := "sess-tail-incremental"
	t.Cleanup(func() { globalLedgerStore.drop(sessionID) })

	dir := t.TempDir()
	path := dir + "/transcript.jsonl"
	line1 := ledgerCliAssistantLine(`{"type":"text","text":"first turn text"}`)
	writeRawTranscript(t, path, line1)

	s.ingestClaudeTranscript(sessionID, path)
	snap, ok := globalLedgerStore.snapshot(sessionID)
	if !ok || len(snap.Entries) != 1 {
		t.Fatalf("after first ingest: snapshot=%+v ok=%v, want 1 entry", snap, ok)
	}
	firstID := snap.Entries[0].ID

	line2 := ledgerCliAssistantLine(`{"type":"text","text":"second turn text"}`)
	writeRawTranscript(t, path, line1+line2)

	s.ingestClaudeTranscript(sessionID, path)
	snap, ok = globalLedgerStore.snapshot(sessionID)
	if !ok || len(snap.Entries) != 2 {
		t.Fatalf("after second ingest: snapshot=%+v ok=%v, want 2 entries", snap, ok)
	}
	if snap.Entries[0].ID != firstID {
		t.Errorf("first entry ID changed across ingests: %q -> %q (re-appended, not tailed)", firstID, snap.Entries[0].ID)
	}

	// A third call with no file growth must not duplicate anything.
	s.ingestClaudeTranscript(sessionID, path)
	if snap, _ := globalLedgerStore.snapshot(sessionID); len(snap.Entries) != 2 {
		t.Errorf("re-ingesting an unchanged file added entries: now %d, want 2", len(snap.Entries))
	}
}

// TestSpawnerIngestClaudeTranscript_PartialTrailingLineNotConsumed verifies a
// trailing line with no newline yet (CLI still writing it) is left
// unconsumed until it completes.
func TestSpawnerIngestClaudeTranscript_PartialTrailingLineNotConsumed(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})
	sessionID := "sess-tail-partial"
	t.Cleanup(func() { globalLedgerStore.drop(sessionID) })

	dir := t.TempDir()
	path := dir + "/transcript.jsonl"
	complete := ledgerCliAssistantLine(`{"type":"text","text":"complete line"}`)
	partial := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"cut off`
	writeRawTranscript(t, path, complete+partial)

	s.ingestClaudeTranscript(sessionID, path)
	snap, ok := globalLedgerStore.snapshot(sessionID)
	if !ok || len(snap.Entries) != 1 {
		t.Fatalf("snapshot=%+v ok=%v, want exactly 1 entry (partial line withheld)", snap, ok)
	}

	// Completing the line on the next tick must pick it up.
	writeRawTranscript(t, path, complete+partial+`"}]}}`+"\n")
	s.ingestClaudeTranscript(sessionID, path)
	if snap, _ := globalLedgerStore.snapshot(sessionID); len(snap.Entries) != 2 {
		t.Errorf("after completing the trailing line: entries = %d, want 2", len(snap.Entries))
	}
}

// TestSpawnerIngestClaudeTranscript_EmptyPathAndMissingFileAreNoOps verifies
// the tailer never panics on an empty path or a nonexistent file: an empty
// path bails before touching the store at all, while a nonexistent file
// still lazily creates an (empty) ledger for the session, since get() runs
// before the file open is attempted.
func TestSpawnerIngestClaudeTranscript_EmptyPathAndMissingFileAreNoOps(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})

	s.ingestClaudeTranscript("sess-empty-path", "")
	if _, ok := globalLedgerStore.snapshot("sess-empty-path"); ok {
		t.Errorf("snapshot ok = true after an empty-path call, want false")
	}

	sessionID := "sess-missing-file"
	t.Cleanup(func() { globalLedgerStore.drop(sessionID) })
	s.ingestClaudeTranscript(sessionID, "/nonexistent/path/does/not/exist.jsonl")
	snap, ok := globalLedgerStore.snapshot(sessionID)
	if !ok || len(snap.Entries) != 0 {
		t.Errorf("snapshot = %+v, ok=%v; want ok=true with 0 entries (lazy empty ledger, file open failed)", snap, ok)
	}
}

// TestUpdateLedgerFromTranscript_GatedBySupportsResume verifies
// updateLedgerFromTranscript (and by extension tailClaudeLedgers) is a no-op
// for backends whose SupportsResume() is false — a capability check, not a
// name-check (root CLAUDE.md rule 6) — and proceeds for a Claude-PTY-like
// backend that supports resume.
func TestUpdateLedgerFromTranscript_GatedBySupportsResume(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Now())})

	t.Run("nil backend is a no-op", func(t *testing.T) {
		proc := &processInfo{sessionID: "sess-nil-backend"}
		s.updateLedgerFromTranscript(proc) // must not panic
		if _, ok := globalLedgerStore.snapshot("sess-nil-backend"); ok {
			t.Errorf("ledger created for a nil-backend proc, want none")
		}
	})

	t.Run("SupportsResume=false is a no-op", func(t *testing.T) {
		sessionID := "sess-no-resume"
		t.Cleanup(func() { globalLedgerStore.drop(sessionID) })
		proc := &processInfo{sessionID: sessionID, backend: fakeBackend{name: "codex", supportsResume: false}}
		s.updateLedgerFromTranscript(proc)
		if _, ok := globalLedgerStore.snapshot(sessionID); ok {
			t.Errorf("ledger created despite SupportsResume()=false")
		}
	})

	t.Run("SupportsResume=true ingests the transcript", func(t *testing.T) {
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
			backend:   fakeBackend{name: "claude", supportsResume: true},
		}
		s.updateLedgerFromTranscript(proc)

		snap, ok := globalLedgerStore.snapshot(sessionID)
		if !ok || len(snap.Entries) != 1 {
			t.Fatalf("snapshot=%+v ok=%v, want 1 entry", snap, ok)
		}
	})
}
