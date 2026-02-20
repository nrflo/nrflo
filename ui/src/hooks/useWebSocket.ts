import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { getProject } from '../api/client'
import { ticketKeys, projectWorkflowKeys, dailyStatsKeys } from './useTickets'
import { chainKeys } from './useChains'
import type { WSEventV2, WSSubscribeMessage } from './useWSProtocol'
import { isControlEvent, subscriptionKey } from './useWSProtocol'
import {
  dispatchV2Event,
  getLastSeq,
  persistSeqs,
  restoreSeqs,
} from './useWSReducer'
import {
  handleSnapshotBegin,
  handleSnapshotChunk,
  handleSnapshotEnd,
  isReceivingSnapshot,
  bufferEventDuringSnapshot,
} from './useWSSnapshot'

// Event types from backend
export type WSEventType =
  | 'agent.started'
  | 'agent.completed'
  | 'agent.continued'
  | 'agent.context_updated'
  | 'findings.updated'
  | 'messages.updated'
  | 'workflow.updated'
  | 'workflow_def.created'
  | 'workflow_def.updated'
  | 'workflow_def.deleted'
  | 'agent_def.created'
  | 'agent_def.updated'
  | 'agent_def.deleted'
  | 'orchestration.started'
  | 'orchestration.completed'
  | 'orchestration.failed'
  | 'orchestration.retried'
  | 'orchestration.callback'
  | 'agent.take_control'
  | 'chain.updated'
  | 'ticket.updated'
  | 'test.echo'

export interface WSEvent {
  type: WSEventType
  project_id: string
  ticket_id: string
  workflow?: string
  timestamp: string
  data?: Record<string, unknown>
}

interface UseWebSocketOptions {
  enabled?: boolean
  onEvent?: (event: WSEvent) => void
}

interface UseWebSocketReturn {
  isConnected: boolean
  subscribe: (ticketId?: string) => void
  unsubscribe: (ticketId?: string) => void
}

const MAX_RECONNECT_ATTEMPTS = 5
const BASE_RECONNECT_DELAY = 3000 // 3 seconds
const HEARTBEAT_TIMEOUT = 60_000 // 60 seconds
const isDev = import.meta.env.DEV

function getWebSocketUrl(): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host

  const apiUrl = import.meta.env.VITE_API_URL
  if (apiUrl) {
    const url = new URL(apiUrl)
    return `${protocol}//${url.host}/api/v1/ws`
  }

  return `${protocol}//${host}/api/v1/ws`
}

// Restore persisted seq state on module load
restoreSeqs()

export function useWebSocket(options: UseWebSocketOptions = {}): UseWebSocketReturn {
  const { enabled = true, onEvent } = options
  const queryClient = useQueryClient()

  const [isConnected, setIsConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectAttemptsRef = useRef(0)
  const reconnectTimeoutRef = useRef<number | null>(null)
  const subscriptionsRef = useRef<Set<string>>(new Set())
  const mountedRef = useRef(true)
  const heartbeatTimerRef = useRef<number | null>(null)

  // Use refs for callbacks to avoid dependency chain issues.
  // This prevents connect() from being recreated when handlers change,
  // which would cause unnecessary WS disconnect/reconnect cycles.
  const queryClientRef = useRef(queryClient)
  queryClientRef.current = queryClient
  const onEventRef = useRef(onEvent)
  onEventRef.current = onEvent

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

  // Request resync for a subscription
  const requestResync = useCallback((projectId: string, ticketId: string) => {
    if (wsRef.current?.readyState !== WebSocket.OPEN) return
    if (isDev) console.debug('[ws] requesting resync for', projectId, ticketId)
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

    // Dispatch through v2 reducer (handles seq tracking + cache invalidation)
    dispatchV2Event(event, qc)
    persistSeqs()
  }, [requestResync]) // stable — requestResync uses refs

  // Invalidate all queries on connect/reconnect to catch up on missed events
  const invalidateAll = useCallback(() => {
    const qc = queryClientRef.current
    qc.invalidateQueries({ queryKey: ticketKeys.all })
    qc.invalidateQueries({ queryKey: projectWorkflowKeys.all })
    qc.invalidateQueries({ queryKey: chainKeys.all })
    qc.invalidateQueries({ queryKey: dailyStatsKeys.all })
    qc.invalidateQueries({ queryKey: ['workflow-defs'] })
    qc.invalidateQueries({ queryKey: ['workflows', 'defs'] })
    qc.invalidateQueries({ queryKey: ['agent-defs'] })
    qc.invalidateQueries({ queryKey: ['session-messages'] })
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

  // Connect to WebSocket
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

      // Re-subscribe with cursor resume
      const projectId = getProject()
      subscriptionsRef.current.forEach((ticketId) => {
        const message = buildSubscribeMessage(projectId, ticketId)
        if (isDev) console.debug('[ws] subscribe:', message)
        ws.send(JSON.stringify(message))
      })

      // If no cursor data, fall back to full invalidation
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

          // Ignore ack messages
          if (message.type === 'ack') {
            if (isDev) console.debug('[ws] ack:', message.action, message.project_id, message.ticket_id)
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

      if (isDev) console.debug('[ws] disconnected, code:', e.code, 'reason:', e.reason)
      setIsConnected(false)
      wsRef.current = null

      if (heartbeatTimerRef.current) {
        clearTimeout(heartbeatTimerRef.current)
        heartbeatTimerRef.current = null
      }

      // Attempt reconnection
      if (enabled && reconnectAttemptsRef.current < MAX_RECONNECT_ATTEMPTS) {
        const delay = BASE_RECONNECT_DELAY * (reconnectAttemptsRef.current + 1)
        if (isDev) console.debug('[ws] reconnecting in', delay, 'ms (attempt', reconnectAttemptsRef.current + 1, ')')
        reconnectTimeoutRef.current = window.setTimeout(() => {
          reconnectAttemptsRef.current++
          connect()
        }, delay)
      }
    }

    ws.onerror = (err) => {
      console.error('[ws] WebSocket error:', err)
    }

    wsRef.current = ws
  }, [enabled, handleEvent, invalidateAll, resetHeartbeat, buildSubscribeMessage])

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
      const projectId = getProject()
      const message = buildSubscribeMessage(projectId, ticketId)
      if (isDev) console.debug('[ws] subscribe:', message)
      wsRef.current.send(JSON.stringify(message))
    }
  }, [buildSubscribeMessage])

  // Unsubscribe from a ticket
  const unsubscribe = useCallback((ticketId = '') => {
    subscriptionsRef.current.delete(ticketId)

    if (wsRef.current?.readyState === WebSocket.OPEN) {
      const projectId = getProject()
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

  return {
    isConnected,
    subscribe,
    unsubscribe,
  }
}
