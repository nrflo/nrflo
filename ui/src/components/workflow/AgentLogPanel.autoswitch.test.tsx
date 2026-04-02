import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogPanel } from './AgentLogPanel'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

vi.mock('@/hooks/useTickets', () => ({
  useSessionMessages: () => ({ data: { messages: [], total: 0 }, isLoading: false }),
}))

vi.mock('./AgentLogDetail', () => ({
  AgentLogDetail: ({ selectedAgent, onBack }: { selectedAgent: SelectedAgentData; onBack: () => void }) => (
    <div data-testid="agent-log-detail">
      <span data-testid="detail-phase">{selectedAgent.phaseName}</span>
      <button data-testid="back-button" onClick={onBack}>Back</button>
    </div>
  ),
  formatTime: (s: string) => s,
}))

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
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

function makeSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 'session-1',
    project_id: 'test-project',
    ticket_id: 'TICKET-1',
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

function makeQC() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

function renderWithPanel(
  props: React.ComponentProps<typeof AgentLogPanel>,
  qc = makeQC()
) {
  return render(
    <QueryClientProvider client={qc}>
      <AgentLogPanel {...props} />
    </QueryClientProvider>
  )
}

describe('AgentLogPanel auto-switch (nrflow-6c78e8)', () => {
  beforeEach(() => vi.clearAllMocks())

  it('switches to next running agent when selected agent completes and runners remain', () => {
    const agentA = makeAgent({
      agent_id: 'a-selected',
      agent_type: 'implementor',
      phase: 'implementation',
      session_id: 'sess-a',
      result: undefined,
    })
    const agentB = makeAgent({
      agent_id: 'a-next',
      agent_type: 'tester',
      phase: 'verification',
      session_id: 'sess-b',
      result: undefined,
    })
    const sessionA = makeSession({ id: 'sess-a', phase: 'implementation', agent_type: 'implementor' })
    const sessionB = makeSession({ id: 'sess-b', phase: 'verification', agent_type: 'tester' })
    const onAgentSelect = vi.fn()

    const { rerender } = renderWithPanel({
      activeAgents: { 'a-selected': agentA, 'a-next': agentB },
      sessions: [sessionA, sessionB],
      collapsed: false,
      selectedAgent: { phaseName: 'implementation', agent: agentA, session: sessionA },
      onAgentSelect,
    })

    // Initially no switch — liveAgentResult is undefined
    expect(onAgentSelect).not.toHaveBeenCalled()

    // agentA completes; agentB still running
    const completedA = { ...agentA, result: 'pass' }
    rerender(
      <QueryClientProvider client={makeQC()}>
        <AgentLogPanel
          activeAgents={{ 'a-selected': completedA, 'a-next': agentB }}
          sessions={[sessionA, sessionB]}
          collapsed={false}
          selectedAgent={{ phaseName: 'implementation', agent: agentA, session: sessionA }}
          onAgentSelect={onAgentSelect}
        />
      </QueryClientProvider>
    )

    // Should return to all-running view (null) instead of selecting next agent
    expect(onAgentSelect).toHaveBeenCalledWith(null)
  })

  it('does not switch when selected agent completes but no running agents remain', () => {
    const agentA = makeAgent({
      agent_id: 'a-only',
      agent_type: 'implementor',
      phase: 'implementation',
      session_id: 'sess-a',
      result: undefined,
    })
    const sessionA = makeSession({ id: 'sess-a' })
    const onAgentSelect = vi.fn()

    const { rerender } = renderWithPanel({
      activeAgents: { 'a-only': agentA },
      sessions: [sessionA],
      collapsed: false,
      selectedAgent: { phaseName: 'implementation', agent: agentA, session: sessionA },
      onAgentSelect,
    })

    // agentA completes; no other running agents
    const completedA = { ...agentA, result: 'pass' }
    rerender(
      <QueryClientProvider client={makeQC()}>
        <AgentLogPanel
          activeAgents={{ 'a-only': completedA }}
          sessions={[sessionA]}
          collapsed={false}
          selectedAgent={{ phaseName: 'implementation', agent: agentA, session: sessionA }}
          onAgentSelect={onAgentSelect}
        />
      </QueryClientProvider>
    )

    // Should NOT auto-switch — stays on the completed agent panel
    expect(onAgentSelect).not.toHaveBeenCalled()
  })

  it('does not switch when no agent is selected', () => {
    const agentA = makeAgent({
      agent_id: 'a-running',
      session_id: 'sess-a',
      result: undefined,
    })
    const onAgentSelect = vi.fn()

    const { rerender } = renderWithPanel({
      activeAgents: { 'a-running': agentA },
      sessions: [makeSession({ id: 'sess-a' })],
      collapsed: false,
      selectedAgent: null,
      onAgentSelect,
    })

    // agentA completes; no selected agent
    const completedA = { ...agentA, result: 'pass' }
    rerender(
      <QueryClientProvider client={makeQC()}>
        <AgentLogPanel
          activeAgents={{ 'a-running': completedA }}
          sessions={[makeSession({ id: 'sess-a' })]}
          collapsed={false}
          selectedAgent={null}
          onAgentSelect={onAgentSelect}
        />
      </QueryClientProvider>
    )

    expect(onAgentSelect).not.toHaveBeenCalled()
  })
})
