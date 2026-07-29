import { cn } from '@/lib/utils'
import { Tooltip } from '@/components/ui/Tooltip'
import type { TraceLaneData, TraceMarker } from './types'
import type { TimeDomain } from './timeScale'
import { toPct, parseTs } from './timeScale'
import { segmentClasses } from './colors'
import { TraceMarkers } from './TraceMarkers'
import { TimeBreakdownBar } from './TimeBreakdownBar'

function segmentTooltip(status: string, result: string | undefined, start?: string | null, end?: string | null): string {
  const fmt = (ts?: string | null) => {
    const t = parseTs(ts)
    return t == null ? '…' : new Date(t).toLocaleTimeString()
  }
  return `${status}${result ? ` (${result})` : ''} · ${fmt(start)} → ${fmt(end)}`
}

/** One agent lane: relaunch-chain segments + bucketed markers. */
export function TraceLane({
  lane,
  markers,
  domain,
  widthPx,
  onSelect,
  indent,
}: {
  lane: TraceLaneData
  markers: TraceMarker[]
  domain: TimeDomain
  widthPx: number
  onSelect?: (sessionId: string) => void
  /** Nested worker row: shifts the sticky label column and dims the row. */
  indent?: boolean
}) {
  return (
    <div
      data-testid="trace-lane"
      className={cn(
        'grid grid-cols-[10rem_1fr] border-b border-border/40 last:border-b-0',
        indent && 'opacity-80'
      )}
    >
      <div className={cn('px-2 py-1 sticky left-0 bg-background min-w-0', indent && 'pl-5')}>
        <button
          className="text-xs font-medium truncate block max-w-full hover:text-primary text-left"
          onClick={() => {
            const last = lane.segments?.[lane.segments.length - 1]
            if (last) onSelect?.(last.session_id)
          }}
          title={lane.phase}
        >
          {lane.agent_type.replace(/_/g, ' ')}
        </button>
        <div className="text-[10px] text-muted-foreground truncate">
          {lane.model_id ?? ''}
          {(lane.restarts?.length ?? 0) > 0 && (
            <span className="ml-1" data-testid="trace-lane-restarts" title={`${lane.restarts!.length} restart(s)`}>
              ↻{lane.restarts!.length}
            </span>
          )}
          {(lane.nudge_count ?? 0) > 0 && (
            <span className="ml-1" data-testid="trace-lane-nudges" title={`nudged ${lane.nudge_count} time(s)`}>
              nudged×{lane.nudge_count}
            </span>
          )}
          {(lane.stop_block_count ?? 0) > 0 && (
            <span className="ml-1" data-testid="trace-lane-stopblocks" title={`Stop hook blocked completion ${lane.stop_block_count} time(s)`}>
              blocked×{lane.stop_block_count}
            </span>
          )}
        </div>
        <TimeBreakdownBar buckets={lane.time_buckets} />
      </div>
      <div className="relative h-9">
        {(lane.segments ?? []).map((seg) => {
          const startPct = toPct(parseTs(seg.started_at), domain)
          if (startPct == null) return null
          const endPct = toPct(parseTs(seg.ended_at), domain) ?? 100
          // Tooltip lives inside the positioned wrapper so it anchors at the
          // segment, not at the row start (the trigger span is zero-size).
          return (
            <div
              key={seg.session_id}
              data-testid="trace-segment"
              className="absolute top-1 h-4"
              style={{ left: `${startPct}%`, width: `${Math.max(endPct - startPct, 0.5)}%` }}
            >
              <Tooltip text={segmentTooltip(seg.status, seg.result, seg.started_at, seg.ended_at)}>
                <button
                  onClick={() => onSelect?.(seg.session_id)}
                  className={cn('absolute inset-0 rounded border', segmentClasses(seg.status, seg.result))}
                  aria-label={`${lane.agent_type} ${seg.status}`}
                />
              </Tooltip>
            </div>
          )
        })}
        <TraceMarkers markers={markers} domain={domain} widthPx={widthPx} onSelect={onSelect} />
      </div>
    </div>
  )
}
