import type { QueryClient } from '@tanstack/react-query'
import type { WSEventV2 } from './useWSProtocol'
import { isControlEvent } from './useWSProtocol'
import type { WSEvent } from './useWebSocketTypes'
import { handleSessionScopedEvent } from './useWSSessionChannel'
import { dispatchV2Event, persistSeqs, schedulePersistSeqs } from './useWSReducer'
import { handleSnapshotBegin, handleSnapshotChunk, handleSnapshotEnd, isReceivingSnapshot, bufferEventDuringSnapshot } from './useWSSnapshot'
import { runningAgentsKeys } from './useRunningAgents'

const isDev = import.meta.env.DEV

export function handleIncomingEvent(
  event: WSEventV2,
  qc: QueryClient,
  onEvent: ((event: WSEvent) => void) | undefined,
  requestResync: (project: string, ticket: string) => void,
) {
  if (isDev) {
    console.debug('[ws] event:', event.type, event.ticket_id, event.sequence, event.data)
  }

  onEvent?.(event as WSEvent)

  if (event.type === 'global.notifications') {
    qc.invalidateQueries({ queryKey: ['notification-inbox'] })
    return
  }

  // Handle control events
  if (isControlEvent(event.type)) {
    switch (event.type) {
      case 'snapshot.begin':
        handleSnapshotBegin(event)
        return
      case 'snapshot.chunk':
        handleSnapshotChunk(event)
        return
      case 'snapshot.end': {
        const buffered = handleSnapshotEnd(event, qc)
        // Replay buffered live events in order
        for (const e of buffered) {
          dispatchV2Event(e, qc)
        }
        persistSeqs()
        return
      }
      case 'resync.required':
        if (isDev) console.debug('[ws] resync required for', event.project_id, event.ticket_id)
        requestResync(event.project_id, event.ticket_id)
        return
      case 'heartbeat':
        // Heartbeat handled by resetHeartbeat in onmessage
        return
    }
  }

  // Ephemeral session-channel events bypass seq/snapshot entirely — see
  // useWSSessionChannel.ts and the global.running_agents return below.
  if (event.session_id) {
    handleSessionScopedEvent(event, qc)
    return
  }

  // Buffer events that arrive during snapshot
  if (isReceivingSnapshot(event.project_id, event.ticket_id)) {
    bufferEventDuringSnapshot(event.project_id, event.ticket_id, event)
    return
  }

  // Handle test echo
  if (event.type === 'test.echo') {
    console.log('[ws] TEST BROADCAST RECEIVED:', event)
    return
  }

  // Handle global running agents signal (no subscription scope)
  if (event.type === 'global.running_agents') {
    qc.invalidateQueries({ queryKey: runningAgentsKeys.all })
    return
  }

  // Dispatch through v2 reducer (handles seq tracking + cache invalidation)
  dispatchV2Event(event, qc)
  schedulePersistSeqs()
}
