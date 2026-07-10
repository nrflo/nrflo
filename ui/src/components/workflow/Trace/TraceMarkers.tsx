import { cn } from '@/lib/utils'
import { Tooltip } from '@/components/ui/Tooltip'
import type { TraceMarker } from './types'
import type { TimeDomain } from './timeScale'
import { bucketMarkers } from './timeScale'
import { markerClasses } from './colors'

function markerTooltip(m: TraceMarker): string {
  const t = new Date(m.at)
  const time = Number.isNaN(t.getTime()) ? m.at : t.toLocaleTimeString()
  const label = m.label.length > 120 ? m.label.slice(0, 120) + '…' : m.label
  return `${m.type} · ${time} — ${label}`
}

/** Bucketed point-event dots for one lane. */
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
  const buckets = bucketMarkers(markers, domain, widthPx)
  return (
    <>
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
          <Tooltip key={`${b.pct}-${first.at}`} text={<span className="whitespace-pre-line">{tooltip}</span>}>
            <button
              data-testid="trace-marker"
              onClick={() => first.session_id && onSelect?.(first.session_id)}
              className="absolute bottom-0 -translate-x-1/2 flex items-end"
              style={{ left: `${b.pct}%` }}
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
        )
      })}
    </>
  )
}
