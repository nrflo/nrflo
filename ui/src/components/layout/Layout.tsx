import { useEffect } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { Header } from './Header'
import { Sidebar } from './Sidebar'
import { useProjectStore } from '@/stores/projectStore'

export function Layout() {
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  const projects = useProjectStore((s) => s.projects)
  const { pathname } = useLocation()
  const navigate = useNavigate()

  useEffect(() => {
    if (projectsLoaded && projects.length === 0 && !pathname.startsWith('/settings')) {
      navigate('/settings?tab=projects', { replace: true })
    }
  }, [projectsLoaded, projects.length, pathname, navigate])

  return (
    <div className="min-h-screen flex flex-col">
      <Header />
      <div className="flex flex-1">
        <Sidebar />
        <main className="flex-1 p-6 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
