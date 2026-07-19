package spawner

import "be/internal/spawner/apirun/provider"

// apiLedgerObserver adapts apirun.LedgerObserver to the shared context-ledger
// store for API-mode agents — EXACT accounting straight from every appended
// provider.ContentBlock. One instance per spawned session.
type apiLedgerObserver struct {
	store     *ledgerStore
	sessionID string
	broadcast func() // nil-safe; production wires the spawner's WS broadcast
}

// newAPILedgerObserver wires an apiLedgerObserver against the process-global
// ledger store and the spawner's debounced WS broadcast for proc's session.
func newAPILedgerObserver(s *Spawner, proc *processInfo) *apiLedgerObserver {
	return &apiLedgerObserver{
		store:     globalLedgerStore,
		sessionID: proc.sessionID,
		broadcast: func() { s.broadcastLedgerEpoch(proc) },
	}
}

// OnMessage classifies each newly appended content block into a ledger
// entry. Called once per apirun append site — never re-observes prior
// history, so a Conversation's replayed turns are not double-counted.
func (o *apiLedgerObserver) OnMessage(role string, blocks []provider.ContentBlock) {
	if len(blocks) == 0 {
		return
	}
	l := o.store.get(o.sessionID)
	if role == "assistant" {
		l.nextTurn()
	}
	for _, b := range blocks {
		observeAPIBlock(l, b)
	}
	o.maybeBroadcast()
}

// OnUsage reconciles the ledger's current bytes/4 estimate against the
// provider-reported input-token total for the request that just returned.
func (o *apiLedgerObserver) OnUsage(u provider.Usage) {
	total := u.InputTokens + u.CacheReadTokens + u.CacheCreationTokens
	o.store.get(o.sessionID).reconcileUsage(total)
	o.maybeBroadcast()
}

// maybeBroadcast fires the debounced WS broadcast when the store's window
// has elapsed; no-op when no broadcast is wired (test construction).
func (o *apiLedgerObserver) maybeBroadcast() {
	if o.broadcast != nil && o.store.shouldBroadcast(o.sessionID) {
		o.broadcast()
	}
}

// observeAPIBlock maps one provider.ContentBlock to a ledger entry: text/
// thinking -> dialog; tool_use -> tool_use (source = path hint if the input
// carries one, else the tool name); tool_result -> image (OutputMedia
// present), file_read (paired tool is a read tool with a path), or a generic
// tool_result otherwise.
func observeAPIBlock(l *ledger, b provider.ContentBlock) {
	switch b.Type {
	case "text", "thinking":
		if b.Text == "" {
			return
		}
		l.append(LedgerKindDialog, estTokens(len(b.Text)), "", ledgerSHA([]byte(b.Text)), false)
	case "tool_use":
		path := extractPathHint(b.Input)
		l.recordToolMeta(b.ToolUseID, toolCallMeta{name: b.ToolName, path: path})
		source := b.ToolName
		if path != "" {
			l.markRef(path)
			source = path
		}
		l.append(LedgerKindToolUse, estTokens(len(b.ToolName)+len(b.Input)), source, ledgerSHA(b.Input), false)
	case "tool_result":
		meta := l.lookupToolMeta(b.ToolUseID)
		if len(b.OutputMedia) > 0 {
			var nbytes int
			for _, m := range b.OutputMedia {
				nbytes += len(m.DataB64)
			}
			l.append(LedgerKindImage, estTokens(nbytes), meta.name, "", false)
			return
		}
		if isReadToolName(meta.name) && meta.path != "" {
			l.append(LedgerKindFileRead, estTokens(len(b.Output)), meta.path, "", false)
			return
		}
		l.append(LedgerKindToolResult, estTokens(len(b.Output)), meta.name, ledgerSHA([]byte(b.Output)), false)
	}
}
