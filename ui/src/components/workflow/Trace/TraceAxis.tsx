import { cn } from '@/lib/utils'
import type { WorkflowTraceResponse } from './types'
import type { TimeDomain } from './timeScale'
import { niceTicks, toPct, parseTs } from './timeScale'
import { statusColor } from '@/lib/utils'

function tickLabel(ms: number, spanMs: number): string {
  const d = new Date(ms)
  if (spanMs >= 86_400_000) {
    return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  }
  return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

/** Time ruler + workflow root span bar. */
export function TraceAxis({
  trace,
  domain,
  tickTarget = 7,
}: {
  trace: WorkflowTraceResponse
  domain: TimeDomain
  tickTarget?: number
}) {
  const ticks = niceTicks(domain, tickTarget)
  const startPct = toPct(parseTs(trace.started_at), domain) ?? 0
  const endPct = toPct(parseTs(trace.ended_at), domain) ?? 100
  const running = trace.status === 'active' || trace.status === 'waiting'

  return (
    <div className="grid grid-cols-[10rem_1fr] border-b border-border">
      <div className="px-2 py-1 text-xs font-medium text-muted-foreground sticky left-0 bg-background">
        <span className={cn('px-1.5 py-0.5 rounded text-[10px] font-medium mr-1', statusColor(trace.status))}>
          {trace.status}
        </span>
        {trace.workflow}
      </div>
      <div className="relative h-12">
        {ticks.map((t) => {
          const pct = toPct(t, domain)
          if (pct == null) return null
          return (
            <div key={t} className="absolute top-0 bottom-0" style={{ left: `${pct}%` }}>
              <div className="h-full border-l border-border/60" />
              <span className="absolute top-0.5 left-1 text-[10px] text-muted-foreground whitespace-nowrap">
                {tickLabel(t, domain.max - domain.min)}
              </span>
            </div>
          )
        })}
        <div
          data-testid="trace-root-span"
          className={cn(
            'absolute bottom-1 h-3 rounded border',
            running
              ? 'border-amber-400 dark:border-amber-500 bg-amber-50 dark:bg-amber-950/30 animate-pulse'
              : trace.status === 'failed'
                ? 'border-red-500 bg-red-50 dark:bg-red-950/30'
                : 'border-green-500 bg-green-50 dark:bg-green-950/30'
          )}
          style={{ left: `${startPct}%`, width: `${Math.max(endPct - startPct, 0.5)}%` }}
        />
      </div>
    </div>
  )
}
