import { useEffect, useCallback } from 'react'
import { Eye } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { useExperimentalObserverEnabled } from '@/hooks/useGlobalSettings'
import { useObservers, observerKeys } from '@/hooks/useObservers'
import { useInteractiveSessionsStore } from '@/stores/interactiveSessionsStore'
import { useWebSocketContext } from '@/providers/WebSocketProvider'
import { useQueryClient } from '@tanstack/react-query'
import { useProjectStore } from '@/stores/projectStore'
import type { WSEvent } from '@/hooks/useWebSocket'

export function ActiveObserversPanel() {
  const enabled = useExperimentalObserverEnabled()
  const { data } = useObservers()
  const project = useProjectStore((s) => s.currentProject)
  const add = useInteractiveSessionsStore((s) => s.add)
  const qc = useQueryClient()
  const { addEventListener, removeEventListener } = useWebSocketContext()

  const handleWSEvent = useCallback(
    (event: WSEvent) => {
      if (event.type !== 'agent.started' && event.type !== 'agent.completed') return
      if (event.data?.kind !== 'observer') return
      qc.invalidateQueries({ queryKey: observerKeys.list(project) })
    },
    [qc, project]
  )

  useEffect(() => {
    addEventListener(handleWSEvent)
    return () => removeEventListener(handleWSEvent)
  }, [addEventListener, removeEventListener, handleWSEvent])

  if (!enabled) return null

  const sessions = data?.sessions ?? []
  if (sessions.length === 0) return null

  return (
    <div className="fixed bottom-0 left-0 z-30 p-3 space-y-1 max-w-xs">
      <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground mb-1">
        <Eye className="h-3.5 w-3.5" />
        Active Observers
      </div>
      {sessions.map((s) => (
        <div key={s.id} className="flex items-center justify-between gap-2 rounded-md border border-border bg-background px-2 py-1 text-xs">
          <span className="text-muted-foreground truncate">{s.workflow || 'observer'} ({s.id.slice(0, 6)})</span>
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs shrink-0"
            onClick={() => add({
              sessionId: s.id,
              agentType: 'observer',
              scope: { type: 'project', projectId: s.project_id || project },
              workflow: s.workflow || 'observer',
              startedAt: s.started_at ? new Date(s.started_at).getTime() : Date.now(),
            })}
          >
            Attach
          </Button>
        </div>
      ))}
    </div>
  )
}
