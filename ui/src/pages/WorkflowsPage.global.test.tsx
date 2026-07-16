import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WorkflowsPage } from './WorkflowsPage'
import * as workflowsApi from '@/api/workflows'
import type { WorkflowDefSummary } from '@/types/workflow'

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

vi.mock('@/components/workflow/AgentDefsSection', () => ({
  AgentDefsSection: () => <div>agent-defs</div>,
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

describe('WorkflowsPage global workflows', () => {
  beforeEach(() => vi.clearAllMocks())

  it('hides global (is_global) definitions from the management list', async () => {
    const defs: Record<string, WorkflowDefSummary> = {
      feature: { description: 'Local feature', scope_type: 'project', phases: [] },
      'global-research': { description: 'Global research', scope_type: 'project', is_global: true, phases: [] },
    }
    vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue(defs)

    renderPage()

    await waitFor(() => expect(screen.getByText('feature')).toBeInTheDocument())
    // The global definition must not be editable/deletable here.
    expect(screen.queryByText('global-research')).not.toBeInTheDocument()
  })
})
