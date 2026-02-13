import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { getProject } from '../api/client'
import { ticketKeys, projectWorkflowKeys } from './useTickets'

// Event types from backend
export type WSEventType =
  | 'agent.started'
  | 'agent.completed'
  | 'phase.started'
  | 'phase.completed'
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
const isDev = import.meta.env.DEV

function getWebSocketUrl(): string {
  // Use the same host as the current page, but with ws/wss protocol
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host

  // In development with Vite proxy, we need to use the API URL
  const apiUrl = import.meta.env.VITE_API_URL
  if (apiUrl) {
    const url = new URL(apiUrl)
    return `${protocol}//${url.host}/api/v1/ws`
  }

  return `${protocol}//${host}/api/v1/ws`
}

export function useWebSocket(options: UseWebSocketOptions = {}): UseWebSocketReturn {
  const { enabled = true, onEvent } = options
  const queryClient = useQueryClient()

  const [isConnected, setIsConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectAttemptsRef = useRef(0)
  const reconnectTimeoutRef = useRef<number | null>(null)
  const subscriptionsRef = useRef<Set<string>>(new Set())
  const mountedRef = useRef(true)

  // Use refs for callbacks to avoid dependency chain issues.
  // This prevents connect() from being recreated when handlers change,
  // which would cause unnecessary WS disconnect/reconnect cycles.
  const queryClientRef = useRef(queryClient)
  queryClientRef.current = queryClient
  const onEventRef = useRef(onEvent)
  onEventRef.current = onEvent

  // Handle incoming WebSocket events (uses refs, no deps needed)
  const handleEvent = useCallback((event: WSEvent) => {
    if (isDev) {
      console.debug('[ws] event:', event.type, event.ticket_id, event.data)
    }

    // Call custom handler if provided
    onEventRef.current?.(event)

    // Invalidate relevant queries based on event type
    const qc = queryClientRef.current
    const { ticket_id, project_id } = event
    const isProjectScope = !ticket_id && !!project_id

    // Helper: invalidate project workflow queries for project-scope events
    const invalidateProjectWorkflow = () => {
      if (isProjectScope) {
        qc.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(project_id) })
      }
    }

    switch (event.type) {
      case 'agent.started':
      case 'agent.completed':
        if (isProjectScope) {
          invalidateProjectWorkflow()
          qc.invalidateQueries({ queryKey: projectWorkflowKeys.agentSessions(project_id) })
        } else {
          qc.invalidateQueries({ queryKey: ticketKeys.detail(ticket_id) })
          qc.invalidateQueries({ queryKey: ticketKeys.workflow(ticket_id) })
          qc.invalidateQueries({ queryKey: ticketKeys.agentSessions(ticket_id) })
        }
        break

      case 'phase.started':
      case 'phase.completed':
        if (isProjectScope) {
          invalidateProjectWorkflow()
        } else {
          qc.invalidateQueries({ queryKey: ticketKeys.detail(ticket_id) })
          qc.invalidateQueries({ queryKey: ticketKeys.workflow(ticket_id) })
          qc.invalidateQueries({ queryKey: ticketKeys.lists() })
        }
        break

      case 'findings.updated':
        if (isProjectScope) {
          invalidateProjectWorkflow()
        } else {
          qc.invalidateQueries({ queryKey: ticketKeys.detail(ticket_id) })
          qc.invalidateQueries({ queryKey: ticketKeys.workflow(ticket_id) })
        }
        break

      case 'messages.updated':
        if (isProjectScope) {
          invalidateProjectWorkflow()
          qc.invalidateQueries({ queryKey: projectWorkflowKeys.agentSessions(project_id) })
        } else {
          qc.invalidateQueries({ queryKey: ticketKeys.agentSessions(ticket_id) })
          qc.invalidateQueries({ queryKey: ticketKeys.workflow(ticket_id) })
        }
        // Session-specific invalidation applies to both scopes
        if (event.data?.session_id) {
          qc.invalidateQueries({ queryKey: ['session-messages', event.data.session_id] })
        }
        break

      case 'workflow.updated':
        if (isProjectScope) {
          invalidateProjectWorkflow()
        } else {
          qc.invalidateQueries({ queryKey: ticketKeys.detail(ticket_id) })
          qc.invalidateQueries({ queryKey: ticketKeys.workflow(ticket_id) })
          qc.invalidateQueries({ queryKey: ticketKeys.agentSessions(ticket_id) })
          qc.invalidateQueries({ queryKey: ticketKeys.lists() })
        }
        break

      case 'workflow_def.created':
      case 'workflow_def.updated':
      case 'workflow_def.deleted':
        qc.invalidateQueries({ queryKey: ['workflow-defs'] })
        break

      case 'agent_def.created':
      case 'agent_def.updated':
      case 'agent_def.deleted':
        qc.invalidateQueries({ queryKey: ['workflow-defs'] })
        if (event.data?.workflow_id) {
          qc.invalidateQueries({ queryKey: ['agent-defs', event.data.workflow_id] })
        }
        break

      case 'orchestration.started':
      case 'orchestration.completed':
      case 'orchestration.failed':
      case 'orchestration.retried':
      case 'orchestration.callback':
        if (isProjectScope) {
          invalidateProjectWorkflow()
        } else {
          qc.invalidateQueries({ queryKey: ticketKeys.detail(ticket_id) })
          qc.invalidateQueries({ queryKey: ticketKeys.workflow(ticket_id) })
        }
        break

      case 'ticket.updated':
        qc.invalidateQueries({ queryKey: ticketKeys.status() })
        qc.invalidateQueries({ queryKey: ticketKeys.lists() })
        qc.invalidateQueries({ queryKey: ticketKeys.detail(ticket_id) })
        break

      case 'test.echo':
        console.log('[ws] TEST BROADCAST RECEIVED:', event)
        break
    }
  }, []) // stable - uses refs internally

  // Invalidate all ticket queries (used on connect/reconnect to catch up)
  const invalidateAll = useCallback(() => {
    const qc = queryClientRef.current
    qc.invalidateQueries({ queryKey: ticketKeys.all })
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

      // Re-subscribe to all subscriptions using fresh project ID
      const projectId = getProject()
      subscriptionsRef.current.forEach((ticketId) => {
        const message = {
          action: 'subscribe',
          project_id: projectId,
          ticket_id: ticketId,
        }
        if (isDev) console.debug('[ws] subscribe:', message)
        ws.send(JSON.stringify(message))
      })

      // Invalidate all queries on connect/reconnect to catch up on any missed events
      invalidateAll()
    }

    ws.onmessage = (e) => {
      if (!mountedRef.current) return

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

          // Handle event
          handleEvent(message as WSEvent)
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
  }, [enabled, handleEvent, invalidateAll]) // handleEvent and invalidateAll are stable (empty deps)

  // Disconnect from WebSocket
  const disconnect = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current)
      reconnectTimeoutRef.current = null
    }

    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }

    setIsConnected(false)
  }, [])

  // Subscribe to a ticket (or all tickets in project if ticketId is empty)
  const subscribe = useCallback((ticketId = '') => {
    // Track only the ticketId — project is resolved fresh when sending
    subscriptionsRef.current.add(ticketId)

    // Send subscribe message if connected
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      const projectId = getProject()
      const message = {
        action: 'subscribe',
        project_id: projectId,
        ticket_id: ticketId,
      }
      if (isDev) console.debug('[ws] subscribe:', message)
      wsRef.current.send(JSON.stringify(message))
    }
  }, [])

  // Unsubscribe from a ticket
  const unsubscribe = useCallback((ticketId = '') => {
    // Remove ticketId from tracked subscriptions
    subscriptionsRef.current.delete(ticketId)

    // Send unsubscribe message if connected
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      const projectId = getProject()
      const message = {
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
      disconnect()
    }
  }, [enabled, connect, disconnect])

  return {
    isConnected,
    subscribe,
    unsubscribe,
  }
}
