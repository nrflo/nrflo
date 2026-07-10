import { cn } from '@/lib/utils'
import { Tooltip } from '@/components/ui/Tooltip'
import type { TraceMarker } from './types'
import type { TimeDomain } from './timeScale'
import { bucketMarkers, splitSpans, parseTs } from './timeScale'
import { markerClasses } from './colors'

function markerTooltip(m: TraceMarker): string {
  const t = new Date(m.at)
  const time = Number.isNaN(t.getTime()) ? m.at : t.toLocaleTimeString()
  const label = m.label.length > 120 ? m.label.slice(0, 120) + '…' : m.label
  const start = parseTs(m.at)
  const end = parseTs(m.ended_at)
  const duration = start != null && end != null ? ` (${((end - start) / 1000).toFixed(1)}s)` : ''
  return `${m.type} · ${time}${duration} — ${label}`
}

/** Point-event dots (pixel-bucketed) + closed tool spans as duration bars. */
export function TraceMarkers({
  markers,
  domain,
  widthPx,
  onSelect,
}: {
  markers: TraceMarker[]
  domain: TimeDomain
  widthPx: number
  onSelect?: (sessionId: string) => void
}) {
  const { spans, points } = splitSpans(markers, domain, widthPx)
  const buckets = bucketMarkers(points, domain, widthPx)
  return (
    <>
      {/* Tooltips must sit inside the positioned wrapper — the Tooltip trigger
          span is zero-size, so wrapping an absolute child would anchor the
          tooltip at the row start instead of at the event. */}
      {spans.map((s) => (
        <div
          key={`span-${s.startPct}-${s.marker.at}`}
          data-testid="trace-span"
          className="absolute bottom-0.5 h-1.5"
          style={{ left: `${s.startPct}%`, width: `${s.endPct - s.startPct}%` }}
        >
          <Tooltip text={markerTooltip(s.marker)}>
            <button
              onClick={() => s.marker.session_id && onSelect?.(s.marker.session_id)}
              className={cn('absolute inset-0 rounded-sm opacity-80', markerClasses(s.marker.type))}
              aria-label={markerTooltip(s.marker)}
            />
          </Tooltip>
        </div>
      ))}
      {buckets.map((b) => {
        const first = b.markers[0]
        const cluster = b.markers.length > 1
        const tooltip = cluster
          ? [
              `${b.markers.length} events`,
              ...b.markers.slice(0, 5).map(markerTooltip),
              ...(b.markers.length > 5 ? [`+${b.markers.length - 5} more`] : []),
            ].join('\n')
          : markerTooltip(first)
        return (
          <div
            key={`${b.pct}-${first.at}`}
            className="absolute bottom-0 -translate-x-1/2"
            style={{ left: `${b.pct}%` }}
          >
            <Tooltip text={<span className="whitespace-pre-line">{tooltip}</span>}>
              <button
                data-testid="trace-marker"
                onClick={() => first.session_id && onSelect?.(first.session_id)}
                className="flex items-end"
                aria-label={tooltip}
              >
                <span
                  className={cn(
                    'rounded-full block',
                    cluster ? 'w-2.5 h-2.5' : 'w-1.5 h-1.5',
                    markerClasses(b.hasError ? 'error' : b.dominantType)
                  )}
                />
                {cluster && (
                  <span
                    data-testid="trace-marker-count"
                    className="text-[9px] leading-none text-muted-foreground ml-0.5"
                  >
                    {b.markers.length}
                  </span>
                )}
              </button>
            </Tooltip>
          </div>
        )
      })}
    </>
  )
}
