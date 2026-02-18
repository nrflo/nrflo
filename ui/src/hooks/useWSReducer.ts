import type { QueryClient } from '@tanstack/react-query'
import type { WSEventV2 } from './useWSProtocol'
import { subscriptionKey } from './useWSProtocol'
import { ticketKeys, projectWorkflowKeys, dailyStatsKeys } from './useTickets'
import { chainKeys } from './useChains'
import type { WSEventType } from './useWebSocket'

// Seq tracking per subscription
const seqMap = new Map<string, number>()

export function getLastSeq(subKey: string): number | undefined {
  return seqMap.get(subKey)
}

export function setLastSeq(subKey: string, seq: number): void {
  seqMap.set(subKey, seq)
}

export function getAllSeqs(): Map<string, number> {
  return new Map(seqMap)
}

export function clearSeqs(): void {
  seqMap.clear()
}

// Persist seq map to sessionStorage for tab-refresh resume
const STORAGE_KEY = 'ws_last_seqs'

export function persistSeqs(): void {
  try {
    const obj: Record<string, number> = {}
    seqMap.forEach((v, k) => { obj[k] = v })
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(obj))
  } catch { /* quota exceeded or unavailable */ }
}

export function restoreSeqs(): void {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    if (!raw) return
    const obj = JSON.parse(raw) as Record<string, number>
    for (const [k, v] of Object.entries(obj)) {
      seqMap.set(k, v)
    }
  } catch { /* parse error or unavailable */ }
}

// Gap detection result
export type GapResult =
  | { type: 'ok' }
  | { type: 'duplicate' }
  | { type: 'gap'; expected: number; got: number }

export function checkSeq(subKey: string, seq: number): GapResult {
  const last = seqMap.get(subKey)
  if (last === undefined) {
    // First event for this subscription — accept it
    return { type: 'ok' }
  }
  if (seq <= last) {
    return { type: 'duplicate' }
  }
  // Global seq may have gaps per subscription scope, so we accept any seq > last
  return { type: 'ok' }
}

// Dispatch a v2 event to the appropriate cache patch handler.
// Returns true if the event was handled (not duplicate).
export function dispatchV2Event(
  event: WSEventV2,
  qc: QueryClient,
): boolean {
  const { project_id, ticket_id } = event
  const subKey = subscriptionKey(project_id, ticket_id)
  const seq = event.sequence

  // Seq tracking and idempotency
  if (seq !== undefined) {
    const result = checkSeq(subKey, seq)
    if (result.type === 'duplicate') return false
    setLastSeq(subKey, seq)
  }

  const isProjectScope = !ticket_id && !!project_id
  const handler = eventHandlers[event.type as WSEventType]

  if (handler) {
    handler(event, qc, isProjectScope)
  }

  return true
}

// Helper: invalidate project or ticket workflow queries
function invalidateWorkflow(
  event: WSEventV2,
  qc: QueryClient,
  isProjectScope: boolean,
) {
  if (isProjectScope) {
    qc.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })
  } else {
    qc.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    qc.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
  }
}

type EventHandler = (
  event: WSEventV2,
  qc: QueryClient,
  isProjectScope: boolean,
) => void

// Handler map per event type.
// For v2, the server includes full state in workflow.updated events,
// so most entity events still invalidate — the key change is that we
// track seq for cursor resume and can skip duplicates.
// Deterministic setQueryData patches will be added incrementally
// as the server enriches event payloads with full entity data.
const eventHandlers: Partial<Record<WSEventType, EventHandler>> = {
  'agent.started': (event, qc, isProjectScope) => {
    if (isProjectScope) {
      qc.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })
      qc.invalidateQueries({ queryKey: projectWorkflowKeys.agentSessions(event.project_id) })
    } else {
      qc.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
      qc.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
      qc.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })
    }
  },

  'agent.completed': (event, qc, isProjectScope) => {
    if (isProjectScope) {
      qc.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })
      qc.invalidateQueries({ queryKey: projectWorkflowKeys.agentSessions(event.project_id) })
    } else {
      qc.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
      qc.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
      qc.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })
    }
  },

  'agent.continued': (event, qc, isProjectScope) => {
    if (isProjectScope) {
      qc.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })
      qc.invalidateQueries({ queryKey: projectWorkflowKeys.agentSessions(event.project_id) })
    } else {
      qc.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
      qc.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
      qc.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })
    }
  },

  'agent.context_updated': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
  },

  'phase.started': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
    if (!isProjectScope) {
      qc.invalidateQueries({ queryKey: ticketKeys.lists() })
    }
  },

  'phase.completed': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
    if (!isProjectScope) {
      qc.invalidateQueries({ queryKey: ticketKeys.lists() })
    }
  },

  'findings.updated': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
  },

  'messages.updated': (event, qc, isProjectScope) => {
    if (event.data?.session_id) {
      qc.invalidateQueries({ queryKey: ['session-messages', event.data.session_id] })
    }
    if (isProjectScope) {
      qc.invalidateQueries({ queryKey: projectWorkflowKeys.agentSessions(event.project_id) })
    } else {
      qc.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })
    }
  },

  'workflow.updated': (event, qc, isProjectScope) => {
    if (isProjectScope) {
      qc.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })
    } else {
      qc.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
      qc.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
      qc.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })
      qc.invalidateQueries({ queryKey: ticketKeys.lists() })
    }
  },

  'workflow_def.created': (_event, qc) => {
    qc.invalidateQueries({ queryKey: ['workflow-defs'] })
    qc.invalidateQueries({ queryKey: ['workflows', 'defs'] })
  },
  'workflow_def.updated': (_event, qc) => {
    qc.invalidateQueries({ queryKey: ['workflow-defs'] })
    qc.invalidateQueries({ queryKey: ['workflows', 'defs'] })
  },
  'workflow_def.deleted': (_event, qc) => {
    qc.invalidateQueries({ queryKey: ['workflow-defs'] })
    qc.invalidateQueries({ queryKey: ['workflows', 'defs'] })
  },

  'agent_def.created': (_event, qc) => {
    qc.invalidateQueries({ queryKey: ['workflow-defs'] })
    qc.invalidateQueries({ queryKey: ['workflows', 'defs'] })
    qc.invalidateQueries({ queryKey: ['agent-defs'] })
  },
  'agent_def.updated': (_event, qc) => {
    qc.invalidateQueries({ queryKey: ['workflow-defs'] })
    qc.invalidateQueries({ queryKey: ['workflows', 'defs'] })
    qc.invalidateQueries({ queryKey: ['agent-defs'] })
  },
  'agent_def.deleted': (_event, qc) => {
    qc.invalidateQueries({ queryKey: ['workflow-defs'] })
    qc.invalidateQueries({ queryKey: ['workflows', 'defs'] })
    qc.invalidateQueries({ queryKey: ['agent-defs'] })
  },

  'orchestration.started': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
    if (!isProjectScope) {
      qc.invalidateQueries({ queryKey: ticketKeys.status() })
      qc.invalidateQueries({ queryKey: ticketKeys.lists() })
      qc.invalidateQueries({ queryKey: dailyStatsKeys.all })
    }
  },
  'orchestration.completed': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
    if (!isProjectScope) {
      qc.invalidateQueries({ queryKey: ticketKeys.status() })
      qc.invalidateQueries({ queryKey: ticketKeys.lists() })
      qc.invalidateQueries({ queryKey: dailyStatsKeys.all })
    }
  },
  'orchestration.failed': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
    if (!isProjectScope) {
      qc.invalidateQueries({ queryKey: ticketKeys.status() })
      qc.invalidateQueries({ queryKey: ticketKeys.lists() })
      qc.invalidateQueries({ queryKey: dailyStatsKeys.all })
    }
  },
  'orchestration.retried': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
  },
  'orchestration.callback': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
  },

  'chain.updated': (event, qc) => {
    qc.invalidateQueries({ queryKey: chainKeys.lists() })
    if (event.data?.chain_id) {
      qc.invalidateQueries({ queryKey: chainKeys.detail(event.data.chain_id as string) })
    }
  },

  'ticket.updated': (event, qc) => {
    qc.invalidateQueries({ queryKey: ticketKeys.status() })
    qc.invalidateQueries({ queryKey: ticketKeys.lists() })
    qc.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    qc.invalidateQueries({ queryKey: dailyStatsKeys.all })
  },
}
