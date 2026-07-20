import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useConnectionsStore } from '../stores/connectionsStore'
import { runningAgentsKeys } from './useRunningAgents'
import type { WSEventV2, WSSubscribeMessage } from './useWSProtocol'
import { isControlEvent, subscriptionKey } from './useWSProtocol'
import { useWSSessionChannel, handleSessionScopedEvent, deniedSessionID } from './useWSSessionChannel'
import { getWebSocketUrl, invalidateAllQueries } from './useWSBootstrap'
import {
  dispatchV2Event,
  getLastSeq,
  setLastSeq,
  persistSeqs,
  restoreSeqs,
  resetSeqs,
} from './useWSReducer'
import {
  handleSnapshotBegin,
  handleSnapshotChunk,
  handleSnapshotEnd,
  isReceivingSnapshot,
  bufferEventDuringSnapshot,
} from './useWSSnapshot'
import { computeReconnectDelay, useConnectionRecovery } from './useWSReconnect'

// Event types from backend
export type WSEventType =
  | 'agent.started'
  | 'agent.completed'
  | 'agent.continued'
  | 'agent.context_updated'
  | 'agent.retry_waiting'
  | 'agent.context_saving'
  | 'agent.stall_restart'
  | 'agent.nudged'
  | 'findings.updated'
  | 'project_findings.updated'
  | 'messages.updated'
  | 'workflow.updated'
  | 'workflow_def.created'
  | 'workflow_def.updated'
  | 'workflow_def.deleted'
  | 'agent_def.created'
  | 'agent_def.updated'
  | 'agent_def.deleted'
  | 'model.created' | 'model.updated' | 'model.deleted'
  | 'orchestration.started'
  | 'orchestration.completed'
  | 'orchestration.failed'
  | 'orchestration.retried'
  | 'orchestration.callback'
  | 'agent.take_control'
  | 'agent.take_control_rejected'
  | 'agent.killed'
  | 'agent.stall_waiting'
  | 'chain.updated'
  | 'layer.skipped'
  | 'merge.conflict_resolving'
  | 'merge.conflict_resolved'
  | 'merge.conflict_failed'
  | 'ticket.updated'
  | 'global.running_agents'
  | 'error.created'
  | 'schedule.created'
  | 'schedule.updated'
  | 'schedule.deleted'
  | 'schedule.triggered'
  | 'notification_channel.created'
  | 'notification_channel.updated'
  | 'notification_channel.deleted'
  | 'notification.delivered'
  | 'notification.failed'
  | 'chain_def.created'
  | 'chain_def.updated'
  | 'chain_def.deleted'
  | 'chain.run_started'
  | 'chain.step_started'
  | 'chain.step_completed'
  | 'chain.run_completed'
  | 'chain.run_failed'
  | 'workflow.finalize_succeeded'
  | 'workflow.finalize_failed'
  | 'workflow.paused'
  | 'workflow.resumed'
  | 'project.env_vars_updated'
  | 'service_tokens.updated'
  | 'spec_import.started'
  | 'spec_import.ready'
  | 'spec_import.failed'
  | 'artifact.created'
  | 'artifact.deleted'
  | 'agent.rate_limited'
  | 'consult.started'
  | 'consult.answered'
  | 'consult.failed'
  | 'plan.drafted'
  | 'plan.revised'
  | 'plan.approved'
  | 'plan.cancelled'
  | 'plan.materialized'
  | 'workflow.plan_waiting'
  | 'test.echo'
  | 'console_chat.delta'
  | 'console_chat.thinking'
  | 'console_chat.turn'
  | 'console_chat.approval_request'
  | 'console_chat.approval_resolved'
  | 'console_chat.error'
  | 'console_chat.session_approvals' | 'console_chat.sibling_opened' | 'session.cost_updated'

export interface WSEvent {
  type: WSEventType
  project_id: string
  ticket_id: string
  workflow?: string
  timestamp: string
  session_id?: string
  data?: Record<string, unknown>
}

interface UseWebSocketOptions {
  enabled?: boolean
  onEvent?: (event: WSEvent) => void
  onSessionSubscriptionDenied?: (sessionId: string) => void
}

interface UseWebSocketReturn {
  isConnected: boolean
  subscribe: (ticketId?: string) => void
  unsubscribe: (ticketId?: string) => void
  subscribeSession: (sessionId: string) => void
  unsubscribeSession: (sessionId: string) => void
}

const BASE_RECONNECT_DELAY = 3000 // 3 seconds
const HEARTBEAT_TIMEOUT = 60_000 // 60 seconds
const isDev = import.meta.env.DEV

// Restore persisted seq state on module load
restoreSeqs()

export function useWebSocket(options: UseWebSocketOptions = {}): UseWebSocketReturn {
  const { enabled = true, onEvent, onSessionSubscriptionDenied } = options
  const queryClient = useQueryClient()
  const activeId = useConnectionsStore((s) => s.activeId)

  const [isConnected, setIsConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectAttemptsRef = useRef(0)
  const reconnectTimeoutRef = useRef<number | null>(null)
  const subscriptionsRef = useRef<Set<string>>(new Set())
  const mountedRef = useRef(true)
  const heartbeatTimerRef = useRef<number | null>(null)
  const prevActiveIdRef = useRef(activeId)

  // Stable refs for callbacks — prevents connect() recreation on handler changes
  const queryClientRef = useRef(queryClient)
  queryClientRef.current = queryClient
  const onEventRef = useRef(onEvent)
  onEventRef.current = onEvent
  const onSessionSubscriptionDeniedRef = useRef(onSessionSubscriptionDenied)
  onSessionSubscriptionDeniedRef.current = onSessionSubscriptionDenied

  const { sessionSubscriptionsRef, subscribeSession, unsubscribeSession, resendSessionSubscriptions } = useWSSessionChannel(wsRef)

  // Reset heartbeat timer — if no message in HEARTBEAT_TIMEOUT, trigger reconnect
  const resetHeartbeat = useCallback(() => {
    if (heartbeatTimerRef.current) {
      clearTimeout(heartbeatTimerRef.current)
    }
    heartbeatTimerRef.current = window.setTimeout(() => {
      if (isDev) console.debug('[ws] heartbeat timeout, reconnecting')
      wsRef.current?.close()
    }, HEARTBEAT_TIMEOUT)
  }, [])

  const requestResync = useCallback((projectId: string, ticketId: string) => {
    if (wsRef.current?.readyState !== WebSocket.OPEN) return
    if (isDev) console.debug('[ws] requesting resync for', projectId, ticketId)
    setLastSeq(subscriptionKey(projectId, ticketId), 0)
    const message: WSSubscribeMessage = {
      action: 'subscribe',
      project_id: projectId,
      ticket_id: ticketId,
      since_seq: 0, // seq=0 forces server to send snapshot
    }
    wsRef.current.send(JSON.stringify(message))
  }, [])

  // Handle incoming WebSocket events via v2 reducer (uses refs, no deps needed)
  const handleEvent = useCallback((event: WSEventV2) => {
    if (isDev) {
      console.debug('[ws] event:', event.type, event.ticket_id, event.sequence, event.data)
    }

    // Call custom handler if provided (cast to WSEvent for backward compat)
    onEventRef.current?.(event as WSEvent)

    const qc = queryClientRef.current

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
    persistSeqs()
  }, [requestResync]) // stable — requestResync uses refs

  // Invalidate all queries on connect/reconnect to catch up on missed events
  const invalidateAll = useCallback(() => {
    invalidateAllQueries(queryClientRef.current)
  }, [])

  // Build subscribe message with cursor for v2 resume
  const buildSubscribeMessage = useCallback((projectId: string, ticketId: string): WSSubscribeMessage => {
    const subKey = subscriptionKey(projectId, ticketId)
    const lastSeq = getLastSeq(subKey)
    const msg: WSSubscribeMessage = {
      action: 'subscribe',
      project_id: projectId,
      ticket_id: ticketId,
    }
    if (lastSeq !== undefined) {
      msg.since_seq = lastSeq
    }
    return msg
  }, [])

  const connect = useCallback(() => {
    if (!enabled || wsRef.current?.readyState === WebSocket.OPEN) {
      return
    }

    // Close any existing connection in CONNECTING state
    if (wsRef.current?.readyState === WebSocket.CONNECTING) {
      wsRef.current.close()
      wsRef.current = null
    }

    const url = getWebSocketUrl()
    if (isDev) console.debug('[ws] connecting to', url)
    const ws = new WebSocket(url)

    ws.onopen = () => {
      if (!mountedRef.current) {
        ws.close()
        return
      }

      if (isDev) console.debug('[ws] connected')
      setIsConnected(true)
      reconnectAttemptsRef.current = 0
      resetHeartbeat()

      const projectId = useConnectionsStore.getState().active().activeProject ?? 'default'
      subscriptionsRef.current.forEach((ticketId) => {
        const message = buildSubscribeMessage(projectId, ticketId)
        if (isDev) console.debug('[ws] subscribe:', message)
        ws.send(JSON.stringify(message))
      })

      resendSessionSubscriptions(ws)

      const hasAnyCursor = Array.from(subscriptionsRef.current).some((ticketId) => {
        const subKey = subscriptionKey(projectId, ticketId)
        return getLastSeq(subKey) !== undefined
      })
      if (!hasAnyCursor) {
        invalidateAll()
      }
    }

    ws.onmessage = (e) => {
      if (!mountedRef.current) return
      resetHeartbeat()

      try {
        // Messages can be newline-separated (batched by server WritePump)
        const lines = e.data.split('\n').filter((line: string) => line.trim())
        for (const line of lines) {
          const message = JSON.parse(line)

          // Ignore ack messages, except session_subscription_denied
          if (message.type === 'ack') {
            if (isDev) console.debug('[ws] ack:', message.action, message.project_id, message.ticket_id)
            const denied = deniedSessionID(message)
            if (denied) onSessionSubscriptionDeniedRef.current?.(denied)
            continue
          }

          handleEvent(message as WSEventV2)
        }
      } catch (err) {
        console.error('[ws] Failed to parse message:', err, e.data)
      }
    }

    ws.onclose = (e) => {
      if (!mountedRef.current) return
      // A connection switch may have already replaced wsRef with a newer socket.
      if (wsRef.current !== null && wsRef.current !== ws) return

      if (isDev) console.debug('[ws] disconnected, code:', e.code, 'reason:', e.reason)
      setIsConnected(false)
      wsRef.current = null

      if (heartbeatTimerRef.current) clearTimeout(heartbeatTimerRef.current)
      heartbeatTimerRef.current = null

      // Attempt reconnection
      if (enabled) {
        if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current)
        const delay = computeReconnectDelay(reconnectAttemptsRef.current, BASE_RECONNECT_DELAY)
        if (isDev) console.debug('[ws] reconnecting in', delay, 'ms (attempt', reconnectAttemptsRef.current + 1, ')')
        reconnectTimeoutRef.current = window.setTimeout(() => {
          reconnectTimeoutRef.current = null; reconnectAttemptsRef.current++; connect()
        }, delay)
      }
    }

    ws.onerror = (err) => {
      console.error('[ws] WebSocket error:', err)
    }

    wsRef.current = ws
  }, [enabled, handleEvent, invalidateAll, resetHeartbeat, buildSubscribeMessage, resendSessionSubscriptions])

  // Disconnect from WebSocket
  const disconnect = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current)
      reconnectTimeoutRef.current = null
    }
    if (heartbeatTimerRef.current) {
      clearTimeout(heartbeatTimerRef.current)
      heartbeatTimerRef.current = null
    }

    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }

    setIsConnected(false)
  }, [])

  // Subscribe to a ticket (or all tickets in project if ticketId is empty)
  const subscribe = useCallback((ticketId = '') => {
    subscriptionsRef.current.add(ticketId)

    if (wsRef.current?.readyState === WebSocket.OPEN) {
      const projectId = useConnectionsStore.getState().active().activeProject ?? 'default'
      const message = buildSubscribeMessage(projectId, ticketId)
      if (isDev) console.debug('[ws] subscribe:', message)
      wsRef.current.send(JSON.stringify(message))
    }
  }, [buildSubscribeMessage])

  // Unsubscribe from a ticket
  const unsubscribe = useCallback((ticketId = '') => {
    subscriptionsRef.current.delete(ticketId)

    if (wsRef.current?.readyState === WebSocket.OPEN) {
      const projectId = useConnectionsStore.getState().active().activeProject ?? 'default'
      const message: WSSubscribeMessage = {
        action: 'unsubscribe',
        project_id: projectId,
        ticket_id: ticketId,
      }
      wsRef.current.send(JSON.stringify(message))
    }
  }, [])

  // Connect on mount, disconnect on unmount
  useEffect(() => {
    mountedRef.current = true

    if (enabled) {
      connect()
    }

    return () => {
      mountedRef.current = false
      persistSeqs()
      disconnect()
    }
  }, [enabled, connect, disconnect])

  // Tear down and reconnect when the active connection changes
  useEffect(() => {
    if (prevActiveIdRef.current === activeId) return
    prevActiveIdRef.current = activeId
    disconnect()
    resetSeqs()
    subscriptionsRef.current.clear()
    sessionSubscriptionsRef.current.clear()
    reconnectAttemptsRef.current = 0
    if (enabled) connect()
  }, [activeId, enabled, disconnect, connect, sessionSubscriptionsRef])

  // Recovery: revive a dead socket immediately on tab focus / browser online, skipping the backoff wait.
  useConnectionRecovery(() => {
    if (!enabled || wsRef.current?.readyState === WebSocket.OPEN || wsRef.current?.readyState === WebSocket.CONNECTING) return
    if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current)
    reconnectTimeoutRef.current = null
    reconnectAttemptsRef.current = 0
    connect()
  }, enabled)

  return {
    isConnected,
    subscribe,
    unsubscribe,
    subscribeSession,
    unsubscribeSession,
  }
}
