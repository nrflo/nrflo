import { Loader2, Gauge } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useSessionContextLedger } from '@/hooks/useSessionContextLedger'
import type { LedgerKind } from '@/types/contextLedger'

const KIND_ORDER: LedgerKind[] = ['dialog', 'tool_use', 'tool_result', 'file_read', 'image', 'injected']

const KIND_LABEL: Record<LedgerKind, string> = {
  dialog: 'Dialog',
  tool_use: 'Tool use',
  tool_result: 'Tool result',
  file_read: 'File read',
  image: 'Image',
  injected: 'Injected',
}

const KIND_COLOR: Record<LedgerKind, string> = {
  dialog: 'bg-blue-500',
  tool_use: 'bg-purple-500',
  tool_result: 'bg-teal-500',
  file_read: 'bg-amber-500',
  image: 'bg-pink-500',
  injected: 'bg-gray-500',
}

interface ContextLedgerPanelProps {
  sessionId: string | undefined
  enabled: boolean
  // Positive token budget to render the budget bar against. Neither the
  // snapshot nor the WS payload carries a budget today (scope guard: no BE
  // payload changes) — this is FE-only, for a future BE field / tests.
  budgetTokens?: number
}

export function ContextLedgerPanel({ sessionId, enabled, budgetTokens }: ContextLedgerPanelProps) {
  const { data: snapshot, isLoading, liveTotals } = useSessionContextLedger(sessionId, enabled)

  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
        <Loader2 className="h-6 w-6 mb-2 spin-sync opacity-50" />
        <p className="text-xs">Loading context ledger...</p>
      </div>
    )
  }

  if (!snapshot) {
    return (
      <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
        <Gauge className="h-8 w-8 mb-2 opacity-30" />
        <p className="text-xs">No context ledger available</p>
      </div>
    )
  }

  const totalsByKind = liveTotals?.totals_by_kind ?? snapshot.totals_by_kind
  const totalTokens = liveTotals?.total_tokens ?? Object.values(totalsByKind).reduce((sum, v) => sum + (v ?? 0), 0)
  const entryCount = liveTotals?.entry_count ?? snapshot.entries.length
  const supersededEntries = snapshot.entries.filter((e) => e.superseded)

  const overBudget = budgetTokens != null && budgetTokens > 0 && totalTokens > budgetTokens

  return (
    <div className="space-y-4">
      <div>
        <p className="text-xs text-muted-foreground mb-1">Per-kind breakdown</p>
        <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-muted">
          {KIND_ORDER.map((kind) => {
            const tokens = totalsByKind[kind] ?? 0
            if (tokens <= 0 || totalTokens <= 0) return null
            return (
              <div
                key={kind}
                className={KIND_COLOR[kind]}
                style={{ width: `${(tokens / totalTokens) * 100}%` }}
                title={`${KIND_LABEL[kind]}: ${tokens} tokens`}
              />
            )
          })}
        </div>
        <div className="mt-2 space-y-1">
          {KIND_ORDER.map((kind) => {
            const tokens = totalsByKind[kind]
            if (!tokens) return null
            return (
              <div key={kind} className="flex items-center gap-2 text-xs">
                <span className={cn('h-2 w-2 rounded-full shrink-0', KIND_COLOR[kind])} />
                <span className="text-muted-foreground flex-1">{KIND_LABEL[kind]}</span>
                <span className="font-mono">{tokens} tok</span>
              </div>
            )
          })}
        </div>
      </div>

      {budgetTokens != null && budgetTokens > 0 && (
        <div>
          <div className="flex items-center justify-between text-xs mb-1">
            <span className="text-muted-foreground">Budget</span>
            <span className={cn('font-mono', overBudget && 'text-destructive')}>
              {totalTokens} / {budgetTokens} tok
            </span>
          </div>
          <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
            <div
              className={cn('h-full rounded-full', overBudget ? 'bg-destructive' : 'bg-primary')}
              style={{ width: `${Math.min(100, (totalTokens / budgetTokens) * 100)}%` }}
            />
          </div>
        </div>
      )}

      <div className="flex items-center gap-3 text-xs text-muted-foreground">
        <span>{totalTokens} total tokens</span>
        <span>·</span>
        <span>{entryCount} entries</span>
      </div>

      {supersededEntries.length > 0 && (
        <div>
          <p className="text-xs text-muted-foreground mb-1">
            Superseded — excluded from totals ({supersededEntries.length})
          </p>
          <div className="space-y-1">
            {supersededEntries.map((entry) => (
              <div
                key={entry.id}
                className="flex items-center gap-2 rounded border border-border bg-muted/20 px-2 py-1 text-xs text-muted-foreground opacity-60"
              >
                <span className="line-through flex-1 truncate">{KIND_LABEL[entry.kind]} · {entry.source}</span>
                <span className="font-mono shrink-0">{entry.tokens_est} tok</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
