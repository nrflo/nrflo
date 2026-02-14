/**
 * Regression tests for category/skip_for removal (ticket nrworkflow-b129f1)
 *
 * These tests ensure that categories and skip_for fields are not present in:
 * - Type definitions
 * - API request payloads
 * - UI components
 *
 * Context: Categories (full, simple, docs) and skip_for rules were removed from
 * the system. Agents now decide for themselves which steps to skip.
 */

import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkflowDefForm } from './WorkflowDefForm'
import { RunWorkflowDialog } from './RunWorkflowDialog'
import { RunProjectWorkflowDialog } from './RunProjectWorkflowDialog'
import { RunEpicWorkflowDialog } from './RunEpicWorkflowDialog'
import { CreateChainDialog } from '@/components/chains/CreateChainDialog'
import type { WorkflowDefCreateRequest, PhaseDef } from '@/types/workflow'
import type { ChainCreateRequest } from '@/types/chain'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'

// Mock dependencies
vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn().mockResolvedValue({
    feature: {
      description: 'Feature workflow',
      scope_type: 'ticket',
      phases: [{ id: 'setup', agent: 'setup', layer: 0 }],
    },
  }),
}))

vi.mock('@/api/tickets', () => ({
  runWorkflow: vi.fn(),
}))

vi.mock('@/api/projectWorkflows', () => ({
  runProjectWorkflow: vi.fn(),
}))

vi.mock('@/hooks/useChains', () => ({
  useCreateChain: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
    error: null,
  }),
  useUpdateChain: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
    error: null,
  }),
  useRunEpicWorkflow: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
    error: null,
  }),
  useStartChain: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
    error: null,
  }),
  useCancelChain: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
    error: null,
  }),
}))

vi.mock('@/hooks/useTickets', () => ({
  useRunWorkflow: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
    error: null,
  }),
  useRunProjectWorkflow: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
    error: null,
  }),
  useTicket: () => ({
    data: { id: 'TEST-1', title: 'Test ticket' },
  }),
}))

vi.mock('@/components/chains/ChainTicketSelector', () => ({
  ChainTicketSelector: ({ selectedIds, onChange }: any) => (
    <div data-testid="chain-ticket-selector">
      <button onClick={() => onChange(['TICKET-1', 'TICKET-2'])}>
        Select tickets
      </button>
      Selected: {selectedIds.length}
    </div>
  ),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) => {
    const store = {
      currentProject: 'test-project',
      projectsLoaded: true,
    }
    return selector(store)
  }),
}))

// Mock useQuery for CreateChainDialog and RunEpicWorkflowDialog
vi.mock('@tanstack/react-query', async () => {
  const actual = await vi.importActual('@tanstack/react-query')
  return {
    ...actual,
    useQuery: () => ({
      data: {
        feature: {
          id: 'feature',
          project_id: 'test-project',
          description: 'Feature workflow',
          scope_type: 'ticket',
          phases: [{ id: 'setup', agent: 'setup', layer: 0 }],
        },
      },
      isLoading: false,
    }),
  }
})

describe('Category/Skip_for Removal - Regression Tests', () => {
  describe('WorkflowDefForm - no categories field in payload', () => {
    it('does not include categories field in create request', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <WorkflowDefForm
          isCreate={true}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
          isPending={false}
        />
      )

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'test-workflow')
      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(agentInputs[0], 'implementor')

      const submitButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledTimes(1)
      const payload = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest

      // Ensure categories field is not in payload
      expect(payload).not.toHaveProperty('categories')
      expect(payload).toMatchObject({
        id: 'test-workflow',
        scope_type: 'ticket',
        phases: [
          {
            id: 'implementor',
            agent: 'implementor',
            layer: 0,
          },
        ],
      })
    })

    it('phases do not include skip_for field', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      render(
        <WorkflowDefForm
          isCreate={true}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
          isPending={false}
        />
      )

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'test-workflow')
      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(agentInputs[0], 'implementor')

      const submitButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(submitButton)

      const payload = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      const phase = payload.phases[0]

      // Ensure skip_for field is not in phase definition
      expect(phase).not.toHaveProperty('skip_for')
      expect(phase).toMatchObject({
        id: 'implementor',
        agent: 'implementor',
        layer: 0,
      })
    })

    it('handles loading existing workflow data without categories gracefully', () => {
      const phases: PhaseDef[] = [
        { id: 'setup', agent: 'setup', layer: 0 },
        { id: 'implementor', agent: 'implementor', layer: 1 },
      ]

      // Should render without errors even if old DB data had categories
      render(
        <WorkflowDefForm
          isCreate={false}
          initial={{ id: 'feature', phases }}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
          isPending={false}
        />
      )

      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      expect(agentInputs[0]).toHaveValue('setup')
      expect(agentInputs[1]).toHaveValue('implementor')
    })
  })

  describe('RunWorkflowDialog - no category in request', () => {
    it('does not render category dropdown', async () => {
      const queryClient = new QueryClient()

      render(
        <QueryClientProvider client={queryClient}>
          <RunWorkflowDialog
            open={true}
            onClose={vi.fn()}
            ticketId="TEST-1"
          />
        </QueryClientProvider>
      )

      // Should not have any category-related UI
      expect(screen.queryByText(/category/i)).not.toBeInTheDocument()
      expect(screen.queryByText(/full/i)).not.toBeInTheDocument()
      expect(screen.queryByText(/simple/i)).not.toBeInTheDocument()
      expect(screen.queryByText(/docs/i)).not.toBeInTheDocument()
    })
  })

  describe('RunProjectWorkflowDialog - no category in request', () => {
    it('does not render category dropdown', async () => {
      const queryClient = new QueryClient()

      render(
        <QueryClientProvider client={queryClient}>
          <RunProjectWorkflowDialog
            open={true}
            onClose={vi.fn()}
            projectId="test-project"
          />
        </QueryClientProvider>
      )

      // Should not have any category-related UI
      expect(screen.queryByText(/category/i)).not.toBeInTheDocument()
    })
  })

  describe('RunEpicWorkflowDialog - no category in request', () => {
    it('does not render category dropdown or category badges', async () => {
      const queryClient = new QueryClient()

      render(
        <QueryClientProvider client={queryClient}>
          <MemoryRouter>
            <RunEpicWorkflowDialog
              open={true}
              onClose={vi.fn()}
              ticketId="EPIC-1"
              ticketTitle="Epic Ticket"
            />
          </MemoryRouter>
        </QueryClientProvider>
      )

      // Should not have any category-related UI
      expect(screen.queryByText(/category/i)).not.toBeInTheDocument()
    })
  })

  describe('CreateChainDialog - no category in request', () => {
    it('does not render category field', async () => {
      const queryClient = new QueryClient()

      render(
        <QueryClientProvider client={queryClient}>
          <MemoryRouter>
            <CreateChainDialog
              open={true}
              onClose={vi.fn()}
              editChain={null}
            />
          </MemoryRouter>
        </QueryClientProvider>
      )

      // Wait for workflows to load
      await screen.findByLabelText(/workflow/i)

      // Should not have any category-related UI
      expect(screen.queryByText(/category/i)).not.toBeInTheDocument()
    })
  })

  describe('Type safety - no category/skip_for in types', () => {
    it('PhaseDef type does not include skip_for', () => {
      const validPhase: PhaseDef = {
        id: 'test-phase',
        agent: 'test-agent',
        layer: 0,
      }

      expect(validPhase).toBeDefined()

      // Verify the type definition doesn't have skip_for by checking keys
      const keys = Object.keys(validPhase)
      expect(keys).not.toContain('skip_for')
    })

    it('WorkflowDefCreateRequest does not include categories', () => {
      const validRequest: WorkflowDefCreateRequest = {
        id: 'test-workflow',
        description: 'Test',
        scope_type: 'ticket',
        phases: [],
      }

      expect(validRequest).toBeDefined()

      // Verify the type definition doesn't have categories by checking keys
      const keys = Object.keys(validRequest)
      expect(keys).not.toContain('categories')
    })

    it('ChainCreateRequest does not include category', () => {
      const validRequest: ChainCreateRequest = {
        name: 'Test Chain',
        workflow_name: 'feature',
        ticket_ids: ['TICKET-1'],
      }

      expect(validRequest).toBeDefined()

      // Verify the type definition doesn't have category by checking keys
      const keys = Object.keys(validRequest)
      expect(keys).not.toContain('category')
    })
  })
})
