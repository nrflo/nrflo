import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { listSystemAgentRuns } from '@/api/systemAgentRuns'
import { useWebSocketContext } from '@/providers/WebSocketProvider'
import type { WSEvent } from '@/hooks/useWebSocket'

export const systemAgentRunKeys = {
  all: ['system-agent-runs'] as const,
  list: (limit: number) => [...systemAgentRunKeys.all, limit] as const,
}

// TanStack Query hook for the merged system-agent-run listing, plus a WS
// subscription that invalidates on the two events that can change it:
// a session's handoff digest updating, or a refinery fold failing.
// Lifecycle mirrors useSessionHandoffDigest.ts.
export function useSystemAgentRuns(limit: number) {
  const queryClient = useQueryClient()
  const { addEventListener, removeEventListener } = useWebSocketContext()

  const query = useQuery({
    queryKey: systemAgentRunKeys.list(limit),
    queryFn: () => listSystemAgentRuns({ limit }),
  })

  useEffect(() => {
    const handler = (event: WSEvent) => {
      if (event.type !== 'agent.handoff_digest' && event.type !== 'refinery.fold_failed') return
      queryClient.invalidateQueries({ queryKey: systemAgentRunKeys.list(limit) })
    }
    addEventListener(handler)
    return () => removeEventListener(handler)
  }, [limit, addEventListener, removeEventListener, queryClient])

  return query
}
