import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import type { SubLaneGroup } from './subLanes'
import type { TraceMarker } from './types'
import type { TimeDomain } from './timeScale'
import { TraceLane } from './TraceLane'

/** Collapsed-by-default disclosure row for one delegation/consult fanout. */
export function TraceSubLaneGroup({
  group,
  domain,
  widthPx,
  activeTypes,
  onSelect,
}: {
  group: SubLaneGroup
  domain: TimeDomain
  widthPx: number
  activeTypes: Set<string>
  onSelect?: (phase: string, sessionId: string) => void
}) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div data-testid="trace-sublane-group">
      <div className="grid grid-cols-[10rem_1fr] border-b border-border/40">
        <div className="px-2 py-1 sticky left-0 bg-background min-w-0">
          <Button
            variant="ghost"
            size="sm"
            className="h-5 px-1 text-[10px] font-medium gap-1 justify-start w-full"
            onClick={() => setExpanded((v) => !v)}
            aria-expanded={expanded}
          >
            {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            <span className="truncate">{group.label}</span>
          </Button>
        </div>
        <div />
      </div>
      {expanded &&
        group.lanes.map((lane) => (
          <div key={lane.lane_id} data-testid="trace-sublane">
            <TraceLane
              lane={lane}
              markers={(lane.markers ?? []).filter((m: TraceMarker) => activeTypes.has(m.type))}
              domain={domain}
              widthPx={widthPx}
              onSelect={(sid) => onSelect?.(lane.phase, sid)}
              indent
            />
          </div>
        ))}
    </div>
  )
}
