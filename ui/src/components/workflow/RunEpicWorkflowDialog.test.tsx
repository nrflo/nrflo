import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RunEpicWorkflowDialog } from './RunEpicWorkflowDialog'
import { renderWithQuery } from '@/test/utils'
import * as workflowApi from '@/api/workflows'
import * as chainsApi from '@/api/chains'
import type { WorkflowDefSummary } from '@/types/workflow'
import type { ChainExecution } from '@/types/chain'

// Mock navigation
const mockNavigate = vi.fn()
vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
}))

// Mock API modules
vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
}))

vi.mock('@/api/chains', () => ({
  runEpicWorkflow: vi.fn(),
  startChain: vi.fn(),
  cancelChain: vi.fn(),
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
  'ticket-workflow-1': {
    description: 'First ticket workflow',
    scope_type: 'ticket' as const,
    phases: [{ id: 'setup', agent: 'setup', layer: 0 }],
  } as WorkflowDefSummary,
  'ticket-workflow-2': {
    description: 'Second ticket workflow',
    scope_type: 'ticket' as const,
    phases: [{ id: 'impl', agent: 'impl', layer: 0 }],
  } as WorkflowDefSummary,
  'project-workflow': {
    description: 'Project-scoped workflow',
    scope_type: 'project' as const,
    phases: [{ id: 'analyzer', agent: 'analyzer', layer: 0 }],
  } as WorkflowDefSummary,
}

const mockChain: ChainExecution = {
  id: 'chain-123',
  project_id: 'test-project',
  name: 'Epic TICKET-EPIC workflow chain',
  status: 'pending',
  workflow_name: 'ticket-workflow-1',
  epic_ticket_id: 'TICKET-EPIC',
  created_by: 'test-user',
  total_items: 3,
  completed_items: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  items: [
    {
      id: 'item-1',
      chain_id: 'chain-123',
      ticket_id: 'TICKET-1',
      ticket_title: 'First child ticket',
      position: 0,
      status: 'pending',
    },
    {
      id: 'item-2',
      chain_id: 'chain-123',
      ticket_id: 'TICKET-2',
      ticket_title: 'Second child ticket',
      position: 1,
      status: 'pending',
    },
    {
      id: 'item-3',
      chain_id: 'chain-123',
      ticket_id: 'TICKET-3',
      ticket_title: 'Third child ticket',
      position: 2,
      status: 'pending',
    },
  ],
}

describe('RunEpicWorkflowDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockNavigate.mockClear()
  })

  const renderDialog = (props: Partial<React.ComponentProps<typeof RunEpicWorkflowDialog>> = {}) => {
    const mergedProps = {
      open: true,
      onClose: vi.fn(),
      ticketId: 'TICKET-EPIC',
      ticketTitle: 'Epic Ticket Title',
      ...props,
    }
    return renderWithQuery(<RunEpicWorkflowDialog {...mergedProps} />)
  }

  describe('workflow filtering', () => {
    it('filters to ticket-scoped workflows only', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByText(/ticket-workflow-1/i)).toBeInTheDocument()
        expect(screen.getByText(/ticket-workflow-2/i)).toBeInTheDocument()
      })

      expect(screen.queryByText(/project-workflow/i)).not.toBeInTheDocument()
    })

    it('shows empty state when no ticket workflows exist', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({
        'project-only': {
          description: 'Project only',
          scope_type: 'project' as const,
                phases: [],
        },
      })
      renderDialog()

      await waitFor(() => {
        expect(screen.getByText(/no ticket-scoped workflow definitions found/i)).toBeInTheDocument()
      })
    })

    it('shows empty state when workflow defs are empty', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({})
      renderDialog()

      await waitFor(() => {
        expect(screen.getByText(/no ticket-scoped workflow definitions found/i)).toBeInTheDocument()
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
    it('auto-selects first ticket workflow', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        const select = screen.getByLabelText(/workflow/i) as HTMLSelectElement
        expect(select.value).toBe('ticket-workflow-1')
      })
    })

    it('changes workflow when user selects different option', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByLabelText(/workflow/i)).toBeInTheDocument()
      })

      const workflowSelect = screen.getByLabelText(/workflow/i)
      await user.selectOptions(workflowSelect, 'ticket-workflow-2')

      expect((workflowSelect as HTMLSelectElement).value).toBe('ticket-workflow-2')
    })

    it('displays workflow descriptions in options', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByText(/ticket-workflow-1 - First ticket workflow/i)).toBeInTheDocument()
      })

      expect(screen.getByText(/ticket-workflow-2 - Second ticket workflow/i)).toBeInTheDocument()
    })
  })

  describe('chain preview', () => {
    it('creates chain preview when Preview Chain button clicked', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue(mockChain)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      const previewButton = screen.getByRole('button', { name: /preview chain/i })
      await user.click(previewButton)

      await waitFor(() => {
        expect(chainsApi.runEpicWorkflow).toHaveBeenCalledWith('TICKET-EPIC', {
          workflow_name: 'ticket-workflow-1',
                  start: false,
        })
      })
    })

    it('displays ordered child tickets after preview', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue(mockChain)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByText('TICKET-1')).toBeInTheDocument()
        expect(screen.getByText('First child ticket')).toBeInTheDocument()
      })

      expect(screen.getByText('TICKET-2')).toBeInTheDocument()
      expect(screen.getByText('Second child ticket')).toBeInTheDocument()
      expect(screen.getByText('TICKET-3')).toBeInTheDocument()
      expect(screen.getByText('Third child ticket')).toBeInTheDocument()
    })

    it('shows workflow badge after preview', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue(mockChain)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByText('ticket-workflow-1')).toBeInTheDocument()
      })
    })

    it('shows ticket count after preview', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue(mockChain)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByText(/3 tickets in chain/i)).toBeInTheDocument()
      })
    })

    it('displays preview error when API fails', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockRejectedValue(
        new Error('Epic has no children')
      )
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByText(/epic has no children/i)).toBeInTheDocument()
      })
    })

    it('shows loading state during preview', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockImplementation(
        () => new Promise(() => {}) // Never resolves
      )
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        const button = screen.getByRole('button', { name: /preview chain/i })
        expect(button).toBeDisabled()
      })
    })

    it('disables preview button when no workflow selected', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({})
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      const previewButton = screen.getByRole('button', { name: /preview chain/i })
      expect(previewButton).toBeDisabled()
    })
  })

  describe('chain execution', () => {
    it('starts chain and navigates when Run Now clicked', async () => {
      const user = userEvent.setup()
      const onClose = vi.fn()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue(mockChain)
      vi.mocked(chainsApi.startChain).mockResolvedValue({ status: 'running', chain_id: 'chain-123' })
      renderDialog({ onClose })

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      // Preview first
      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /run now/i })).toBeInTheDocument()
      })

      // Run now
      await user.click(screen.getByRole('button', { name: /run now/i }))

      await waitFor(() => {
        expect(chainsApi.startChain).toHaveBeenCalledWith('chain-123')
        expect(mockNavigate).toHaveBeenCalledWith('/chains/chain-123')
        expect(onClose).toHaveBeenCalled()
      })
    })

    it('displays error when start chain fails', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue(mockChain)
      vi.mocked(chainsApi.startChain).mockRejectedValue(
        new Error('Lock conflict: TICKET-1 is already in use')
      )
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /run now/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /run now/i }))

      await waitFor(() => {
        expect(screen.getByText(/lock conflict.*ticket-1.*already in use/i)).toBeInTheDocument()
      })
    })

    it('shows loading state during start chain', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue(mockChain)
      vi.mocked(chainsApi.startChain).mockImplementation(
        () => new Promise(() => {}) // Never resolves
      )
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /run now/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /run now/i }))

      await waitFor(() => {
        const button = screen.getByRole('button', { name: /run now/i })
        expect(button).toBeDisabled()
      })
    })

    it('disables Run Now button when no pending chain', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      // Run Now should not be visible before preview
      expect(screen.queryByRole('button', { name: /run now/i })).not.toBeInTheDocument()
    })
  })

  describe('cancel behavior', () => {
    it('calls cancelChain when dialog closed with pending chain', async () => {
      const user = userEvent.setup()
      const onClose = vi.fn()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue(mockChain)
      vi.mocked(chainsApi.cancelChain).mockResolvedValue({ status: 'canceled', chain_id: 'chain-123' })
      renderDialog({ onClose })

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      // Create preview
      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /run now/i })).toBeInTheDocument()
      })

      // Cancel dialog
      await user.click(screen.getByRole('button', { name: /cancel/i }))

      await waitFor(() => {
        expect(chainsApi.cancelChain).toHaveBeenCalledWith('chain-123')
        expect(onClose).toHaveBeenCalled()
      })
    })

    it('does not call cancelChain when closed without pending chain', async () => {
      const user = userEvent.setup()
      const onClose = vi.fn()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.cancelChain).mockResolvedValue({ status: 'canceled', chain_id: 'chain-123' })
      renderDialog({ onClose })

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      // Cancel without creating preview
      await user.click(screen.getByRole('button', { name: /cancel/i }))

      await waitFor(() => {
        expect(chainsApi.cancelChain).not.toHaveBeenCalled()
        expect(onClose).toHaveBeenCalled()
      })
    })

    it('closes dialog even if cancelChain fails (best-effort cleanup)', async () => {
      const user = userEvent.setup()
      const onClose = vi.fn()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue(mockChain)
      vi.mocked(chainsApi.cancelChain).mockRejectedValue(new Error('Cancel failed'))
      renderDialog({ onClose })

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /run now/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /cancel/i }))

      await waitFor(() => {
        expect(onClose).toHaveBeenCalled()
      })
    })

    it('disables cancel button during cancel operation', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue(mockChain)
      let resolveCancel: (() => void) | undefined
      vi.mocked(chainsApi.cancelChain).mockImplementation(
        () => new Promise((resolve) => { resolveCancel = () => resolve({ status: 'canceled', chain_id: 'chain-123' }) })
      )
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /run now/i })).toBeInTheDocument()
      })

      const clickPromise = user.click(screen.getByRole('button', { name: /cancel/i }))

      // Cancel button should be disabled immediately
      await waitFor(() => {
        expect(screen.getByRole('button', { name: /cancel/i })).toBeDisabled()
      })

      // Clean up
      resolveCancel?.()
      await clickPromise
    })
  })

  describe('dialog state management', () => {
    it('resets state when dialog closes', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue(mockChain)
      const { rerender } = renderDialog({ open: true })

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      // Create preview
      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /run now/i })).toBeInTheDocument()
      })

      // Close dialog
      rerender(
        <RunEpicWorkflowDialog
          open={false}
          onClose={vi.fn()}
          ticketId="TICKET-EPIC"
          ticketTitle="Epic Ticket Title"
        />
      )

      // Reopen dialog
      rerender(
        <RunEpicWorkflowDialog
          open={true}
          onClose={vi.fn()}
          ticketId="TICKET-EPIC"
          ticketTitle="Epic Ticket Title"
        />
      )

      await waitFor(() => {
        // Should be back to initial state (preview button visible)
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
        expect(screen.queryByRole('button', { name: /run now/i })).not.toBeInTheDocument()
      })
    })

    it('does not render when open is false', () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog({ open: false })

      expect(screen.queryByText(/run epic workflow/i)).not.toBeInTheDocument()
    })

    it('displays epic ticket title in header', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog({ ticketTitle: 'My Epic Feature' })

      await waitFor(() => {
        expect(screen.getByText(/run epic workflow/i)).toBeInTheDocument()
      })

      expect(screen.getByText('My Epic Feature')).toBeInTheDocument()
    })

    it('does not fetch workflows when dialog is closed', () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      renderDialog({ open: false })

      expect(workflowApi.listWorkflowDefs).not.toHaveBeenCalled()
    })
  })

  describe('edge cases', () => {
    it('handles chain with no items', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue({
        ...mockChain,
        items: [],
        total_items: 0,
      })
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByText(/0 tickets in chain/i)).toBeInTheDocument()
      })
    })

    it('handles chain with single item', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue({
        ...mockChain,
        items: [mockChain.items![0]],
        total_items: 1,
      })
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByText(/1 ticket in chain/i)).toBeInTheDocument()
      })
    })

    it('handles chain items without ticket titles', async () => {
      const user = userEvent.setup()
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue(mockWorkflowDefs)
      vi.mocked(chainsApi.runEpicWorkflow).mockResolvedValue({
        ...mockChain,
        items: [
          {
            id: 'item-1',
            chain_id: 'chain-123',
            ticket_id: 'TICKET-1',
            position: 0,
            status: 'pending',
          },
        ],
      })
      renderDialog()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /preview chain/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /preview chain/i }))

      await waitFor(() => {
        expect(screen.getByText('TICKET-1')).toBeInTheDocument()
      })
    })

  })
})
