import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogDetail } from './AgentLogDetail'
import * as ticketsApi from '@/api/tickets'
import type { SelectedAgentData } from './PhaseGraph/types'
import type { ActiveAgentV4, WorkflowFindings } from '@/types/workflow'

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return { ...actual, getSessionMessages: vi.fn() }
})

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    pid: 1,
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeSelectedAgent(overrides: Partial<SelectedAgentData> = {}): SelectedAgentData {
  return {
    phaseName: 'implementation',
    agent: makeAgent(),
    ...overrides,
  }
}

function renderDetail(
  selectedAgent: SelectedAgentData,
  agentFindings?: WorkflowFindings,
  projectFindings?: Record<string, unknown>,
) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <AgentLogDetail
        selectedAgent={selectedAgent}
        agentFindings={agentFindings}
        projectFindings={projectFindings}
      />
    </QueryClientProvider>
  )
}

describe('AgentLogDetail - Findings tab', () => {
  beforeEach(() => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [],
      total: 0,
    })
  })

  it('renders Messages, Context, and Findings tab buttons', async () => {
    renderDetail(makeSelectedAgent())
    expect(screen.getByText('Messages')).toBeInTheDocument()
    expect(screen.getByText('Context')).toBeInTheDocument()
    expect(screen.getByText('Findings')).toBeInTheDocument()
  })

  it('Messages is the active tab by default', async () => {
    renderDetail(makeSelectedAgent())
    await waitFor(() => {
      expect(screen.getByText('No messages available')).toBeInTheDocument()
    })
    // Findings content is NOT visible (FindingsPanel not rendered yet)
    expect(screen.queryByText('No findings available')).not.toBeInTheDocument()
  })

  it('clicking Findings tab renders FindingsPanel empty state', async () => {
    const user = userEvent.setup()
    renderDetail(makeSelectedAgent())

    await user.click(screen.getByText('Findings'))

    expect(screen.getByText('No findings available')).toBeInTheDocument()
  })

  it('clicking Findings tab shows project findings', async () => {
    const user = userEvent.setup()
    renderDetail(
      makeSelectedAgent(),
      undefined,
      { deploy_url: 'https://example.com' },
    )

    await user.click(screen.getByText('Findings'))

    expect(screen.getByText('Project Findings')).toBeInTheDocument()
    expect(screen.getByText('deploy_url')).toBeInTheDocument()
  })

  it('clicking Findings tab shows agent findings filtered to selected agent', async () => {
    const user = userEvent.setup()
    renderDetail(
      makeSelectedAgent({ agent: makeAgent({ agent_type: 'implementor' }) }),
      {
        implementor: { result: 'done' },
        'qa-verifier': { tests: 10 },
      },
    )

    await user.click(screen.getByText('Findings'))

    // Only implementor findings shown (selectedAgentType = 'implementor')
    expect(screen.getByText('implementor')).toBeInTheDocument()
    expect(screen.queryByText('qa-verifier')).not.toBeInTheDocument()
    expect(screen.getByText('result')).toBeInTheDocument()
  })

  it('switching back to Messages tab hides findings content', async () => {
    const user = userEvent.setup()
    renderDetail(
      makeSelectedAgent(),
      undefined,
      { key1: 'val1' },
    )

    await user.click(screen.getByText('Findings'))
    expect(screen.getByText('Project Findings')).toBeInTheDocument()

    await user.click(screen.getByText('Messages'))
    expect(screen.queryByText('Project Findings')).not.toBeInTheDocument()
  })
})
