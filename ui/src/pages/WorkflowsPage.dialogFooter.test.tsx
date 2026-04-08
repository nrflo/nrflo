/**
 * Tests for the DialogFooter buttons in WorkflowsPage (create + edit dialogs).
 *
 * These buttons were moved from inside WorkflowDefForm into DialogFooter
 * and now submit the form via the HTML `form` attribute.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WorkflowsPage } from './WorkflowsPage'
import * as workflowsApi from '@/api/workflows'
import type { WorkflowDefSummary } from '@/types/workflow'

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
  createWorkflowDef: vi.fn(),
  updateWorkflowDef: vi.fn(),
  deleteWorkflowDef: vi.fn(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: () => 'test-project',
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
  return render(
    <QueryClientProvider client={queryClient}>
      <WorkflowsPage />
    </QueryClientProvider>
  )
}

const featureDef: Record<string, WorkflowDefSummary> = {
  feature: {
    description: 'Feature workflow',
    phases: [{ id: 'setup-analyzer', agent: 'setup-analyzer', layer: 0 }],
  },
}

describe('WorkflowsPage – DialogFooter buttons', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('Create dialog', () => {
    it('Cancel button closes the create dialog', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue({})
      renderPage()

      await user.click(screen.getByRole('button', { name: /create workflow/i }))
      await screen.findByRole('heading', { name: /create workflow/i })

      // Cancel is the ghost button in the footer
      await user.click(screen.getByRole('button', { name: /^cancel$/i }))

      await waitFor(() => {
        expect(screen.queryByRole('heading', { name: /create workflow/i })).not.toBeInTheDocument()
      })
    })

    it('Save button submits create form via form= attribute', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue({})
      vi.mocked(workflowsApi.createWorkflowDef).mockResolvedValue({
        id: 'new-flow',
        project_id: 'test-project',
        phases: [],
        created_at: '',
        updated_at: '',
      })
      renderPage()

      await user.click(screen.getByRole('button', { name: /create workflow/i }))
      await screen.findByRole('heading', { name: /create workflow/i })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'new-flow')

      // The footer submit button is the SECOND "Create Workflow" button
      const createButtons = screen.getAllByRole('button', { name: /create workflow/i })
      await user.click(createButtons[1])

      await waitFor(() => {
        expect(workflowsApi.createWorkflowDef).toHaveBeenCalledWith(
          expect.objectContaining({ id: 'new-flow' })
        )
      })
    })

    it('shows Saving... and disables submit button while mutation is pending', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue({})
      vi.mocked(workflowsApi.createWorkflowDef).mockReturnValue(new Promise(() => {}))
      renderPage()

      await user.click(screen.getByRole('button', { name: /create workflow/i }))
      await screen.findByRole('heading', { name: /create workflow/i })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'slow-flow')

      const createButtons = screen.getAllByRole('button', { name: /create workflow/i })
      await user.click(createButtons[1])

      await screen.findByRole('button', { name: /saving\.\.\./i })
      expect(screen.getByRole('button', { name: /saving\.\.\./i })).toBeDisabled()
    })
  })

  describe('Edit dialog', () => {
    it('Cancel button closes the edit dialog', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue(featureDef)
      renderPage()

      await screen.findByTitle('Edit workflow')
      await user.click(screen.getByTitle('Edit workflow'))
      await screen.findByText('Edit Workflow: feature')

      await user.click(screen.getByRole('button', { name: /^cancel$/i }))

      await waitFor(() => {
        expect(screen.queryByText('Edit Workflow: feature')).not.toBeInTheDocument()
      })
    })

    it('Save Changes button submits edit form via form= attribute', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue(featureDef)
      vi.mocked(workflowsApi.updateWorkflowDef).mockResolvedValue({
        id: 'feature',
        project_id: 'test-project',
        phases: featureDef.feature.phases!,
        created_at: '',
        updated_at: '',
      })
      renderPage()

      await screen.findByTitle('Edit workflow')
      await user.click(screen.getByTitle('Edit workflow'))
      await screen.findByText('Edit Workflow: feature')

      const descInput = screen.getByPlaceholderText(/short description/i)
      await user.clear(descInput)
      await user.type(descInput, 'Updated description')

      await user.click(screen.getByRole('button', { name: /save changes/i }))

      await waitFor(() => {
        expect(workflowsApi.updateWorkflowDef).toHaveBeenCalledWith(
          'feature',
          expect.objectContaining({ description: 'Updated description' })
        )
      })
    })

    it('shows Saving... and disables submit button while update is pending', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue(featureDef)
      vi.mocked(workflowsApi.updateWorkflowDef).mockReturnValue(new Promise(() => {}))
      renderPage()

      await screen.findByTitle('Edit workflow')
      await user.click(screen.getByTitle('Edit workflow'))
      await screen.findByText('Edit Workflow: feature')

      await user.click(screen.getByRole('button', { name: /save changes/i }))

      await screen.findByRole('button', { name: /saving\.\.\./i })
      expect(screen.getByRole('button', { name: /saving\.\.\./i })).toBeDisabled()
    })
  })
})
