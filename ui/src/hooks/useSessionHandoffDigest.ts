import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchSessionHandoffDigest } from '@/api/handoffDigest'
import { useWebSocketContext } from '@/providers/WebSocketProvider'
import type { WSEvent } from '@/hooks/useWebSocket'
import type { HandoffDigestEvent } from '@/types/handoffDigest'

export function handoffDigestKeys(sessionId: string | undefined) {
  return ['session-handoff-digest', sessionId] as const
}

// applyDigestEvent is the pure merge step for a WS agent.handoff_digest
// payload: replaces the live digest when the event's session_id matches,
// otherwise leaves state untouched (event is for a different session).
export function applyDigestEvent(
  prev: HandoffDigestEvent | undefined,
  payload: HandoffDigestEvent,
  sessionId: string | undefined
): HandoffDigestEvent | undefined {
  if (!sessionId || payload.session_id !== sessionId) return prev
  return payload
}

// TanStack Query hook (mirrors useSessionContextLedger) for the durable
// handoff digest, plus a WS agent.handoff_digest subscription that keeps a
// live overlay current and invalidates the query on match — see
// ui/src/hooks/CLAUDE.md (WS-only realtime, no polling).
export function useSessionHandoffDigest(sessionId: string | undefined, enabled: boolean) {
  const queryClient = useQueryClient()
  const { addEventListener, removeEventListener } = useWebSocketContext()
  const [live, setLive] = useState<HandoffDigestEvent | undefined>(undefined)

  const query = useQuery({
    queryKey: handoffDigestKeys(sessionId),
    queryFn: () => fetchSessionHandoffDigest(sessionId!),
    enabled: !!sessionId && enabled,
    staleTime: Infinity,
  })

  useEffect(() => {
    if (!sessionId || !enabled) return
    const handler = (event: WSEvent) => {
      if (event.type !== 'agent.handoff_digest') return
      const payload = event.data as unknown as HandoffDigestEvent
      setLive((prev) => applyDigestEvent(prev, payload, sessionId))
      if (payload.session_id === sessionId) {
        queryClient.invalidateQueries({ queryKey: handoffDigestKeys(sessionId) })
      }
    }
    addEventListener(handler)
    return () => removeEventListener(handler)
  }, [sessionId, enabled, addEventListener, removeEventListener, queryClient])

  return { ...query, live }
}
