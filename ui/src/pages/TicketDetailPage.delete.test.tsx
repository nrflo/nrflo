import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as ticketsApi from '@/api/tickets'
import { sampleTicket, workflowNoActivePhase, emptySessions, renderPage } from './TicketDetailPage.test-utils'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true }),
}))

vi.mock('@/hooks/useWebSocketSubscription', () => ({
  useWebSocketSubscription: () => ({ isConnected: true }),
}))

vi.mock('@/components/workflow/PhaseTimeline', () => ({
  PhaseTimeline: () => <div data-testid="phase-timeline">PhaseTimeline</div>,
}))

vi.mock('@/components/workflow/RunWorkflowDialog', () => ({
  RunWorkflowDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="run-dialog">RunWorkflowDialog</div> : null,
}))

vi.mock('@/components/workflow/RunEpicWorkflowDialog', () => ({
  RunEpicWorkflowDialog: () => null,
}))

vi.mock('@/hooks/useChains', () => ({
  useChainList: () => ({ data: [] }),
}))

vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: () => null,
}))

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return {
    ...actual,
    getTicket: vi.fn(),
    getWorkflow: vi.fn(),
    getAgentSessions: vi.fn(),
    closeTicket: vi.fn(),
    deleteTicket: vi.fn(),
  }
})

vi.mock('@/api/workflows', () => ({
  runWorkflow: vi.fn(),
  stopWorkflow: vi.fn(),
}))

describe('TicketDetailPage - delete confirmation dialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowNoActivePhase)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)
  })

  it('clicking delete button opens ConfirmDialog', async () => {
    const user = userEvent.setup()
    const { container } = renderPage()

    await waitFor(() => expect(screen.getByText('Test ticket')).toBeInTheDocument())

    // Delete button is icon-only (Trash2), identified by destructive class
    const deleteBtn = container.querySelector<HTMLButtonElement>('button.text-destructive')!
    await user.click(deleteBtn)

    expect(await screen.findByText('Delete Ticket')).toBeInTheDocument()
    expect(screen.getByText('Are you sure you want to delete this ticket?')).toBeInTheDocument()
  })

  it('clicking Cancel closes dialog without deleting', async () => {
    const user = userEvent.setup()
    const { container } = renderPage()

    await waitFor(() => expect(screen.getByText('Test ticket')).toBeInTheDocument())

    const deleteBtn = container.querySelector<HTMLButtonElement>('button.text-destructive')!
    await user.click(deleteBtn)
    await screen.findByText('Delete Ticket')

    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() => expect(screen.queryByText('Delete Ticket')).not.toBeInTheDocument())
    expect(ticketsApi.deleteTicket).not.toHaveBeenCalled()
  })

  it('clicking Delete in dialog calls deleteTicket mutation', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.deleteTicket).mockResolvedValue(undefined)
    const { container } = renderPage()

    await waitFor(() => expect(screen.getByText('Test ticket')).toBeInTheDocument())

    const deleteBtn = container.querySelector<HTMLButtonElement>('button.text-destructive')!
    await user.click(deleteBtn)
    await screen.findByText('Delete Ticket')

    const confirmDeleteBtn = screen.getByRole('button', { name: 'Delete' })
    await user.click(confirmDeleteBtn)

    await waitFor(() => {
      expect(ticketsApi.deleteTicket).toHaveBeenCalledWith('TICKET-1')
    })
  })
})
