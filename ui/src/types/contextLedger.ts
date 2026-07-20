// Mirrors be/internal/spawner/ledger_types.go (snake_case JSON).

export type LedgerKind = 'dialog' | 'tool_use' | 'tool_result' | 'file_read' | 'image' | 'injected'

// One ordered block in a session's context ledger. Superseded marks an entry
// a later dedup-matching entry has replaced — excluded from epoch totals but
// kept in the snapshot for inspection; it is the FE's eviction indicator
// (the context-watcher evicts message history, not ledger entries, so there
// is no separate per-entry 'evicted' flag).
export interface LedgerEntry {
  id: string
  kind: LedgerKind
  tokens_est: number
  born_turn: number
  last_ref_turn: number
  source: string
  sha?: string
  superseded: boolean
  approx: boolean
}

// GET /api/v1/sessions/{id}/context-ledger response.
export interface ContextLedgerSnapshot {
  session_id: string
  entries: LedgerEntry[]
  totals_by_kind: Partial<Record<LedgerKind, number>>
}

// agent.context_ledger WS payload (ledgerBroadcastData, ledger_store.go) —
// debounced totals only, no individual entries.
export interface LedgerLiveTotals {
  session_id: string
  total_tokens: number
  entry_count: number
  totals_by_kind: Partial<Record<LedgerKind, number>>
}
