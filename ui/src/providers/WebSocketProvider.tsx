import { createContext, useContext, useEffect, useRef, useMemo, type ReactNode } from 'react'
import { useWebSocket, type WSEvent } from '@/hooks/useWebSocket'
import { useProjectStore } from '@/stores/projectStore'
import { useConnectionsStore } from '@/stores/connectionsStore'

interface WebSocketContextValue {
  isConnected: boolean
  subscribe: (ticketId?: string) => void
  unsubscribe: (ticketId?: string) => void
  addEventListener: (fn: (event: WSEvent) => void) => void
  removeEventListener: (fn: (event: WSEvent) => void) => void
}

const WebSocketContext = createContext<WebSocketContextValue | null>(null)

export function useWebSocketContext(): WebSocketContextValue {
  const ctx = useContext(WebSocketContext)
  if (!ctx) throw new Error('useWebSocketContext must be used within WebSocketProvider')
  return ctx
}

interface WebSocketProviderProps {
  children: ReactNode
  onEvent?: (event: WSEvent) => void
}

/**
 * Single WebSocket connection owner per tab.
 * Wrap the app in this provider — components use useWebSocketSubscription() to subscribe.
 */
export function WebSocketProvider({ children, onEvent }: WebSocketProviderProps) {
  const listenersRef = useRef<Set<(event: WSEvent) => void>>(new Set())

  function handleEvent(event: WSEvent) {
    onEvent?.(event)
    listenersRef.current.forEach((fn) => fn(event))
  }

  const { isConnected, subscribe, unsubscribe } = useWebSocket({ onEvent: handleEvent })
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  const currentProject = useProjectStore((s) => s.currentProject)
  const activeId = useConnectionsStore((s) => s.activeId)
  const subscribedRef = useRef(false)

  // Auto-subscribe to project-wide events; re-runs when connection switches so
  // the cleanup resets subscribedRef and the body re-subscribes on the new socket.
  useEffect(() => {
    if (projectsLoaded && !subscribedRef.current) {
      subscribe('')
      subscribedRef.current = true
      return () => {
        unsubscribe('')
        subscribedRef.current = false
      }
    }
  }, [projectsLoaded, currentProject, activeId, subscribe, unsubscribe])

  const ctxValue = useMemo(
    () => ({
      isConnected,
      subscribe,
      unsubscribe,
      addEventListener: (fn: (event: WSEvent) => void) => {
        listenersRef.current.add(fn)
      },
      removeEventListener: (fn: (event: WSEvent) => void) => {
        listenersRef.current.delete(fn)
      },
    }),
    [isConnected, subscribe, unsubscribe]
  )

  return (
    <WebSocketContext.Provider value={ctxValue}>
      {children}
    </WebSocketContext.Provider>
  )
}
