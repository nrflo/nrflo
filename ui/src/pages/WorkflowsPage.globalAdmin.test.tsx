import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WorkflowsPage } from './WorkflowsPage'
import * as workflowsApi from '@/api/workflows'
import * as agentDefsApi from '@/api/agentDefs'
import type { WorkflowDefSummary } from '@/types/workflow'

const mockUseIsAdmin = vi.fn().mockReturnValue(false)
vi.mock('@/stores/authStore', () => ({
  useIsAdmin: () => mockUseIsAdmin(),
}))

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
  createWorkflowDef: vi.fn(),
  updateWorkflowDef: vi.fn(),
  deleteWorkflowDef: vi.fn(),
  exportWorkflow: vi.fn(),
  exportAllWorkflows: vi.fn(),
  checkImport: vi.fn(),
  importWorkflows: vi.fn(),
}))

vi.mock('@/api/agentDefs', () => ({
  listAgentDefs: vi.fn().mockResolvedValue([]),
  createAgentDef: vi.fn(),
  updateAgentDef: vi.fn(),
  deleteAgentDef: vi.fn(),
  getAgentDef: vi.fn(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: () => 'test-project',
}))

vi.mock('@/lib/downloadBlob', () => ({
  triggerDownload: vi.fn(),
  fallbackExportFilename: vi.fn(() => 'x.json'),
}))

vi.mock('@/hooks/usePythonScripts', () => ({
  pythonScriptKeys: { all: ['python-scripts'] },
}))

vi.mock('@/components/workflow/WorkflowImportDialog', () => ({
  WorkflowImportDialog: () => null,
}))

vi.mock('@/components/workflows/WorkflowNotificationsSection', () => ({
  WorkflowNotificationsSection: () => <div>notifications</div>,
}))

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <WorkflowsPage />
    </QueryClientProvider>
  )
}

describe('WorkflowsPage global workflows — admin visibility', () => {
  const defs: Record<string, WorkflowDefSummary> = {
    feature: { description: 'Local feature', scope_type: 'project', phases: [] },
    'global-research': { description: 'Global research', scope_type: 'project', is_global: true, phases: [] },
  }

  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue(defs)
  })

  it('non-admin: global definitions stay hidden', async () => {
    mockUseIsAdmin.mockReturnValue(false)
    renderPage()

    await waitFor(() => expect(screen.getByText('feature')).toBeInTheDocument())
    expect(screen.queryByText('global-research')).not.toBeInTheDocument()
  })

  it('admin: global definitions render with a Global badge, and its agent-def CRUD is scoped to __global__', async () => {
    mockUseIsAdmin.mockReturnValue(true)
    renderPage()

    await waitFor(() => expect(screen.getByText('global-research')).toBeInTheDocument())
    expect(screen.getByText('Global')).toBeInTheDocument()

    screen.getByText('global-research').click()
    await waitFor(() =>
      expect(agentDefsApi.listAgentDefs).toHaveBeenCalledWith('global-research', '__global__')
    )
  })

  it('admin: the global card hides workflow-level Export/Edit/Delete controls (only the local card keeps them)', async () => {
    mockUseIsAdmin.mockReturnValue(true)
    renderPage()

    await waitFor(() => expect(screen.getByText('global-research')).toBeInTheDocument())
    // Only the local 'feature' card exposes these — the global card must not add its own.
    expect(screen.getAllByTitle('Export workflow')).toHaveLength(1)
    expect(screen.getAllByTitle('Edit workflow')).toHaveLength(1)
    expect(screen.getAllByTitle('Delete workflow')).toHaveLength(1)
  })
})
