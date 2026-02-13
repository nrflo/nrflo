import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as ticketsApi from '@/api/tickets'
import {
  sampleTicket,
  emptySessions,
  renderPage,
} from './TicketDetailPage.test-utils'
import type { TicketWithDeps } from '@/types/ticket'
import type { WorkflowResponse } from '@/types/workflow'
import type { ChainExecution } from '@/types/chain'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true }),
}))

vi.mock('@/hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    isConnected: true,
    subscribe: vi.fn(),
    unsubscribe: vi.fn(),
  }),
}))

vi.mock('@/hooks/useChains', () => ({
  useChainList: vi.fn(() => ({ data: [] })),
}))

vi.mock('@/components/workflow/PhaseTimeline', () => ({
  PhaseTimeline: () => <div data-testid="phase-timeline">PhaseTimeline</div>,
}))

vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: () => null,
}))

vi.mock('@/components/workflow/RunWorkflowDialog', () => ({
  RunWorkflowDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="run-workflow-dialog">RunWorkflowDialog</div> : null,
}))

vi.mock('@/components/workflow/RunEpicWorkflowDialog', () => ({
  RunEpicWorkflowDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="run-epic-workflow-dialog">RunEpicWorkflowDialog</div> : null,
}))

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return {
    ...actual,
    getTicket: vi.fn(),
    getWorkflow: vi.fn(),
    getAgentSessions: vi.fn(),
  }
})

const epicTicket: TicketWithDeps = {
  ...sampleTicket,
  id: 'TICKET-EPIC',
  title: 'Epic ticket',
  description: 'Epic description',
  issue_type: 'epic',
}

const taskTicket: TicketWithDeps = {
  ...sampleTicket,
  id: 'TICKET-TASK',
  title: 'Task ticket',
  issue_type: 'task',
}

const noWorkflow: WorkflowResponse = {
  ticket_id: 'TICKET-EPIC',
  has_workflow: false,
  state: {} as any,
  workflows: [],
  all_workflows: {},
}

describe('TicketDetailPage - Epic workflow integration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(noWorkflow)
  })

  describe('issue type detection', () => {
    it('passes issueType to WorkflowTabContent for epic ticket', async () => {
      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByText('Epic ticket')).toBeInTheDocument()
      })

      expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /^run workflow$/i })).not.toBeInTheDocument()
    })

    it('passes issueType to WorkflowTabContent for non-epic ticket', async () => {
      vi.mocked(ticketsApi.getTicket).mockResolvedValue(taskTicket)

      renderPage('TICKET-TASK')

      await waitFor(() => {
        expect(screen.getByText('Task ticket')).toBeInTheDocument()
      })

      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /run workflow/i })).toBeInTheDocument()
    })
  })

  describe('active chain detection', () => {
    it('queries chains with epic_ticket_id filter for epic tickets', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      const mockChainList = vi.mocked(useChainList)
      mockChainList.mockReturnValue({ data: [] } as any)

      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByText('Epic ticket')).toBeInTheDocument()
      })

      expect(mockChainList).toHaveBeenCalledWith(
        { epic_ticket_id: 'TICKET-EPIC' },
        { enabled: true }
      )
    })

    it('does not query chains for non-epic tickets', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      const mockChainList = vi.mocked(useChainList)
      mockChainList.mockReturnValue({ data: [] } as any)

      vi.mocked(ticketsApi.getTicket).mockResolvedValue(taskTicket)

      renderPage('TICKET-TASK')

      await waitFor(() => {
        expect(screen.getByText('Task ticket')).toBeInTheDocument()
      })

      expect(mockChainList).toHaveBeenCalledWith(
        { epic_ticket_id: 'TICKET-TASK' },
        { enabled: false }
      )
    })

    it('shows View Chain link when active chain exists', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      const mockChainList = vi.mocked(useChainList)
      const activeChain: ChainExecution = {
        id: 'chain-123',
        project_id: 'test-project',
        name: 'Test chain',
        status: 'running',
        workflow_name: 'feature',
        epic_ticket_id: 'TICKET-EPIC',
        created_by: 'test-user',
        total_items: 3,
        completed_items: 1,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      }
      mockChainList.mockReturnValue({ data: [activeChain] } as any)

      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByRole('link', { name: /view chain/i })).toBeInTheDocument()
      })

      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
    })

    it('shows Run Epic Workflow button when no active chain', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      vi.mocked(useChainList).mockReturnValue({ data: [] } as any)

      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
      })

      expect(screen.queryByRole('link', { name: /view chain/i })).not.toBeInTheDocument()
    })

    it('shows Run Epic Workflow button when only completed chains exist', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      const mockChainList = vi.mocked(useChainList)
      const completedChain: ChainExecution = {
        id: 'chain-123',
        project_id: 'test-project',
        name: 'Test chain',
        status: 'completed',
        workflow_name: 'feature',
        epic_ticket_id: 'TICKET-EPIC',
        created_by: 'test-user',
        total_items: 3,
        completed_items: 3,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      }
      mockChainList.mockReturnValue({ data: [completedChain] } as any)

      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
      })

      expect(screen.queryByRole('link', { name: /view chain/i })).not.toBeInTheDocument()
    })

    it('shows View Chain link for pending chain', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      const mockChainList = vi.mocked(useChainList)
      const pendingChain: ChainExecution = {
        id: 'chain-123',
        project_id: 'test-project',
        name: 'Test chain',
        status: 'pending',
        workflow_name: 'feature',
        epic_ticket_id: 'TICKET-EPIC',
        created_by: 'test-user',
        total_items: 3,
        completed_items: 0,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      }
      mockChainList.mockReturnValue({ data: [pendingChain] } as any)

      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByRole('link', { name: /view chain/i })).toBeInTheDocument()
      })

      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
    })

    it('detects active chain from multiple chains', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      const mockChainList = vi.mocked(useChainList)
      const chains: ChainExecution[] = [
        {
          id: 'chain-1',
          project_id: 'test-project',
          name: 'Completed',
          status: 'completed',
          workflow_name: 'feature',
          epic_ticket_id: 'TICKET-EPIC',
          created_by: 'test-user',
          total_items: 3,
          completed_items: 3,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
        {
          id: 'chain-2',
          project_id: 'test-project',
          name: 'Running',
          status: 'running',
          workflow_name: 'feature',
          epic_ticket_id: 'TICKET-EPIC',
          created_by: 'test-user',
          total_items: 3,
          completed_items: 1,
          created_at: '2026-01-01T01:00:00Z',
          updated_at: '2026-01-01T01:00:00Z',
        },
      ]
      mockChainList.mockReturnValue({ data: chains } as any)

      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByRole('link', { name: /view chain/i })).toBeInTheDocument()
      })

      const link = screen.getByRole('link', { name: /view chain/i })
      expect(link).toHaveAttribute('href', '/chains/chain-2')
    })
  })

  describe('dialog integration', () => {
    it('opens RunEpicWorkflowDialog when button clicked', async () => {
      const user = userEvent.setup()
      const { useChainList } = await import('@/hooks/useChains')
      vi.mocked(useChainList).mockReturnValue({ data: [] } as any)
      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /run epic workflow/i }))

      await waitFor(() => {
        expect(screen.getByTestId('run-epic-workflow-dialog')).toBeInTheDocument()
      })
    })

    it('does not open RunEpicWorkflowDialog for non-epic tickets', async () => {
      const user = userEvent.setup()
      vi.mocked(ticketsApi.getTicket).mockResolvedValue(taskTicket)

      renderPage('TICKET-TASK')

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /run workflow/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /run workflow/i }))

      await waitFor(() => {
        expect(screen.getByTestId('run-workflow-dialog')).toBeInTheDocument()
      })

      expect(screen.queryByTestId('run-epic-workflow-dialog')).not.toBeInTheDocument()
    })

    it('passes correct props to RunEpicWorkflowDialog', async () => {
      const user = userEvent.setup()
      const { useChainList } = await import('@/hooks/useChains')
      vi.mocked(useChainList).mockReturnValue({ data: [] } as any)
      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /run epic workflow/i }))

      await waitFor(() => {
        expect(screen.getByTestId('run-epic-workflow-dialog')).toBeInTheDocument()
      })
    })
  })

  describe('edge cases', () => {
    it('handles ticket with undefined issue_type', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      vi.mocked(useChainList).mockReturnValue({ data: [] } as any)
      vi.mocked(ticketsApi.getTicket).mockResolvedValue({
        ...epicTicket,
        issue_type: undefined as any,
      })

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByText('Epic ticket')).toBeInTheDocument()
      })

      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /run workflow/i })).toBeInTheDocument()
    })

    it('handles chain query returning undefined data', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      vi.mocked(useChainList).mockReturnValue({ data: undefined } as any)
      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByText('Epic ticket')).toBeInTheDocument()
      })

      expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
    })

    it('handles failed chain', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      const mockChainList = vi.mocked(useChainList)
      const failedChain: ChainExecution = {
        id: 'chain-123',
        project_id: 'test-project',
        name: 'Test chain',
        status: 'failed',
        workflow_name: 'feature',
        epic_ticket_id: 'TICKET-EPIC',
        created_by: 'test-user',
        total_items: 3,
        completed_items: 1,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      }
      mockChainList.mockReturnValue({ data: [failedChain] } as any)

      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByText('Epic ticket')).toBeInTheDocument()
      })

      expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
      expect(screen.queryByRole('link', { name: /view chain/i })).not.toBeInTheDocument()
    })

    it('handles canceled chain', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      const mockChainList = vi.mocked(useChainList)
      const canceledChain: ChainExecution = {
        id: 'chain-123',
        project_id: 'test-project',
        name: 'Test chain',
        status: 'canceled',
        workflow_name: 'feature',
        epic_ticket_id: 'TICKET-EPIC',
        created_by: 'test-user',
        total_items: 3,
        completed_items: 0,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      }
      mockChainList.mockReturnValue({ data: [canceledChain] } as any)

      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByText('Epic ticket')).toBeInTheDocument()
      })

      expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
      expect(screen.queryByRole('link', { name: /view chain/i })).not.toBeInTheDocument()
    })
  })

  describe('empty state', () => {
    it('shows Run Epic Workflow button in empty state for epic', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      vi.mocked(useChainList).mockReturnValue({ data: [] } as any)
      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByText(/no workflow configured/i)).toBeInTheDocument()
      })

      expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
    })

    it('shows View Chain link in empty state when active chain exists', async () => {
      const { useChainList } = await import('@/hooks/useChains')
      const mockChainList = vi.mocked(useChainList)
      const activeChain: ChainExecution = {
        id: 'chain-123',
        project_id: 'test-project',
        name: 'Test chain',
        status: 'running',
        workflow_name: 'feature',
        epic_ticket_id: 'TICKET-EPIC',
        created_by: 'test-user',
        total_items: 3,
        completed_items: 1,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      }
      mockChainList.mockReturnValue({ data: [activeChain] } as any)

      vi.mocked(ticketsApi.getTicket).mockResolvedValue(epicTicket)

      renderPage('TICKET-EPIC')

      await waitFor(() => {
        expect(screen.getByRole('link', { name: /view chain/i })).toBeInTheDocument()
      })

      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
    })
  })
})
