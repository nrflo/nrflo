import { useEffect } from 'react'
import { useAuthStore } from '@/stores/authStore'
import { set401Handler } from '@/api/client'

interface AuthGateProps {
  children: React.ReactNode
}

export function AuthGate({ children }: AuthGateProps) {
  const status = useAuthStore((s) => s.status)

  useEffect(() => {
    set401Handler(() => {
      useAuthStore.getState().clear()
      if (window.location.pathname === '/login') return
      const next = encodeURIComponent(window.location.pathname + window.location.search)
      window.history.pushState({}, '', `/login?next=${next}`)
      window.dispatchEvent(new PopStateEvent('popstate'))
    })
    useAuthStore.getState().refresh()
  }, [])

  if (status === 'loading') return null

  return <>{children}</>
}
