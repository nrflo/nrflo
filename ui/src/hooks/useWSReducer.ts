import type { QueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import type { WSEventV2 } from './useWSProtocol'
import { subscriptionKey } from './useWSProtocol'
import { ticketKeys, projectWorkflowKeys, dailyStatsKeys } from './useTickets'
import { chainKeys } from './useChains'
import { scheduleKeys } from './useScheduledTasks'
import { workflowChainKeys, workflowChainRunKeys } from './useWorkflowChains'
import { runningAgentsKeys } from './useRunningAgents'
import { errorKeys } from './useErrors'
import { agentSessionLogKeys } from './useAgentSessionLogs'
import { projectEnvVarKeys } from './useProjectEnvVars'
import { serviceTokenKeys } from './useServiceTokens'
import { artifactKeys } from './useArtifacts'
import { traceKeys } from './useTrace'
import { planKeys } from './usePlan'
import { throttledInvalidate as inv } from './useWSInvalidate'
import { defRegistryHandlers, type WSEventHandler } from './useWSReducerDefs'
import { invalidateAgents, childAgentHandlers } from './useWSReducerAgents'
import type { WSEventType } from './useWebSocket'
import type { LiveAgentSessionsResponse } from '@/types/agentSessionLogs'

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

// Trailing-debounced persist for the per-event hot path; direct persistSeqs
// stays for unmount/snapshot boundaries where the write must land now.
let persistTimer: ReturnType<typeof setTimeout> | null = null

export function schedulePersistSeqs(): void {
  if (persistTimer) return
  persistTimer = setTimeout(() => {
    persistTimer = null
    persistSeqs()
  }, 1000)
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

export function resetSeqs(): void {
  seqMap.clear()
  sessionStorage.removeItem(STORAGE_KEY)
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
    inv(qc, projectWorkflowKeys.workflow(event.project_id))
  } else {
    inv(qc, ticketKeys.detail(event.ticket_id))
    inv(qc, ticketKeys.workflow(event.ticket_id))
  }
}

// Helper: workflow lifecycle set — workflow state plus ticket lists
const invalidateWorkflowLifecycle: WSEventHandler = (event, qc, isProjectScope) => {
  if (isProjectScope) {
    inv(qc, projectWorkflowKeys.workflow(event.project_id))
  } else {
    inv(qc, ticketKeys.detail(event.ticket_id))
    inv(qc, ticketKeys.workflow(event.ticket_id))
    inv(qc, ticketKeys.agentSessions(event.ticket_id))
    inv(qc, ticketKeys.lists())
  }
}

const planHandler: WSEventHandler = (event, qc, isProjectScope) => {
  const instanceId = event.data?.instance_id as string | undefined
  if (instanceId) inv(qc, planKeys.detail(instanceId))
  invalidateWorkflow(event, qc, isProjectScope)
}

const orchestrationLifecycleHandler: WSEventHandler = (event, qc, isProjectScope) => {
  invalidateWorkflow(event, qc, isProjectScope)
  if (!isProjectScope) {
    inv(qc, ticketKeys.status())
    inv(qc, ticketKeys.lists())
    inv(qc, dailyStatsKeys.all)
  }
}

const chainRunHandler: WSEventHandler = (_event, qc) => {
  inv(qc, workflowChainRunKeys.all)
}

const notificationChannelHandler: WSEventHandler = (_event, qc) => {
  inv(qc, ['notification-channels'])
}

const notificationDeliveryHandler: WSEventHandler = (event, qc) => {
  inv(qc, ['notification-channels'])
  if (event.data?.channel_id) {
    inv(qc, ['notification-deliveries', event.data.channel_id as string])
  }
}

const artifactHandler: WSEventHandler = (event, qc) => {
  const iid = event.data?.workflow_instance_id as string | undefined
  if (iid) inv(qc, artifactKeys.instance(iid))
}

// Handler map per event type. Heavy queries (workflow state, agent sessions)
// go through throttledInvalidate so event bursts refetch at most ~1/s per key.
const eventHandlers: Partial<Record<WSEventType, WSEventHandler>> = {
  'agent.started': (event, qc, isProjectScope) => {
    invalidateAgents(event, qc, isProjectScope)
    // Only active (mounted) trace queries refetch, so the broad key is cheap.
    inv(qc, traceKeys.all)
  },

  'agent.completed': (event, qc, isProjectScope) => {
    invalidateAgents(event, qc, isProjectScope)
    if (!isProjectScope) inv(qc, ticketKeys.lists())
    inv(qc, agentSessionLogKeys.all)
    inv(qc, traceKeys.all)
  },

  ...childAgentHandlers,

  'agent.continued': invalidateAgents,
  'agent.take_control': invalidateAgents,
  'agent.killed': invalidateAgents,
  'agent.retry_waiting': invalidateAgents,
  'agent.context_saving': invalidateAgents,
  'agent.stall_restart': invalidateAgents,
  'agent.nudged': invalidateAgents,

  'agent.take_control_rejected': () => {
    toast.error('Take-control is not supported for API-mode agents.')
  },

  'agent.rate_limited': (event, qc, isProjectScope) => {
    const sessionId = event.data?.session_id as string | undefined
    const waitSeconds = event.data?.wait_seconds as number | undefined
    if (!sessionId || waitSeconds === undefined) return

    const rateLimitUntilTs = new Date(Date.now() + waitSeconds * 1000).toISOString()
    const cacheKey = agentSessionLogKeys.live(event.project_id)
    const prev = qc.getQueryData<LiveAgentSessionsResponse>(cacheKey)

    if (prev) {
      qc.setQueryData<LiveAgentSessionsResponse>(cacheKey, {
        ...prev,
        sessions: prev.sessions.map((s) =>
          s.session_id === sessionId
            ? {
                ...s,
                rate_limit_until_ts: rateLimitUntilTs,
                rate_limit_wait_seconds: waitSeconds,
                rate_limit_total_wait_seconds: event.data?.total_wait_seconds as number | undefined,
                rate_limit_matched_pattern: event.data?.matched_pattern as string | undefined,
                rate_limit_retry_count: event.data?.retry_count as number | undefined,
              }
            : s
        ),
      })
    } else {
      inv(qc, agentSessionLogKeys.all)
    }

    inv(qc, runningAgentsKeys.all)
    invalidateWorkflow(event, qc, isProjectScope)
  },

  'agent.context_updated': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
  },

  'findings.updated': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
  },

  'project_findings.updated': (event, qc) => {
    inv(qc, projectWorkflowKeys.findings(event.project_id))
    inv(qc, projectWorkflowKeys.workflow(event.project_id))
  },

  'project.env_vars_updated': (event, qc) => {
    inv(qc, projectEnvVarKeys.list(event.project_id))
  },

  'service_tokens.updated': (_event, qc) => {
    inv(qc, serviceTokenKeys.all)
  },

  'messages.updated': (event, qc, _isProjectScope) => {
    // Fires roughly every 2s while an agent is running. Only invalidate the
    // narrow per-session messages cache; the heavy workflow/agent-sessions
    // queries are refetched on lifecycle events (agent.*, workflow.updated)
    // — refetching them on every message tick is what made the Sidebar
    // re-pull the multi-MB /projects/{id}/workflow endpoint dozens of times
    // per run.
    if (event.data?.session_id) {
      inv(qc, ['session-messages', event.data.session_id])
    }
  },

  'workflow.updated': (event, qc, isProjectScope) => {
    invalidateWorkflowLifecycle(event, qc, isProjectScope)
    inv(qc, traceKeys.all)
  },

  'workflow.finalize_succeeded': invalidateWorkflowLifecycle,
  'workflow.finalize_failed': invalidateWorkflowLifecycle,
  'workflow.paused': invalidateWorkflowLifecycle,
  'workflow.resumed': invalidateWorkflowLifecycle,

  'plan.drafted': planHandler,
  'plan.revised': planHandler,
  'plan.approved': planHandler,
  'plan.cancelled': planHandler,
  'plan.materialized': planHandler,
  'workflow.plan_waiting': planHandler,

  ...defRegistryHandlers,

  'orchestration.started': orchestrationLifecycleHandler,
  'orchestration.completed': orchestrationLifecycleHandler,
  'orchestration.failed': orchestrationLifecycleHandler,
  'orchestration.retried': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
  },
  'orchestration.callback': (event, qc, isProjectScope) => {
    invalidateWorkflow(event, qc, isProjectScope)
  },

  'layer.skipped': (event, qc, isProjectScope) => {
    invalidateAgents(event, qc, isProjectScope)
    if (!isProjectScope) inv(qc, ticketKeys.lists())
  },

  'merge.conflict_resolving': (event, qc, isProjectScope) => {
    invalidateAgents(event, qc, isProjectScope)
    inv(qc, runningAgentsKeys.all)
  },
  'merge.conflict_resolved': invalidateAgents,
  'merge.conflict_failed': invalidateAgents,

  'chain.updated': (event, qc) => {
    inv(qc, chainKeys.lists())
    if (event.data?.chain_id) {
      inv(qc, chainKeys.detail(event.data.chain_id as string))
    }
  },

  'ticket.updated': (event, qc) => {
    inv(qc, ticketKeys.status())
    inv(qc, ticketKeys.lists())
    inv(qc, ticketKeys.detail(event.ticket_id))
    inv(qc, dailyStatsKeys.all)
  },

  'error.created': (_event, qc) => {
    inv(qc, errorKeys.all)
  },

  'schedule.created': (_event, qc) => {
    inv(qc, scheduleKeys.all)
  },
  'schedule.updated': (_event, qc) => {
    inv(qc, scheduleKeys.all)
  },
  'schedule.deleted': (_event, qc) => {
    inv(qc, scheduleKeys.all)
  },
  'schedule.triggered': (event, qc) => {
    inv(qc, scheduleKeys.all)
    if (event.data?.task_id) {
      inv(qc, scheduleKeys.runs(event.data.task_id as string))
    }
  },

  'notification_channel.created': notificationChannelHandler,
  'notification_channel.updated': notificationChannelHandler,
  'notification_channel.deleted': notificationChannelHandler,
  'notification.delivered': notificationDeliveryHandler,
  'notification.failed': notificationDeliveryHandler,

  'chain_def.created': (_event, qc) => {
    inv(qc, workflowChainKeys.all)
  },
  'chain_def.updated': (_event, qc) => {
    inv(qc, workflowChainKeys.all)
  },
  'chain_def.deleted': (_event, qc) => {
    inv(qc, workflowChainKeys.all)
  },

  'chain.run_started': chainRunHandler,
  'chain.step_started': chainRunHandler,
  'chain.step_completed': chainRunHandler,
  'chain.run_completed': chainRunHandler,
  'chain.run_failed': chainRunHandler,

  'artifact.created': artifactHandler,
  'artifact.deleted': artifactHandler,
}
