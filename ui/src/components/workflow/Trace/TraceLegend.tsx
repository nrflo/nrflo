import { cn } from '@/lib/utils'
import { MARKER_TYPES, markerClasses } from './colors'

/** Marker-type filter chips + truncation notice. */
export function TraceLegend({
  active,
  onToggle,
  truncated,
}: {
  active: Set<string>
  onToggle: (type: string) => void
  truncated?: boolean
}) {
  return (
    <div className="flex items-center gap-1 flex-wrap">
      {MARKER_TYPES.map((type) => (
        <button
          key={type}
          data-testid={`trace-chip-${type}`}
          onClick={() => onToggle(type)}
          className={cn(
            'flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] border transition-colors',
            active.has(type)
              ? 'border-border text-foreground bg-muted/50'
              : 'border-transparent text-muted-foreground opacity-50 hover:opacity-80'
          )}
        >
          <span className={cn('w-2 h-2 rounded-full', markerClasses(type))} />
          {type.replace('_', ' ')}
        </button>
      ))}
      {truncated && (
        <span className="text-[11px] text-amber-600 dark:text-amber-400 ml-2" data-testid="trace-truncated">
          marker limit reached — earliest events shown
        </span>
      )}
    </div>
  )
}
