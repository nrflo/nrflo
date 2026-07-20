import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchSessionContextLedger } from '@/api/contextLedger'
import { useWebSocketContext } from '@/providers/WebSocketProvider'
import type { WSEvent } from '@/hooks/useWebSocket'
import type { LedgerLiveTotals } from '@/types/contextLedger'

export function contextLedgerKeys(sessionId: string | undefined) {
  return ['session-context-ledger', sessionId] as const
}

// applyLedgerEvent is the pure merge step for a WS agent.context_ledger
// payload: replaces the live totals when the event's session_id matches,
// otherwise leaves state untouched (event is for a different session).
export function applyLedgerEvent(
  prev: LedgerLiveTotals | undefined,
  payload: LedgerLiveTotals,
  sessionId: string | undefined
): LedgerLiveTotals | undefined {
  if (!sessionId || payload.session_id !== sessionId) return prev
  return payload
}

// TanStack Query hook (mirrors useSessionPrompt) for the full snapshot, plus
// a WS agent.context_ledger subscription that keeps a live totals-only
// summary current and invalidates the snapshot query so entries refetch —
// see ui/src/hooks/CLAUDE.md (WS-only realtime, no polling).
export function useSessionContextLedger(sessionId: string | undefined, enabled: boolean) {
  const queryClient = useQueryClient()
  const { addEventListener, removeEventListener } = useWebSocketContext()
  const [liveTotals, setLiveTotals] = useState<LedgerLiveTotals | undefined>(undefined)

  const query = useQuery({
    queryKey: contextLedgerKeys(sessionId),
    queryFn: () => fetchSessionContextLedger(sessionId!),
    enabled: !!sessionId && enabled,
    staleTime: Infinity,
  })

  useEffect(() => {
    if (!sessionId || !enabled) return
    const handler = (event: WSEvent) => {
      if (event.type !== 'agent.context_ledger') return
      const payload = event.data as unknown as LedgerLiveTotals
      setLiveTotals((prev) => applyLedgerEvent(prev, payload, sessionId))
      if (payload.session_id === sessionId) {
        queryClient.invalidateQueries({ queryKey: contextLedgerKeys(sessionId) })
      }
    }
    addEventListener(handler)
    return () => removeEventListener(handler)
  }, [sessionId, enabled, addEventListener, removeEventListener, queryClient])

  return { ...query, liveTotals }
}
