import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'

interface RequireAdminProps {
  children?: React.ReactNode
}

export function RequireAdmin({ children }: RequireAdminProps) {
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

  if (!user || user.role !== 'admin') {
    return <Navigate to="/forbidden" replace />
  }

  return children ? <>{children}</> : <Outlet />
}
