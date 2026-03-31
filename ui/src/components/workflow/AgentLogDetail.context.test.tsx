import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogDetail } from './AgentLogDetail'
import * as ticketsApi from '@/api/tickets'
import * as agentsApi from '@/api/agents'
import type { SelectedAgentData } from './PhaseGraph/types'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return { ...actual, getSessionMessages: vi.fn() }
})

vi.mock('@/api/agents', async () => {
  const actual = await vi.importActual('@/api/agents')
  return { ...actual, fetchSessionPrompt: vi.fn() }
})

function makeSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 'session-1',
    project_id: 'proj',
    ticket_id: 'T-1',
    workflow_instance_id: 'wi-1',
    phase: 'implementation',
    workflow: 'feature',
    agent_type: 'implementor',
    model_id: 'claude-sonnet-4-5',
    status: 'running',
    message_count: 0,
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeRunningAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    pid: 12345,
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderDetail(selectedAgent: SelectedAgentData) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentLogDetail selectedAgent={selectedAgent} />
    </QueryClientProvider>,
  )
}

describe('AgentLogDetail — Context tab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [],
      total: 0,
    })
  })

  it('renders Messages and Context top-level tabs', () => {
    renderDetail({ phaseName: 'implementation', agent: makeRunningAgent(), session: makeSession() })

    expect(screen.getByText('Messages')).toBeInTheDocument()
    expect(screen.getByText('Context')).toBeInTheDocument()
  })

  it('Messages tab is active by default — fetchSessionPrompt not called', () => {
    renderDetail({ phaseName: 'implementation', agent: makeRunningAgent(), session: makeSession() })

    expect(agentsApi.fetchSessionPrompt).not.toHaveBeenCalled()
  })

  it('clicking Context tab triggers fetchSessionPrompt with the session ID', async () => {
    vi.mocked(agentsApi.fetchSessionPrompt).mockResolvedValue({ prompt_context: '# Hello' })
    const user = userEvent.setup()

    renderDetail({ phaseName: 'implementation', agent: makeRunningAgent(), session: makeSession() })

    await user.click(screen.getByText('Context'))

    await waitFor(() => {
      expect(agentsApi.fetchSessionPrompt).toHaveBeenCalledWith('session-1')
    })
  })

  it('shows loading spinner while prompt is being fetched', async () => {
    vi.mocked(agentsApi.fetchSessionPrompt).mockReturnValue(new Promise(() => {}))
    const user = userEvent.setup()

    renderDetail({ phaseName: 'implementation', agent: makeRunningAgent(), session: makeSession() })
    await user.click(screen.getByText('Context'))

    expect(screen.getByText('Loading prompt context...')).toBeInTheDocument()
  })

  it('renders markdown when prompt_context is non-empty', async () => {
    vi.mocked(agentsApi.fetchSessionPrompt).mockResolvedValue({
      prompt_context: '# My Prompt\nInstructions go here.',
    })
    const user = userEvent.setup()

    renderDetail({ phaseName: 'implementation', agent: makeRunningAgent(), session: makeSession() })
    await user.click(screen.getByText('Context'))

    await screen.findByText('My Prompt')
    expect(screen.getByText('Instructions go here.')).toBeInTheDocument()
  })

  it('shows placeholder when prompt_context is empty', async () => {
    vi.mocked(agentsApi.fetchSessionPrompt).mockResolvedValue({ prompt_context: '' })
    const user = userEvent.setup()

    renderDetail({ phaseName: 'implementation', agent: makeRunningAgent(), session: makeSession() })
    await user.click(screen.getByText('Context'))

    await screen.findByText('No prompt context available')
  })

  it('does not fetch when no sessionId is available', async () => {
    const user = userEvent.setup()

    renderDetail({
      phaseName: 'implementation',
      agent: makeRunningAgent({ session_id: undefined }),
      session: undefined,
    })
    await user.click(screen.getByText('Context'))

    expect(agentsApi.fetchSessionPrompt).not.toHaveBeenCalled()
  })

  it('switching back to Messages tab shows messages content', async () => {
    vi.mocked(agentsApi.fetchSessionPrompt).mockResolvedValue({ prompt_context: '# Prompt' })
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [{ content: 'Running tests...', created_at: '2026-01-01T00:00:10Z' }],
      total: 1,
    })
    const user = userEvent.setup()

    renderDetail({ phaseName: 'implementation', agent: makeRunningAgent(), session: makeSession() })

    await user.click(screen.getByText('Context'))
    await screen.findByText('Prompt')

    await user.click(screen.getByText('Messages'))
    await screen.findByText('Running tests...')
  })
})
