import { useEffect } from 'react'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Layout } from '@/components/layout/Layout'
import { Dashboard } from '@/pages/Dashboard'
import { TicketListPage } from '@/pages/TicketListPage'
import { TicketDetailPage } from '@/pages/TicketDetailPage'
import { CreateTicketPage } from '@/pages/CreateTicketPage'
import { EditTicketPage } from '@/pages/EditTicketPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { WorkflowsPage } from '@/pages/WorkflowsPage'
import { ProjectWorkflowsPage } from '@/pages/ProjectWorkflowsPage'
import { ChainListPage } from '@/pages/ChainListPage'
import { ChainDetailPage } from '@/pages/ChainDetailPage'
import { useProjectStore } from '@/stores/projectStore'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30 * 1000, // 30 seconds
      refetchOnWindowFocus: true,
    },
  },
})

function App() {
  const loadProjects = useProjectStore((s) => s.loadProjects)

  useEffect(() => {
    loadProjects()
  }, [loadProjects])

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<Layout />}>
            <Route index element={<Dashboard />} />
            <Route path="tickets" element={<TicketListPage />} />
            <Route path="tickets/new" element={<CreateTicketPage />} />
            <Route path="tickets/:id" element={<TicketDetailPage />} />
            <Route path="tickets/:id/edit" element={<EditTicketPage />} />
            <Route path="workflows" element={<WorkflowsPage />} />
            <Route path="project-workflows" element={<ProjectWorkflowsPage />} />
            <Route path="chains" element={<ChainListPage />} />
            <Route path="chains/:id" element={<ChainDetailPage />} />
            <Route path="settings" element={<SettingsPage />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

export default App
