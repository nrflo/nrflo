package spawner

import "encoding/json"

// codexLedgerEmitter returns an EventEmitter that translates normalized codex
// app-server events into APPROX ledger entries (approx=true — token counts
// come from event text/JSON byte length, not codex's own tokenizer). Wired
// as the emit arg to dispatchAppServerEvent from the autonomous codex
// eventLoop; the console path keeps passing nil, which stays a no-op.
//
// Codex events carry no tool-call id, so invoke/result correlation is keyed
// by tool name: dispatchCompletedItem always emits a tool's invoke and its
// result back-to-back from the same synchronous call, so no other call for
// the same name can land in between.
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
			l.append(LedgerKindToolUse, estTokens(codexInputByteLen(ev.ToolInput)), source, "", true)
		case EventToolResult:
			meta := l.lookupToolMeta(ev.ToolName)
			if isReadToolName(ev.ToolName) && meta.path != "" {
				l.append(LedgerKindFileRead, estTokens(len(ev.Text)), meta.path, "", true)
			} else {
				l.append(LedgerKindToolResult, estTokens(len(ev.Text)), ev.ToolName, "", true)
			}
		case EventThinking:
			// Codex reuses EventThinking for both streaming reasoning deltas
			// (ItemID set) and the completed reasoning item (ItemID empty);
			// count only the completed block, else deltas double-count and
			// flood the snapshot with fragments (EventText's deltas use a
			// distinct EventTextDelta, already ignored).
			if ev.Text != "" && ev.ItemID == "" {
				l.append(LedgerKindDialog, estTokens(len(ev.Text)), "", "", true)
			}
		case EventText:
			if ev.Text != "" {
				l.append(LedgerKindDialog, estTokens(len(ev.Text)), "", "", true)
			}
		case EventTurnCompleted:
			l.nextTurn()
		case EventTokenUsage:
			if ev.ContextLeftPct <= 0 || proc.maxContext <= 0 {
				return
			}
			used := proc.maxContext - (proc.maxContext*ev.ContextLeftPct)/100
			l.reconcileUsage(used)
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
