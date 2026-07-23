import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchStepCursors } from '@/api/stepCursors'
import { useWebSocketContext } from '@/providers/WebSocketProvider'
import { applyStepAdvanced } from '@/lib/stepProgress'
import type { WSEvent } from '@/hooks/useWebSocket'
import type { StepAdvancedEvent, StepCursorsResponse } from '@/types/stepwise'

export const stepCursorKeys = {
  all: ['step-cursors'] as const,
  instance: (instanceId: string | undefined) => [...stepCursorKeys.all, instanceId] as const,
}

// TanStack Query hook keyed on the instance id alone so every agent card on
// the phase graph shares one in-flight request, plus a WS step.advanced
// subscription that patches the matching node's cursor and invalidates the
// query. Lifecycle copied from useSessionContextLedger.ts.
export function useStepCursors(instanceId?: string) {
  const queryClient = useQueryClient()
  const { addEventListener, removeEventListener } = useWebSocketContext()

  const query = useQuery({
    queryKey: stepCursorKeys.instance(instanceId),
    queryFn: () => fetchStepCursors(instanceId!),
    enabled: !!instanceId,
  })

  useEffect(() => {
    if (!instanceId) return
    const handler = (event: WSEvent) => {
      if (event.type !== 'step.advanced') return
      const payload = event.data as unknown as StepAdvancedEvent
      if (payload.workflow_instance_id !== instanceId) return

      queryClient.setQueryData<StepCursorsResponse>(
        stepCursorKeys.instance(instanceId),
        (prev) => {
          if (!prev) return prev
          const patched = applyStepAdvanced(prev.cursors[payload.node_id], payload)
          if (!patched) return prev
          return { ...prev, cursors: { ...prev.cursors, [payload.node_id]: patched } }
        }
      )
      queryClient.invalidateQueries({ queryKey: stepCursorKeys.instance(instanceId) })
    }
    addEventListener(handler)
    return () => removeEventListener(handler)
  }, [instanceId, addEventListener, removeEventListener, queryClient])

  return query
}
