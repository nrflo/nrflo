import { useEffect } from 'react'
import { Outlet } from 'react-router-dom'
import { Header } from './Header'
import { Sidebar } from './Sidebar'
import { useWebSocket } from '@/hooks/useWebSocket'
import { useProjectStore } from '@/stores/projectStore'

export function Layout() {
  const { subscribe, unsubscribe } = useWebSocket()
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  const currentProject = useProjectStore((s) => s.currentProject)

  // Project-wide WebSocket subscription for status counts, ticket lists, etc.
  useEffect(() => {
    if (projectsLoaded) {
      subscribe('')
      return () => unsubscribe('')
    }
  }, [projectsLoaded, currentProject, subscribe, unsubscribe])

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
