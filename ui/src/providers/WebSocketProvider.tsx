import { createContext, useContext, useEffect, useRef, type ReactNode } from 'react'
import { useWebSocket, type WSEvent } from '@/hooks/useWebSocket'
import { useProjectStore } from '@/stores/projectStore'

interface WebSocketContextValue {
  isConnected: boolean
  subscribe: (ticketId?: string) => void
  unsubscribe: (ticketId?: string) => void
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
  const { isConnected, subscribe, unsubscribe } = useWebSocket({ onEvent })
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  const currentProject = useProjectStore((s) => s.currentProject)
  const subscribedRef = useRef(false)

  // Auto-subscribe to project-wide events
  useEffect(() => {
    if (projectsLoaded && !subscribedRef.current) {
      subscribe('')
      subscribedRef.current = true
      return () => {
        unsubscribe('')
        subscribedRef.current = false
      }
    }
  }, [projectsLoaded, currentProject, subscribe, unsubscribe])

  return (
    <WebSocketContext.Provider value={{ isConnected, subscribe, unsubscribe }}>
      {children}
    </WebSocketContext.Provider>
  )
}
