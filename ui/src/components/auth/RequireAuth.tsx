import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'

interface RequireAuthProps {
  children?: React.ReactNode
}

export function RequireAuth({ children }: RequireAuthProps) {
  const status = useAuthStore((s) => s.status)
  const user = useAuthStore((s) => s.user)
  const location = useLocation()

  if (status === 'anon') {
    const next = encodeURIComponent(location.pathname + location.search)
    return <Navigate to={`/login?next=${next}`} replace />
  }

  if (
    user?.must_change_password &&
    location.pathname !== '/account' &&
    location.pathname !== '/login'
  ) {
    return <Navigate to="/account?force=1" replace />
  }

  return children ? <>{children}</> : <Outlet />
}
