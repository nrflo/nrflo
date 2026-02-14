import type { QueryClient } from '@tanstack/react-query'
import type { WSEventV2, SnapshotEntityType } from './useWSProtocol'
import { subscriptionKey } from './useWSProtocol'
import { setLastSeq } from './useWSReducer'
import { ticketKeys, projectWorkflowKeys } from './useTickets'
import { chainKeys } from './useChains'

type SnapshotState = 'idle' | 'receiving' | 'applying'

interface SnapshotSession {
  state: SnapshotState
  chunks: Map<SnapshotEntityType, Record<string, unknown>>
  snapshotSeq: number
  projectId: string
  ticketId: string
  bufferedEvents: WSEventV2[]
}

// Active snapshot sessions keyed by subscription key
const sessions = new Map<string, SnapshotSession>()

// Get buffered events for a subscription (returns and clears them)
export function drainBufferedEvents(subKey: string): WSEventV2[] {
  const session = sessions.get(subKey)
  if (!session) return []
  const events = session.bufferedEvents
  session.bufferedEvents = []
  return events
}

// Check if a subscription is currently receiving a snapshot
export function isReceivingSnapshot(projectId: string, ticketId: string): boolean {
  const key = subscriptionKey(projectId, ticketId)
  const session = sessions.get(key)
  return session?.state === 'receiving'
}

// Handle snapshot.begin control event
export function handleSnapshotBegin(event: WSEventV2): void {
  const key = subscriptionKey(event.project_id, event.ticket_id)
  sessions.set(key, {
    state: 'receiving',
    chunks: new Map(),
    snapshotSeq: event.sequence ?? 0,
    projectId: event.project_id,
    ticketId: event.ticket_id,
    bufferedEvents: [],
  })
}

// Handle snapshot.chunk control event
export function handleSnapshotChunk(event: WSEventV2): void {
  const key = subscriptionKey(event.project_id, event.ticket_id)
  const session = sessions.get(key)
  if (!session || session.state !== 'receiving') return

  const entity = event.entity
  if (entity && event.data) {
    session.chunks.set(entity, event.data)
  }
  // Track the highest seq seen during snapshot
  if (event.sequence && event.sequence > session.snapshotSeq) {
    session.snapshotSeq = event.sequence
  }
}

// Buffer a live event that arrived during snapshot
export function bufferEventDuringSnapshot(
  projectId: string,
  ticketId: string,
  event: WSEventV2,
): boolean {
  const key = subscriptionKey(projectId, ticketId)
  const session = sessions.get(key)
  if (!session || session.state !== 'receiving') return false

  // Only buffer events with seq > snapshot seq
  if (event.sequence !== undefined && event.sequence > session.snapshotSeq) {
    session.bufferedEvents.push(event)
  }
  return true
}

// Handle snapshot.end control event — applies all chunks to cache
export function handleSnapshotEnd(
  event: WSEventV2,
  qc: QueryClient,
): WSEventV2[] {
  const key = subscriptionKey(event.project_id, event.ticket_id)
  const session = sessions.get(key)
  if (!session || session.state !== 'receiving') return []

  session.state = 'applying'
  const { projectId, ticketId, chunks, snapshotSeq } = session
  const isProjectScope = !ticketId && !!projectId

  // Apply each chunk to the appropriate cache
  for (const [entity, data] of chunks) {
    applySnapshotChunk(entity, data, qc, projectId, ticketId, isProjectScope)
  }

  // Update seq tracking to snapshot seq
  if (snapshotSeq > 0) {
    setLastSeq(key, snapshotSeq)
  }

  // Collect buffered events to replay
  const buffered = session.bufferedEvents
  sessions.delete(key)
  return buffered
}

// Apply a single snapshot chunk to the query cache
function applySnapshotChunk(
  entity: SnapshotEntityType,
  data: Record<string, unknown>,
  qc: QueryClient,
  projectId: string,
  ticketId: string,
  isProjectScope: boolean,
): void {
  switch (entity) {
    case 'workflow_state':
      // Full workflow state — invalidate to refetch from server with fresh data
      if (isProjectScope) {
        qc.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(projectId) })
      } else {
        qc.invalidateQueries({ queryKey: ticketKeys.workflow(ticketId) })
        qc.invalidateQueries({ queryKey: ticketKeys.detail(ticketId) })
      }
      break

    case 'agent_sessions':
      if (isProjectScope) {
        qc.invalidateQueries({ queryKey: projectWorkflowKeys.agentSessions(projectId) })
      } else {
        qc.invalidateQueries({ queryKey: ticketKeys.agentSessions(ticketId) })
      }
      break

    case 'findings':
      // Findings are part of workflow state
      if (isProjectScope) {
        qc.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(projectId) })
      } else {
        qc.invalidateQueries({ queryKey: ticketKeys.workflow(ticketId) })
      }
      break

    case 'ticket_detail':
      if (ticketId) {
        qc.invalidateQueries({ queryKey: ticketKeys.detail(ticketId) })
      }
      break

    case 'chain_status':
      qc.invalidateQueries({ queryKey: chainKeys.lists() })
      if (data.chain_id) {
        qc.invalidateQueries({ queryKey: chainKeys.detail(data.chain_id as string) })
      }
      break
  }
}
