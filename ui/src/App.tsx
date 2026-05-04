import { useEffect, Suspense, lazy } from 'react'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WebSocketProvider } from '@/providers/WebSocketProvider'
import { Layout } from '@/components/layout/Layout'
import { AuthGate } from '@/components/auth/AuthGate'
import { RequireAuth } from '@/components/auth/RequireAuth'
import { RequireAdmin } from '@/components/auth/RequireAdmin'
import { UsersPage } from '@/pages/admin/UsersPage'
import { AuditLogPage } from '@/pages/admin/AuditLogPage'
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
import { GitStatusPage } from '@/pages/GitStatusPage'
import { DocumentationPage } from '@/pages/DocumentationPage'
import { ErrorsPage } from '@/pages/ErrorsPage'
import { SchedulesPage } from '@/pages/SchedulesPage'
import { ToolDefinitionsPage } from '@/pages/ToolDefinitionsPage'
import { APICredentialsPage } from '@/pages/APICredentialsPage'
import { LoginPage } from '@/pages/auth/LoginPage'
import { AccountPage } from '@/pages/auth/AccountPage'
import { ForbiddenPage } from '@/pages/ForbiddenPage'
import { useProjectStore } from '@/stores/projectStore'
import { useAuthStore } from '@/stores/authStore'
import { useAPIModeEnabled } from '@/hooks/useGlobalSettings'

const ReviewPage = lazy(() =>
  import('@/pages/nrvapp/Review').then((m) => ({ default: m.ReviewPage }))
)
const ReviewDetailPage = lazy(() =>
  import('@/pages/nrvapp/ReviewDetail').then((m) => ({ default: m.ReviewDetailPage }))
)
const ConfigPage = lazy(() =>
  import('@/pages/nrvapp/Config').then((m) => ({ default: m.ConfigPage }))
)
const ConfigEditorPage = lazy(() =>
  import('@/pages/nrvapp/ConfigEditor').then((m) => ({ default: m.ConfigEditorPage }))
)
const NrvappDashboard = lazy(() =>
  import('@/pages/nrvapp/Dashboard').then((m) => ({ default: m.NrvappDashboard }))
)

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30 * 1000, // 30 seconds
      refetchOnWindowFocus: true,
    },
  },
})

function AppRoutes() {
  const apiModeEnabled = useAPIModeEnabled()
  return (
    <BrowserRouter>
      <AuthGate>
        <Suspense fallback={<div className="p-8 text-center text-muted-foreground">Loading…</div>}>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/forbidden" element={<ForbiddenPage />} />
            <Route path="/" element={<RequireAuth><Layout /></RequireAuth>}>
              <Route index element={<Dashboard />} />
              <Route path="tickets" element={<TicketListPage />} />
              <Route path="tickets/new" element={<CreateTicketPage />} />
              <Route path="tickets/:id" element={<TicketDetailPage />} />
              <Route path="tickets/:id/edit" element={<EditTicketPage />} />
              <Route path="workflows" element={<WorkflowsPage />} />
              <Route path="project-workflows" element={<ProjectWorkflowsPage />} />
              <Route path="git-status" element={<GitStatusPage />} />
              <Route path="documentation" element={<DocumentationPage />} />
              <Route path="chains" element={<ChainListPage />} />
              <Route path="chains/:id" element={<ChainDetailPage />} />
              <Route path="schedules" element={<SchedulesPage />} />
              <Route path="errors" element={<ErrorsPage />} />
              {apiModeEnabled && <Route path="tool-definitions" element={<ToolDefinitionsPage />} />}
              {apiModeEnabled && <Route path="api-credentials" element={<APICredentialsPage />} />}
              {apiModeEnabled && <Route path="nrvapp/review" element={<ReviewPage />} />}
              {apiModeEnabled && <Route path="nrvapp/review/:id" element={<ReviewDetailPage />} />}
              {apiModeEnabled && <Route path="nrvapp/config" element={<ConfigPage />} />}
              {apiModeEnabled && <Route path="nrvapp/config/:file" element={<ConfigEditorPage />} />}
              {apiModeEnabled && <Route path="nrvapp/dashboard" element={<NrvappDashboard />} />}
              <Route path="account" element={<AccountPage />} />
              <Route path="settings" element={<RequireAdmin><SettingsPage /></RequireAdmin>} />
              <Route path="admin/users" element={<RequireAdmin><UsersPage /></RequireAdmin>} />
              <Route path="admin/audit" element={<RequireAdmin><AuditLogPage /></RequireAdmin>} />
              <Route path="*" element={<div className="p-8 text-center text-muted-foreground">Page not found.</div>} />
            </Route>
          </Routes>
        </Suspense>
      </AuthGate>
    </BrowserRouter>
  )
}

function App() {
  const authStatus = useAuthStore((s) => s.status)
  const loadProjects = useProjectStore((s) => s.loadProjects)

  useEffect(() => {
    if (authStatus === 'authed') {
      loadProjects()
    }
  }, [authStatus, loadProjects])

  return (
    <QueryClientProvider client={queryClient}>
      <WebSocketProvider>
        <AppRoutes />
      </WebSocketProvider>
    </QueryClientProvider>
  )
}

export default App
