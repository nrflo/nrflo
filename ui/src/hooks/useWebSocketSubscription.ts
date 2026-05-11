import { useEffect, useRef } from 'react'
import { useWebSocketContext } from '@/providers/WebSocketProvider'
import { useProjectStore } from '@/stores/projectStore'
import type { WSEvent } from '@/hooks/useWebSocket'

/**
 * Consumer hook for WebSocket subscriptions.
 * Auto-subscribes on mount, unsubscribes on unmount.
 * Use empty string or omit ticketId for project-wide subscription.
 */
export function useWebSocketSubscription(ticketId = '') {
  const { subscribe, unsubscribe, isConnected } = useWebSocketContext()
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  const currentProject = useProjectStore((s) => s.currentProject)

  useEffect(() => {
    // Skip project-wide subscription — WebSocketProvider handles it
    if (!ticketId) return
    if (!projectsLoaded) return

    subscribe(ticketId)
    return () => unsubscribe(ticketId)
  }, [ticketId, projectsLoaded, currentProject, subscribe, unsubscribe])

  return { isConnected }
}

/**
 * Register a per-component WS event handler without opening a second socket.
 * The handler is called for every event dispatched through WebSocketProvider.
 * Use this from pages/components instead of calling useWebSocket() directly.
 */
export function useWebSocketEvent(handler: (event: WSEvent) => void) {
  const { addEventListener, removeEventListener } = useWebSocketContext()
  const handlerRef = useRef(handler)
  handlerRef.current = handler

  useEffect(() => {
    const fn = (event: WSEvent) => handlerRef.current(event)
    addEventListener(fn)
    return () => removeEventListener(fn)
  }, [addEventListener, removeEventListener])
}
