import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useConnectionsStore } from '../stores/connectionsStore'
import type { WSEventV2, WSSubscribeMessage } from './useWSProtocol'
import { subscriptionKey } from './useWSProtocol'
import { useWSSessionChannel, deniedSessionID } from './useWSSessionChannel'
import { getWebSocketUrl, invalidateAllQueries } from './useWSBootstrap'
import {
  getLastSeq,
  setLastSeq,
  persistSeqs,
  restoreSeqs,
  resetSeqs,
} from './useWSReducer'
import { computeReconnectDelay, useConnectionRecovery } from './useWSReconnect'
import type { WSEvent } from './useWebSocketTypes'
export type { WSEvent, WSEventType } from './useWebSocketTypes'
import { handleIncomingEvent } from './useWSEventHandler'

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

  const handleEvent = useCallback((event: WSEventV2) => {
    handleIncomingEvent(event, queryClientRef.current, onEventRef.current, requestResync)
  }, [requestResync]) // stable — requestResync uses refs

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
      queryClientRef.current.invalidateQueries({ queryKey: ['notification-inbox'] })

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
