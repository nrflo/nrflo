import { useEffect } from 'react'
import { useWebSocketContext } from '@/providers/WebSocketProvider'
import { useProjectStore } from '@/stores/projectStore'

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
