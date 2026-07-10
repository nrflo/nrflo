import { cn, statusColor } from '@/lib/utils'
import type { TraceChild } from './types'
import type { TimeDomain } from './timeScale'
import { toPct, parseTs } from './timeScale'

/** A sub-workflow run launched by this instance; click opens its own trace. */
export function TraceChildRow({
  child,
  domain,
  onOpen,
}: {
  child: TraceChild
  domain: TimeDomain
  onOpen: (instanceId: string) => void
}) {
  const startPct = toPct(parseTs(child.started_at), domain) ?? 0
  const endPct = toPct(parseTs(child.ended_at), domain) ?? 100
  return (
    <div data-testid="trace-child" className="grid grid-cols-[10rem_1fr] border-b border-border/40 last:border-b-0">
      <div className="px-2 py-1 sticky left-0 bg-background min-w-0">
        <button
          className="text-xs font-medium truncate block max-w-full hover:text-primary text-left"
          onClick={() => onOpen(child.instance_id)}
        >
          ↳ {child.workflow}
        </button>
        <span className={cn('px-1.5 py-0.5 rounded text-[10px] font-medium', statusColor(child.status))}>
          {child.status}
        </span>
      </div>
      <div className="relative h-9">
        <button
          onClick={() => onOpen(child.instance_id)}
          className={cn(
            'absolute top-2 h-4 rounded border border-dashed',
            child.status === 'active'
              ? 'border-amber-400 bg-amber-50 dark:bg-amber-950/30 animate-pulse'
              : child.status === 'failed'
                ? 'border-red-500 bg-red-50 dark:bg-red-950/30'
                : 'border-green-500 bg-green-50 dark:bg-green-950/30'
          )}
          style={{ left: `${startPct}%`, width: `${Math.max(endPct - startPct, 0.5)}%` }}
          aria-label={`open trace of ${child.workflow}`}
        />
      </div>
    </div>
  )
}
