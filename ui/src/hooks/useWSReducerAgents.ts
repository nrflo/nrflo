import { ticketKeys, projectWorkflowKeys } from './useTickets'
import { traceKeys } from './useTrace'
import { systemAgentRunKeys } from './useSystemAgentRuns'
import { sessionKeys } from './useSessions'
import { sessionFlowKeys } from './useSessionFlow'
import { throttledInvalidate as inv } from './useWSInvalidate'
import type { WSEventHandler } from './useWSReducerDefs'
import type { WSEventV2 } from './useWSProtocol'
import type { WSEventType } from './useWebSocket'
import type { QueryClient } from '@tanstack/react-query'

// Helper: workflow + agent-session queries — the common agent-lifecycle set.
// Kept here (not imported back from useWSReducer.ts) to avoid a circular import.
export const invalidateAgents: WSEventHandler = (event, qc, isProjectScope) => {
  if (isProjectScope) {
    inv(qc, projectWorkflowKeys.workflow(event.project_id))
    inv(qc, projectWorkflowKeys.agentSessions(event.project_id))
  } else {
    inv(qc, ticketKeys.detail(event.ticket_id))
    inv(qc, ticketKeys.workflow(event.ticket_id))
    inv(qc, ticketKeys.agentSessions(event.ticket_id))
  }
}

// A delegate/consult lifecycle event can change the Sessions tab's list
// (new row) and, when it targets a session already open in the detail pane,
// that session's flow/stats.
function invalidateSessionViews(event: WSEventV2, qc: QueryClient) {
  inv(qc, sessionKeys.all)
  if (event.session_id) {
    inv(qc, sessionFlowKeys.flow(event.session_id))
    inv(qc, sessionFlowKeys.stats(event.session_id))
  }
}

const consultHandler: WSEventHandler = (event, qc, isProjectScope) => {
  invalidateAgents(event, qc, isProjectScope)
  inv(qc, ['session-messages'])
  invalidateSessionViews(event, qc)
}

// Delegate workers are trace sub-lanes (nrworkflow-3b3511) and show up in the
// System Agents Activity table (nrworkflow-401788), so a delegate lifecycle
// event invalidates both alongside the usual agent-session set.
const delegateHandler: WSEventHandler = (event, qc, isProjectScope) => {
  invalidateAgents(event, qc, isProjectScope)
  inv(qc, traceKeys.all)
  inv(qc, systemAgentRunKeys.all)
  invalidateSessionViews(event, qc)
}

export const childAgentHandlers: Partial<Record<WSEventType, WSEventHandler>> = {
  'consult.started': consultHandler,
  'consult.answered': consultHandler,
  'consult.failed': consultHandler,
  'delegate.started': delegateHandler,
  'delegate.completed': delegateHandler,
  'delegate.failed': delegateHandler,
}
