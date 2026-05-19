import { useEffect } from 'react'
import { useAuthStore } from '@/stores/authStore'
import { useConnectionsStore } from '@/stores/connectionsStore'
import { set401Handler } from '@/api/client'

interface AuthGateProps {
  children: React.ReactNode
}

export function AuthGate({ children }: AuthGateProps) {
  const status = useAuthStore((s) => s.status)

  useEffect(() => {
    set401Handler((path, { isLocal, connectionId }) => {
      if (isLocal) {
        useAuthStore.getState().clear()
        if (window.location.pathname === '/login') return
        const next = encodeURIComponent(path)
        window.history.pushState({}, '', `/login?next=${next}`)
        window.dispatchEvent(new PopStateEvent('popstate'))
      } else {
        useConnectionsStore.getState().markAuthFailed(connectionId)
      }
    })
    useAuthStore.getState().refresh()
  }, [])

  if (status === 'loading') return null

  return <>{children}</>
}
