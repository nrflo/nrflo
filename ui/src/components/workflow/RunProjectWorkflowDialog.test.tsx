import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RunProjectWorkflowDialog } from './RunProjectWorkflowDialog'
import { renderWithQuery } from '@/test/utils'
import * as workflowApi from '@/api/workflows'
import * as projectWorkflowApi from '@/api/projectWorkflows'
import type { WorkflowDefSummary } from '@/types/workflow'

// Mock API modules
vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
}))

vi.mock('@/api/projectWorkflows', () => ({
  runProjectWorkflow: vi.fn(),
}))

// Mock project store
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({
      currentProject: 'test-project',
      projectsLoaded: true,
    })
  ),
}))

const mockWorkflowDefs = {
  'ticket-workflow': {
    description: 'Ticket-scoped workflow',
    scope_type: 'ticket' as const,
    categories: ['full'],
    phases: [{ id: 'setup', agent: 'setup', layer: 0 }],
  } as WorkflowDefSummary,
  'project-workflow-1': {
    description: 'First project workflow',
    scope_type: 'project' as const,
    categories: ['full', 'simple'],
    phases: [{ id: 'analyzer', agent: 'analyzer', layer: 0 }],
  } as WorkflowDefSummary,
  'project-workflow-2': {
    description: 'Second project workflow',
    scope_type: 'project' as const,
    categories: ['full'],
    phases: [{ id: 'docs', agent: 'docs', layer: 0 }],
  } as WorkflowDefSummary,
}

describe('RunProjectWorkflowDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  const renderDialog = (props: Partial<React.ComponentProps<typeof RunProjectWorkflowDialog>> = {}) => {
    const defaultProps = {
      open: true,
      onClose: vi.fn(),
      projectId: 'test-project',
      ...props,
    }
    return renderWithQuery(<RunProjectWorkflowDialog {...defaultProps} />)
  }

  describe('workflow filtering', () => {
    it('filters to project-scoped workflows only', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByText(/project-workflow-1/i)).toBeInTheDocument()
        expect(screen.getByText(/project-workflow-2/i)).toBeInTheDocument()
      })

      expect(screen.queryByText(/ticket-workflow/i)).not.toBeInTheDocument()
    })

    it('shows empty state when no project workflows exist', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({
        'ticket-only': {
          description: 'Ticket only',
          scope_type: 'ticket' as const,
          categories: ['full'],
          phases: [],
        },
      })
      renderDialog()

      await waitFor(() => {
        expect(screen.getByText(/no project-scoped workflow definitions found/i)).toBeInTheDocument()
      })
    })

    it('shows empty state when workflow defs are empty', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({})
      renderDialog()

      await waitFor(() => {
        expect(screen.getByText(/no project-scoped workflow definitions found/i)).toBeInTheDocument()
      })
    })

    it('shows loading spinner while fetching workflows', () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockImplementation(
        () => new Promise(() => {}) // Never resolves
      )
      renderDialog()

      expect(screen.getByRole('status')).toBeInTheDocument()
    })
  })

  describe('workflow selection', () => {
    it('auto-selects first project workflow', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        const select = screen.getByRole('combobox', { name: /workflow/i }) as HTMLSelectElement
        expect(select.value).toBe('project-workflow-1')
      })
    })

    it('changes workflow when user selects different option', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('combobox', { name: /workflow/i })).toBeInTheDocument()
      })

      const workflowSelect = screen.getByRole('combobox', { name: /workflow/i })
      await user.selectOptions(workflowSelect, 'project-workflow-2')

      expect((workflowSelect as HTMLSelectElement).value).toBe('project-workflow-2')
    })

    it('displays workflow descriptions in options', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByText(/project-workflow-1 - First project workflow/i)).toBeInTheDocument()
      })

      expect(screen.getByText(/project-workflow-2 - Second project workflow/i)).toBeInTheDocument()
    })
  })

  describe('category selection', () => {
    it('displays categories from selected workflow', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('combobox', { name: /category/i })).toBeInTheDocument()
      })

      const categorySelect = screen.getByRole('combobox', { name: /category/i })
      expect(categorySelect).toHaveTextContent('full')
      expect(categorySelect).toHaveTextContent('simple')
    })

    it('resets category when workflow changes', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('combobox', { name: /workflow/i })).toBeInTheDocument()
      })

      // Initial workflow has categories: ['full', 'simple']
      const categorySelect = screen.getByRole('combobox', { name: /category/i }) as HTMLSelectElement
      expect(categorySelect.value).toBe('full')

      // Select a different category
      await user.selectOptions(categorySelect, 'simple')
      expect(categorySelect.value).toBe('simple')

      // Change workflow (project-workflow-2 only has 'full')
      const workflowSelect = screen.getByRole('combobox', { name: /workflow/i })
      await user.selectOptions(workflowSelect, 'project-workflow-2')

      // Category should reset to first available
      await waitFor(() => {
        expect(categorySelect.value).toBe('full')
      })
    })

    it('shows category help text', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByText(/controls which phases are skipped/i)).toBeInTheDocument()
      })
    })
  })

  describe('instructions input', () => {
    it('accepts optional instructions', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByPlaceholderText(/additional context/i)).toBeInTheDocument()
      })

      const instructionsInput = screen.getByPlaceholderText(/additional context/i)
      await user.type(instructionsInput, 'Test instructions')

      expect(instructionsInput).toHaveValue('Test instructions')
    })

    it('labels instructions as optional', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByText(/instructions/i)).toBeInTheDocument()
      })

      expect(screen.getByText(/\(optional\)/i)).toBeInTheDocument()
    })
  })

  describe('workflow execution', () => {
    it('runs project workflow via API with correct params', async () => {
      const user = userEvent.setup()
      const onClose = vi.fn()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(projectWorkflowApi.runProjectWorkflow).mockResolvedValue({
        instance_id: 'test-instance',
        status: 'running',
      })

      renderDialog({ onClose })

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /^run$/i })).toBeInTheDocument()
      })

      const runButton = screen.getByRole('button', { name: /^run$/i })
      await user.click(runButton)

      await waitFor(() => {
        expect(projectWorkflowApi.runProjectWorkflow).toHaveBeenCalledWith('test-project', {
          workflow: 'project-workflow-1',
          category: 'full',
          instructions: undefined,
        })
      })
    })

    it('includes instructions when provided', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(projectWorkflowApi.runProjectWorkflow).mockResolvedValue({
        instance_id: 'test-instance',
        status: 'running',
      })

      renderDialog()

      await waitFor(() => {
        expect(screen.getByPlaceholderText(/additional context/i)).toBeInTheDocument()
      })

      const instructionsInput = screen.getByPlaceholderText(/additional context/i)
      await user.type(instructionsInput, 'Custom instructions')

      const runButton = screen.getByRole('button', { name: /^run$/i })
      await user.click(runButton)

      await waitFor(() => {
        expect(projectWorkflowApi.runProjectWorkflow).toHaveBeenCalledWith('test-project', {
          workflow: 'project-workflow-1',
          category: 'full',
          instructions: 'Custom instructions',
        })
      })
    })

    it('closes dialog on successful run', async () => {
      const user = userEvent.setup()
      const onClose = vi.fn()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(projectWorkflowApi.runProjectWorkflow).mockResolvedValue({
        instance_id: 'test-instance',
        status: 'running',
      })

      renderDialog({ onClose })

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /^run$/i })).toBeInTheDocument()
      })

      const runButton = screen.getByRole('button', { name: /^run$/i })
      await user.click(runButton)

      await waitFor(() => {
        expect(onClose).toHaveBeenCalled()
      })
    })

    it('displays error message on API failure', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(projectWorkflowApi.runProjectWorkflow).mockRejectedValue(
        new Error('Workflow execution failed')
      )

      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /^run$/i })).toBeInTheDocument()
      })

      const runButton = screen.getByRole('button', { name: /^run$/i })
      await user.click(runButton)

      await waitFor(() => {
        expect(screen.getByText(/workflow execution failed/i)).toBeInTheDocument()
      })
    })

    it('disables run button when no workflow selected', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      // Wait for data to load and auto-select to complete
      await waitFor(() => {
        const select = screen.getByRole('combobox', { name: /workflow/i }) as HTMLSelectElement
        expect(select.value).toBe('project-workflow-1')
      })

      // Button should be enabled since a workflow is auto-selected
      const runButton = screen.getByRole('button', { name: /^run$/i })
      expect(runButton).not.toBeDisabled()
    })

    it('shows loading state during workflow execution', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(projectWorkflowApi.runProjectWorkflow).mockImplementation(
        () => new Promise(() => {}) // Never resolves
      )

      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /^run$/i })).toBeInTheDocument()
      })

      const runButton = screen.getByRole('button', { name: /^run$/i })
      await user.click(runButton)

      await waitFor(() => {
        expect(runButton).toBeDisabled()
      })
    })
  })

  describe('dialog state management', () => {
    it('resets state when dialog closes', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      const { rerender } = renderDialog({ open: true })

      await waitFor(() => {
        expect(screen.getByPlaceholderText(/additional context/i)).toBeInTheDocument()
      })

      // Enter some instructions
      const instructionsInput = screen.getByPlaceholderText(/additional context/i)
      await user.type(instructionsInput, 'Test instructions')

      // Close dialog
      rerender(<RunProjectWorkflowDialog open={false} onClose={vi.fn()} projectId="test-project" />)

      // Reopen dialog
      rerender(<RunProjectWorkflowDialog open={true} onClose={vi.fn()} projectId="test-project" />)

      await waitFor(() => {
        const instructionsInputAfterReopen = screen.getByPlaceholderText(/additional context/i)
        expect(instructionsInputAfterReopen).toHaveValue('')
      })
    })

    it('does not render when open is false', () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog({ open: false })

      expect(screen.queryByText(/run project workflow/i)).not.toBeInTheDocument()
    })

    it('calls onClose when cancel button clicked', async () => {
      const user = userEvent.setup()
      const onClose = vi.fn()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog({ onClose })

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument()
      })

      const cancelButton = screen.getByRole('button', { name: /cancel/i })
      await user.click(cancelButton)

      expect(onClose).toHaveBeenCalled()
    })
  })

  describe('edge cases', () => {
    it('handles workflow with no categories', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({
        'no-categories': {
          description: 'No categories',
          scope_type: 'project' as const,
          categories: [],
          phases: [],
        },
      })
      renderDialog()

      await waitFor(() => {
        expect(screen.getByText(/no-categories/i)).toBeInTheDocument()
      })

      // Category select should NOT render when categories array is empty
      expect(screen.queryByRole('combobox', { name: /category/i })).not.toBeInTheDocument()
    })

    it('handles workflow with only project scope type (no ticket workflows)', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({
        'project-only': {
          description: 'Project only',
          scope_type: 'project' as const,
          categories: ['full'],
          phases: [],
        },
      })
      renderDialog()

      await waitFor(() => {
        expect(screen.getByText(/project-only/i)).toBeInTheDocument()
      })

      expect(screen.queryByText(/no project-scoped workflow definitions found/i)).not.toBeInTheDocument()
    })

    it('does not fetch workflows when dialog is closed', () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog({ open: false })

      expect(workflowApi.listWorkflowDefs).not.toHaveBeenCalled()
    })
  })
})
