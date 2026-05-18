import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WorkflowImportDialog } from './WorkflowImportDialog'
import * as workflowsApi from '@/api/workflows'
import type { WorkflowBundle, ImportConflicts, ImportResult } from '@/api/workflows'

vi.mock('@/api/workflows', () => ({
  checkImport: vi.fn(),
  importWorkflows: vi.fn(),
}))

function renderDialog(onClose = vi.fn(), onSuccess = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return {
    ...render(
      <QueryClientProvider client={queryClient}>
        <WorkflowImportDialog open={true} onClose={onClose} onSuccess={onSuccess} />
      </QueryClientProvider>
    ),
    queryClient,
    onClose,
    onSuccess,
  }
}

function makeBundle(overrides?: Partial<WorkflowBundle>): WorkflowBundle {
  return {
    version: '1',
    exported_at: '2026-01-01T00:00:00Z',
    workflows: [],
    python_scripts: [],
    ...overrides,
  }
}

async function uploadJsonFile(content: unknown) {
  const json = typeof content === 'string' ? content : JSON.stringify(content)
  const file = new File([json], 'bundle.json', { type: 'application/json' })
  const input = document.querySelector('input[type="file"]') as HTMLInputElement
  await userEvent.upload(input, file)
}

const noConflicts: ImportConflicts = { workflow_ids: [], python_script_ids: [] }
const hasConflicts: ImportConflicts = {
  workflow_ids: ['feature'],
  python_script_ids: ['my-script'],
}
const importResult: ImportResult = { workflow_ids: ['feature'], python_script_ids: [], skipped: false }

describe('WorkflowImportDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders file select step initially', () => {
    renderDialog()
    expect(screen.getByText(/select a workflow bundle/i)).toBeInTheDocument()
    expect(document.querySelector('input[type="file"]')).toBeInTheDocument()
  })

  describe('happy path — no conflicts', () => {
    it('checks import then directly imports and calls onSuccess', async () => {
      vi.mocked(workflowsApi.checkImport).mockResolvedValue(noConflicts)
      vi.mocked(workflowsApi.importWorkflows).mockResolvedValue(importResult)
      const { onSuccess } = renderDialog()

      await uploadJsonFile(makeBundle())

      await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
      expect(workflowsApi.checkImport).toHaveBeenCalledWith(makeBundle())
      expect(workflowsApi.importWorkflows).toHaveBeenCalledWith(makeBundle(), 'overwrite')
    })
  })

  describe('conflict resolution', () => {
    async function setupConflicts() {
      vi.mocked(workflowsApi.checkImport).mockResolvedValue(hasConflicts)
      vi.mocked(workflowsApi.importWorkflows).mockResolvedValue(importResult)
      const result = renderDialog()
      await uploadJsonFile(makeBundle())
      await screen.findByRole('button', { name: /overwrite/i })
      return result
    }

    it('shows conflicting workflow and script IDs', async () => {
      await setupConflicts()
      expect(screen.getByText('feature')).toBeInTheDocument()
      expect(screen.getByText('my-script')).toBeInTheDocument()
    })

    it('calls importWorkflows with overwrite action and fires onSuccess', async () => {
      const { onSuccess } = await setupConflicts()
      await userEvent.click(screen.getByRole('button', { name: /overwrite/i }))
      await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
      expect(workflowsApi.importWorkflows).toHaveBeenCalledWith(makeBundle(), 'overwrite')
    })

    it('calls importWorkflows with rename action and fires onSuccess', async () => {
      const { onSuccess } = await setupConflicts()
      await userEvent.click(screen.getByRole('button', { name: /rename/i }))
      await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
      expect(workflowsApi.importWorkflows).toHaveBeenCalledWith(makeBundle(), 'rename')
    })

    it('closes without importing when Cancel is clicked', async () => {
      const { onClose } = await setupConflicts()
      await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
      expect(onClose).toHaveBeenCalledTimes(1)
      expect(workflowsApi.importWorkflows).not.toHaveBeenCalled()
    })
  })

  describe('parse errors', () => {
    it('shows error message for invalid JSON', async () => {
      renderDialog()
      await uploadJsonFile('not { valid json')
      // checkImport must not be called, and an error paragraph must appear
      await waitFor(() => {
        expect(workflowsApi.checkImport).not.toHaveBeenCalled()
        // parsing_error step: file input still visible, no "Checking" text
        expect(screen.queryByText(/checking for conflicts/i)).not.toBeInTheDocument()
        expect(document.querySelector('input[type="file"]')).toBeInTheDocument()
        expect(document.querySelector('.text-destructive')).not.toBeNull()
      })
    })

    it('shows error for bundle missing required version field', async () => {
      renderDialog()
      await uploadJsonFile({ workflows: [] })
      await waitFor(() =>
        expect(screen.getByText(/invalid bundle/i)).toBeInTheDocument()
      )
      expect(workflowsApi.checkImport).not.toHaveBeenCalled()
    })

    it('shows error for bundle missing workflows array', async () => {
      renderDialog()
      await uploadJsonFile({ version: '1' })
      await waitFor(() =>
        expect(screen.getByText(/invalid bundle/i)).toBeInTheDocument()
      )
    })
  })

  describe('backend errors', () => {
    it('shows error step when checkImport rejects', async () => {
      vi.mocked(workflowsApi.checkImport).mockRejectedValue(new Error('server boom'))
      const { onSuccess } = renderDialog()

      await uploadJsonFile(makeBundle())

      await waitFor(() => expect(screen.getByText('server boom')).toBeInTheDocument())
      expect(onSuccess).not.toHaveBeenCalled()
      expect(workflowsApi.importWorkflows).not.toHaveBeenCalled()
    })

    it('shows error and keeps dialog open when importWorkflows rejects', async () => {
      vi.mocked(workflowsApi.checkImport).mockResolvedValue(noConflicts)
      vi.mocked(workflowsApi.importWorkflows).mockRejectedValue(new Error('import failed'))
      const { onSuccess } = renderDialog()

      await uploadJsonFile(makeBundle())

      await waitFor(() => expect(screen.getByText('import failed')).toBeInTheDocument())
      expect(onSuccess).not.toHaveBeenCalled()
    })

    it('shows error from conflict-step importWorkflows rejection', async () => {
      vi.mocked(workflowsApi.checkImport).mockResolvedValue(hasConflicts)
      vi.mocked(workflowsApi.importWorkflows).mockRejectedValue(new Error('overwrite denied'))
      const { onSuccess } = renderDialog()

      await uploadJsonFile(makeBundle())
      await screen.findByRole('button', { name: /overwrite/i })
      await userEvent.click(screen.getByRole('button', { name: /overwrite/i }))

      await waitFor(() => expect(screen.getByText('overwrite denied')).toBeInTheDocument())
      expect(onSuccess).not.toHaveBeenCalled()
    })
  })
})
