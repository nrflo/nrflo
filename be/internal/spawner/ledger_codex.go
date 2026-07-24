package spawner

import "encoding/json"

// codexLedgerEmitter returns an EventEmitter that translates normalized codex
// app-server events into EXACT ledger entries (approx=false — per-block
// token counts still start as a bytes/4 heuristic, but EventTokenUsage
// reconciles them against codex's own exact `thread/tokenUsage/updated`
// input-token count, the same basis api mode uses). Wired as the emit arg to
// dispatchAppServerEvent from the autonomous codex eventLoop; the console
// path keeps passing nil, which stays a no-op.
//
// A codex-side compaction (EventContextCompacted) discards the ledger's
// block composition — resetForCompaction supersedes every entry — because
// the following real EventTokenUsage checkpoint re-establishes exact parity
// against the post-compaction footprint; this is a one-response skew, the
// same class as the already-documented inverted-ordering skew above.
//
// Codex events carry no tool-call id, so invoke/result correlation is keyed
// by tool name: dispatchCompletedItem always emits a tool's invoke and its
// result back-to-back from the same synchronous call, so no other call for
// the same name can land in between.
//
// Timing (best-effort, NOT acceptance-gated — see spawner/sessiontiming.go):
// verified against testdata/codex_appserver/*.jsonl that app-server events
// carry no per-item timestamp field at all (only turn-level startedAt/
// completedAt epoch-second fields), so RecordSessionTimingEvent below is fed
// s.config.Clock.Now() at dispatch time rather than an embedded event
// timestamp — a coarser model-time-vs-tool-wait split gated by wall-clock
// processing latency, not the exact block-granular accounting the Claude
// transcript tailer achieves.
func (s *Spawner) codexLedgerEmitter(proc *processInfo) EventEmitter {
	l := globalLedgerStore.get(proc.sessionID)
	return func(ev EngineEvent) {
		switch ev.Type {
		case EventToolInvoke:
			path := codexPathHint(ev.ToolInput)
			l.recordToolMeta(ev.ToolName, toolCallMeta{name: ev.ToolName, path: path})
			source := ev.ToolName
			if path != "" {
				l.markRef(path)
				source = path
			}
			l.append(LedgerKindToolUse, estTokens(codexInputByteLen(ev.ToolInput)), source, "", false)
			RecordSessionTimingEvent(proc.sessionID, "", s.config.Clock.Now(), TimingBucketToolArg)
		case EventToolResult:
			meta := l.lookupToolMeta(ev.ToolName)
			if isReadToolName(ev.ToolName) && meta.path != "" {
				l.append(LedgerKindFileRead, estTokens(len(ev.Text)), meta.path, "", false)
			} else {
				l.append(LedgerKindToolResult, estTokens(len(ev.Text)), ev.ToolName, "", false)
			}
			RecordSessionTimingEvent(proc.sessionID, "", s.config.Clock.Now(), TimingBucketToolWait)
		case EventThinking:
			// Codex reuses EventThinking for both streaming reasoning deltas
			// (ItemID set) and the completed reasoning item (ItemID empty);
			// count only the completed block, else deltas double-count and
			// flood the snapshot with fragments (EventText's deltas use a
			// distinct EventTextDelta, already ignored).
			if ev.Text != "" && ev.ItemID == "" {
				l.append(LedgerKindDialog, estTokens(len(ev.Text)), "", "", false)
				RecordSessionTimingEvent(proc.sessionID, "", s.config.Clock.Now(), TimingBucketThinking)
			}
		case EventText:
			if ev.Text != "" {
				l.append(LedgerKindDialog, estTokens(len(ev.Text)), "", "", false)
				RecordSessionTimingEvent(proc.sessionID, "", s.config.Clock.Now(), TimingBucketText)
			}
		case EventTurnCompleted:
			l.nextTurn()
		case EventContextCompacted:
			l.resetForCompaction()
		case EventTokenUsage:
			if ev.Usage == nil || ev.Usage.InputTokens <= 0 {
				return
			}
			l.reconcileUsage(ev.Usage.InputTokens)
		default:
			return
		}
		s.broadcastLedgerEpoch(proc)
	}
}

// codexPathHint mirrors extractPathHint for codex's already-decoded
// map[string]any tool input (path/file_path/name, checked in that order).
func codexPathHint(input map[string]any) string {
	for _, key := range []string{"path", "file_path", "name"} {
		if v, ok := input[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// codexInputByteLen returns the JSON-encoded byte length of a tool's input
// map, for the bytes/4 token estimate.
func codexInputByteLen(input map[string]any) int {
	if len(input) == 0 {
		return 0
	}
	b, err := json.Marshal(input)
	if err != nil {
		return 0
	}
	return len(b)
}
