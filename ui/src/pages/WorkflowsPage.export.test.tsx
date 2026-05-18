import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WorkflowsPage } from './WorkflowsPage'
import * as workflowsApi from '@/api/workflows'
import * as downloadBlob from '@/lib/downloadBlob'
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
  fallbackExportFilename: vi.fn(() => 'fallback.json'),
}))

vi.mock('@/hooks/usePythonScripts', () => ({
  pythonScriptKeys: { all: ['python-scripts'] },
}))

vi.mock('@/components/workflow/WorkflowImportDialog', () => ({
  WorkflowImportDialog: ({ open, onSuccess }: { open: boolean; onSuccess: () => void }) =>
    open ? (
      <div data-testid="import-dialog">
        <button onClick={onSuccess}>complete-import</button>
      </div>
    ) : null,
}))

vi.mock('@/components/workflow/AgentDefsSection', () => ({
  AgentDefsSection: () => <div data-testid="agent-defs-section" />,
}))

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return {
    ...render(
      <QueryClientProvider client={queryClient}>
        <WorkflowsPage />
      </QueryClientProvider>
    ),
    queryClient,
  }
}

const oneWorkflow: Record<string, WorkflowDefSummary> = {
  feature: {
    description: 'Feature workflow',
    phases: [{ id: 'impl', agent: 'impl', layer: 0 }],
  },
}

describe('WorkflowsPage — export/import', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue(oneWorkflow)
  })

  describe('Export All button', () => {
    it('calls exportAllWorkflows and triggerDownload with returned filename', async () => {
      const user = userEvent.setup()
      const blob = new Blob(['{}'])
      vi.mocked(workflowsApi.exportAllWorkflows).mockResolvedValue({ blob, filename: 'all.json' })
      renderPage()

      await user.click(screen.getByRole('button', { name: /export all/i }))

      await waitFor(() => {
        expect(workflowsApi.exportAllWorkflows).toHaveBeenCalledTimes(1)
        expect(downloadBlob.triggerDownload).toHaveBeenCalledWith(blob, 'all.json')
      })
    })

    it('falls back to fallbackExportFilename when filename is null', async () => {
      const user = userEvent.setup()
      const blob = new Blob(['{}'])
      vi.mocked(workflowsApi.exportAllWorkflows).mockResolvedValue({ blob, filename: null })
      renderPage()

      await user.click(screen.getByRole('button', { name: /export all/i }))

      await waitFor(() => {
        expect(downloadBlob.fallbackExportFilename).toHaveBeenCalledWith('test-project')
        expect(downloadBlob.triggerDownload).toHaveBeenCalledWith(blob, 'fallback.json')
      })
    })
  })

  describe('per-row Export button', () => {
    it('calls exportWorkflow(id) and triggerDownload with returned filename', async () => {
      const user = userEvent.setup()
      const blob = new Blob(['{}'])
      vi.mocked(workflowsApi.exportWorkflow).mockResolvedValue({ blob, filename: 'feature.json' })
      renderPage()

      await waitFor(() => expect(screen.getByText('feature')).toBeInTheDocument())
      await user.click(screen.getByTitle('Export workflow'))

      await waitFor(() => {
        expect(workflowsApi.exportWorkflow).toHaveBeenCalledWith('feature')
        expect(downloadBlob.triggerDownload).toHaveBeenCalledWith(blob, 'feature.json')
      })
    })

    it('uses hardcoded fallback filename when response filename is null', async () => {
      const user = userEvent.setup()
      const blob = new Blob(['{}'])
      vi.mocked(workflowsApi.exportWorkflow).mockResolvedValue({ blob, filename: null })
      renderPage()

      await waitFor(() => expect(screen.getByText('feature')).toBeInTheDocument())
      await user.click(screen.getByTitle('Export workflow'))

      await waitFor(() => {
        expect(downloadBlob.triggerDownload).toHaveBeenCalledWith(blob, 'nrflo-workflow-feature.json')
      })
    })
  })

  describe('Import button and dialog', () => {
    it('opens import dialog when Import button clicked', async () => {
      const user = userEvent.setup()
      renderPage()

      expect(screen.queryByTestId('import-dialog')).not.toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: /^import$/i }))
      expect(screen.getByTestId('import-dialog')).toBeInTheDocument()
    })

    it('closes import dialog after successful import', async () => {
      const user = userEvent.setup()
      renderPage()

      await user.click(screen.getByRole('button', { name: /^import$/i }))
      expect(screen.getByTestId('import-dialog')).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: 'complete-import' }))

      await waitFor(() => {
        expect(screen.queryByTestId('import-dialog')).not.toBeInTheDocument()
      })
    })

    it('invalidates workflow-defs and python-scripts on import success', async () => {
      const user = userEvent.setup()
      const { queryClient } = renderPage()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      await user.click(screen.getByRole('button', { name: /^import$/i }))
      await user.click(screen.getByRole('button', { name: 'complete-import' }))

      await waitFor(() => {
        expect(invalidateSpy).toHaveBeenCalledWith(
          expect.objectContaining({ queryKey: ['workflow-defs', 'test-project'] })
        )
        expect(invalidateSpy).toHaveBeenCalledWith(
          expect.objectContaining({ queryKey: ['python-scripts'] })
        )
      })
    })
  })
})
