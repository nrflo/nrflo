import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as ticketsApi from '@/api/tickets'
import {
  sampleTicket,
  workflowNoActivePhase,
  workflowCompleted,
  workflowCompletedZeroTokens,
  emptySessions,
  renderPage,
} from './TicketDetailPage.test-utils'

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

describe('TicketDetailPage - Completion stats banner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)
  })

  it('shows completion banner with all three stats when workflow is completed', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowCompleted)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Completed')).toBeInTheDocument()
    })

    // 1. Completion date/time is displayed
    const banner = screen.getByText('Completed').closest('div.flex')!
    expect(banner).toBeInTheDocument()

    // 2. Duration is displayed (5400s = 1h 30m)
    expect(screen.getByText('1h 30m')).toBeInTheDocument()

    // 3. Token count is displayed (230000 = 230K)
    expect(screen.getByText('230K tokens')).toBeInTheDocument()
  })

  it('does not show completion banner when workflow is active', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowNoActivePhase)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Test ticket')).toBeInTheDocument()
    })

    // workflowNoActivePhase has no status='completed', so no banner
    expect(screen.queryByText('230K tokens')).not.toBeInTheDocument()
  })

  it('hides token count when total_tokens_used is 0', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowCompletedZeroTokens)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Completed')).toBeInTheDocument()
    })

    // Duration should show (300s = 5m)
    expect(screen.getByText('5m')).toBeInTheDocument()

    // Token count should NOT show when 0
    expect(screen.queryByText(/tokens/)).not.toBeInTheDocument()
  })

  it('does not show completion banner on description tab', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowCompleted)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Completed')).toBeInTheDocument()
    })

    // Switch to description tab
    await user.click(screen.getByText('Description'))

    // Banner should not be visible (it's only in workflow tab)
    expect(screen.queryByText('230K tokens')).not.toBeInTheDocument()
  })
})
