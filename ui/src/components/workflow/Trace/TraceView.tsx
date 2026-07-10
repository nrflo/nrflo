import { useEffect, useMemo, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTrace, traceKeys } from '@/hooks/useTrace'
import { useWebSocketEvent } from '@/hooks/useWebSocketSubscription'
import type { AgentSession, WorkflowState } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'
import { buildDomain, laneRows } from './timeScale'
import { TraceAxis } from './TraceAxis'
import { TraceLane } from './TraceLane'
import { TraceMarkers } from './TraceMarkers'
import { TraceChildRow } from './TraceChildRow'
import { TraceLegend } from './TraceLegend'
import { TraceBreadcrumb, type TraceCrumb } from './TraceBreadcrumb'
import { MARKER_TYPES } from './colors'

function useContainerWidth(defaultWidth = 1000) {
  const ref = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(defaultWidth)
  useEffect(() => {
    const el = ref.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width
      if (w) setWidth(w)
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return { ref, width }
}

/** Gantt-style trace timeline for one workflow instance. */
export function TraceView({
  instanceId,
  sessions,
  workflowState,
  onAgentSelect,
}: {
  instanceId: string
  sessions?: AgentSession[]
  workflowState?: WorkflowState
  onAgentSelect?: (data: SelectedAgentData) => void
}) {
  const [stack, setStack] = useState<TraceCrumb[]>([])
  // Reset child-trace navigation when the host switches instances
  // (adjust-state-during-render pattern; no effect needed).
  const [prevInstanceId, setPrevInstanceId] = useState(instanceId)
  if (prevInstanceId !== instanceId) {
    setPrevInstanceId(instanceId)
    setStack([])
  }
  const currentIid = stack.length > 0 ? stack[stack.length - 1].instanceId : instanceId

  const { data: trace, isLoading, error, dataUpdatedAt } = useTrace(currentIid)
  const [activeTypes, setActiveTypes] = useState<Set<string>>(new Set(MARKER_TYPES))
  const { ref, width } = useContainerWidth()
  const queryClient = useQueryClient()

  // "Now" is the query's data-arrival time, so the running edge advances on
  // WS-driven refetches only — no timers. 0 (test mocks) falls back to the
  // latest observed timestamp inside buildDomain.
  const domain = useMemo(
    () => (trace ? buildDomain(trace, dataUpdatedAt ?? 0) : null),
    [trace, dataUpdatedAt]
  )

  // Throttled marker liveness: the central messages.updated handler is
  // deliberately narrow, so re-pull the trace here at most every 5s and only
  // for sessions this trace actually displays.
  const sessionIdsRef = useRef<Set<string>>(new Set())
  useEffect(() => {
    const ids = new Set<string>()
    for (const lane of trace?.lanes ?? []) {
      for (const seg of lane.segments ?? []) ids.add(seg.session_id)
    }
    sessionIdsRef.current = ids
  }, [trace])
  const lastInvalidateRef = useRef(0)
  useWebSocketEvent((event) => {
    if (event.type !== 'messages.updated') return
    const sid = event.data?.session_id
    if (typeof sid !== 'string' || !sessionIdsRef.current.has(sid)) return
    const now = Date.now()
    if (now - lastInvalidateRef.current < 5000) return
    lastInvalidateRef.current = now
    queryClient.invalidateQueries({ queryKey: traceKeys.instance(currentIid) })
  })

  const selectSession = (phase: string, sessionId: string) => {
    if (!onAgentSelect) return
    onAgentSelect({
      phaseName: phase,
      session: sessions?.find((s) => s.id === sessionId),
      historyEntry: workflowState?.agent_history?.find((h) => h.session_id === sessionId),
      agent: Object.values(workflowState?.active_agents ?? {}).find((a) => a.session_id === sessionId),
    })
  }

  if (isLoading) return <div className="text-sm text-muted-foreground py-8 text-center">Loading trace…</div>
  if (error) return <div className="text-sm text-red-600 dark:text-red-400 py-8 text-center">Failed to load trace</div>
  if (!trace || !domain) return <div className="text-sm text-muted-foreground py-8 text-center">No trace data</div>

  const groups = laneRows(trace.lanes ?? [])
  const rootMarkers = (trace.root_markers ?? []).filter((m) => activeTypes.has(m.type))

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <TraceBreadcrumb
          stack={stack}
          onNavigate={(i) => setStack(i === 0 ? [] : stack.slice(0, i + 1))}
        />
        <TraceLegend
          active={activeTypes}
          truncated={trace.truncated}
          onToggle={(type) =>
            setActiveTypes((prev) => {
              const next = new Set(prev)
              if (next.has(type)) next.delete(type)
              else next.add(type)
              return next
            })
          }
        />
      </div>
      <div className="overflow-x-auto border border-border rounded-lg bg-background">
        <div className="min-w-[800px]" ref={ref}>
          <TraceAxis trace={trace} domain={domain} />
          {groups.length === 0 && (
            <div className="text-sm text-muted-foreground py-6 text-center">No agent sessions yet</div>
          )}
          {groups.map((group) => (
            <div key={group.layer} className={group.layer % 2 === 0 ? 'bg-muted/20' : ''}>
              <div className="grid grid-cols-[10rem_1fr]">
                <div className="px-2 pt-1 text-[10px] uppercase tracking-wide text-muted-foreground sticky left-0">
                  {group.layer >= 0 ? `Layer ${group.layer}` : 'Unassigned'}
                </div>
                <div />
              </div>
              {group.lanes.map((lane) => (
                <TraceLane
                  key={lane.lane_id}
                  lane={lane}
                  markers={(lane.markers ?? []).filter((m) => activeTypes.has(m.type))}
                  domain={domain}
                  widthPx={width}
                  onSelect={(sid) => selectSession(lane.phase, sid)}
                />
              ))}
            </div>
          ))}
          {rootMarkers.length > 0 && (
            <div className="grid grid-cols-[10rem_1fr] border-t border-border/40">
              <div className="px-2 py-1 text-xs text-muted-foreground sticky left-0 bg-background">workflow</div>
              <div className="relative h-9">
                <TraceMarkers markers={rootMarkers} domain={domain} widthPx={width} />
              </div>
            </div>
          )}
          {(trace.children ?? []).map((child) => (
            <TraceChildRow
              key={child.instance_id}
              child={child}
              domain={domain}
              onOpen={(iid) => {
                const next = { instanceId: iid, workflow: child.workflow }
                // The stack always carries the root crumb once a child is open.
                setStack(stack.length === 0 ? [{ instanceId, workflow: trace.workflow }, next] : [...stack, next])
              }}
            />
          ))}
        </div>
      </div>
    </div>
  )
}
