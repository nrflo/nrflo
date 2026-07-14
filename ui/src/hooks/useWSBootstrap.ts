import type { QueryClient } from '@tanstack/react-query'
import { useConnectionsStore } from '../stores/connectionsStore'
import { ticketKeys, projectWorkflowKeys, dailyStatsKeys } from './useTickets'
import { chainKeys } from './useChains'
import { scheduleKeys } from './useScheduledTasks'
import { runningAgentsKeys } from './useRunningAgents'
import { errorKeys } from './useErrors'
import { planKeys } from './usePlan'

// Connection bootstrap helpers split out of useWebSocket.ts to stay under the
// file size ratchet — pure/one-shot logic with no hook state of its own.

export function getWebSocketUrl(): string {
  const active = useConnectionsStore.getState().active()
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'

  if (active.isLocal) {
    return `${protocol}//${window.location.host}/api/v1/ws`
  }

  const url = new URL(active.baseURL)
  const wsProtocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  const token = active.token ? `?token=${encodeURIComponent(active.token)}` : ''
  return `${wsProtocol}//${url.host}/api/v1/ws${token}`
}

// Invalidate all queries on connect/reconnect (no resumable cursor) to catch
// up on missed events.
export function invalidateAllQueries(qc: QueryClient): void {
  qc.invalidateQueries({ queryKey: ticketKeys.all })
  qc.invalidateQueries({ queryKey: projectWorkflowKeys.all })
  qc.invalidateQueries({ queryKey: chainKeys.all })
  qc.invalidateQueries({ queryKey: dailyStatsKeys.all })
  qc.invalidateQueries({ queryKey: ['workflow-defs'] })
  qc.invalidateQueries({ queryKey: ['workflows', 'defs'] })
  qc.invalidateQueries({ queryKey: ['agent-defs'] })
  qc.invalidateQueries({ queryKey: ['session-messages'] })
  qc.invalidateQueries({ queryKey: runningAgentsKeys.all })
  qc.invalidateQueries({ queryKey: errorKeys.all })
  qc.invalidateQueries({ queryKey: scheduleKeys.all })
  qc.invalidateQueries({ queryKey: ['notification-channels'] })
  qc.invalidateQueries({ queryKey: planKeys.all })
}
