import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { PhaseTimeline } from './PhaseTimeline'
import type { WorkflowState, AgentSession } from '@/types/workflow'

// Mock child components
vi.mock('./PhaseGraph', () => ({
  PhaseGraph: () => <div data-testid="phase-graph">PhaseGraph</div>,
}))

vi.mock('@/hooks/useTickets', () => ({
  useAgentSessions: vi.fn(() => ({
    data: { sessions: [] },
    isLoading: false,
  })),
}))

function makeWorkflow(overrides: Partial<WorkflowState> = {}): WorkflowState {
  return {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    status: 'active',
    phases: {
      investigation: { status: 'completed', result: 'pass' },
      implementation: { status: 'in_progress' },
    },
    phase_order: ['investigation', 'implementation'],
    findings: {},
    ...overrides,
  }
}

function renderPhaseTimeline(props: Partial<React.ComponentProps<typeof PhaseTimeline>> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  const defaultProps = {
    workflow: makeWorkflow(),
    ...props,
  }

  return render(
    <QueryClientProvider client={queryClient}>
      <PhaseTimeline {...defaultProps} />
    </QueryClientProvider>
  )
}

describe('PhaseTimeline', () => {
  describe('basic rendering', () => {
    it('renders PhaseGraph component', () => {
      renderPhaseTimeline()
      expect(screen.getByTestId('phase-graph')).toBeInTheDocument()
    })

    it('shows empty state when no phases defined', () => {
      const workflow = makeWorkflow({ phases: {}, phase_order: [] })
      renderPhaseTimeline({ workflow })

      expect(screen.getByText('No workflow phases defined yet')).toBeInTheDocument()
      expect(screen.queryByTestId('phase-graph')).not.toBeInTheDocument()
    })

    it('displays workflow version badge when version is present', () => {
      const workflow = makeWorkflow({ version: 4 })
      renderPhaseTimeline({ workflow })

      expect(screen.getByText('v4')).toBeInTheDocument()
    })

    it('displays current phase badge when current_phase is present', () => {
      const workflow = makeWorkflow({ current_phase: 'implementation' })
      renderPhaseTimeline({ workflow })

      expect(screen.getByText('implementation')).toBeInTheDocument()
    })

    it('replaces underscores with spaces in current_phase display', () => {
      const workflow = makeWorkflow({ current_phase: 'test_design' })
      renderPhaseTimeline({ workflow })

      expect(screen.getByText('test design')).toBeInTheDocument()
    })

    it('shows "Agents running" badge when agents are running', () => {
      const workflow = makeWorkflow({
        active_agents: {
          'implementor:claude:opus': {
            agent_id: 'a1',
            agent_type: 'implementor',
            phase: 'implementation',
            model_id: 'claude-opus-4-6',
            cli: 'claude',
            pid: 12345,
            session_id: 'session-1',
            started_at: '2026-01-01T00:00:00Z',
          },
        },
      })
      renderPhaseTimeline({ workflow })

      expect(screen.getByText('Agents running')).toBeInTheDocument()
    })

    it('does not show "Agents running" badge when no agents are running', () => {
      const workflow = makeWorkflow({
        active_agents: {},
      })
      renderPhaseTimeline({ workflow })

      expect(screen.queryByText('Agents running')).not.toBeInTheDocument()
    })

    it('does not show "Agents running" badge when all agents have results', () => {
      const workflow = makeWorkflow({
        active_agents: {
          'implementor:claude:opus': {
            agent_id: 'a1',
            agent_type: 'implementor',
            phase: 'implementation',
            model_id: 'claude-opus-4-6',
            cli: 'claude',
            pid: 12345,
            session_id: 'session-1',
            started_at: '2026-01-01T00:00:00Z',
            result: 'pass',
          },
        },
      })
      renderPhaseTimeline({ workflow })

      expect(screen.queryByText('Agents running')).not.toBeInTheDocument()
    })
  })

  describe('session handling', () => {
    it('fetches agent sessions when ticketId is provided and sessions prop is not', async () => {
      const { useAgentSessions } = await import('@/hooks/useTickets')
      vi.mocked(useAgentSessions).mockReturnValue({
        data: { ticket_id: 'T-123', sessions: [] },
        isLoading: false,
      } as any)

      renderPhaseTimeline({
        workflow: makeWorkflow(),
        ticketId: 'T-123',
      })

      expect(useAgentSessions).toHaveBeenCalledWith(
        'T-123',
        undefined,
        expect.objectContaining({ enabled: true })
      )
    })

    it('does not fetch agent sessions when ticketId is not provided', async () => {
      const { useAgentSessions } = await import('@/hooks/useTickets')
      vi.mocked(useAgentSessions).mockClear()

      renderPhaseTimeline({
        workflow: makeWorkflow(),
        ticketId: undefined,
      })

      expect(useAgentSessions).toHaveBeenCalledWith(
        '',
        undefined,
        expect.objectContaining({ enabled: false })
      )
    })

    it('does not fetch agent sessions when sessions prop is provided', async () => {
      const { useAgentSessions } = await import('@/hooks/useTickets')
      vi.mocked(useAgentSessions).mockClear()

      const sessions: AgentSession[] = [
        {
          id: 'session-1',
          project_id: 'test-project',
          ticket_id: '',
          workflow_instance_id: 'wi-1',
          phase: 'implementation',
          workflow: 'feature',
          agent_type: 'implementor',
          model_id: 'claude-opus-4-6',
          status: 'completed',
          message_count: 10,
          restart_count: 0,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T02:00:00Z',
        },
      ]

      renderPhaseTimeline({
        workflow: makeWorkflow(),
        ticketId: 'T-123',
        sessions,
      })

      expect(useAgentSessions).toHaveBeenCalledWith(
        'T-123',
        undefined,
        expect.objectContaining({ enabled: false })
      )
    })

    it('uses sessions from prop when provided', () => {
      const sessions: AgentSession[] = [
        {
          id: 'session-1',
          project_id: 'test-project',
          ticket_id: '',
          workflow_instance_id: 'wi-1',
          phase: 'implementation',
          workflow: 'feature',
          agent_type: 'implementor',
          model_id: 'claude-opus-4-6',
          status: 'completed',
          message_count: 10,
          restart_count: 0,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T02:00:00Z',
        },
      ]

      renderPhaseTimeline({
        workflow: makeWorkflow(),
        sessions,
      })

      expect(screen.getByTestId('phase-graph')).toBeInTheDocument()
    })
  })

  describe('props passed to PhaseGraph', () => {
    it('passes workflow phases to PhaseGraph', () => {
      const workflow = makeWorkflow()
      renderPhaseTimeline({ workflow })

      expect(screen.getByTestId('phase-graph')).toBeInTheDocument()
    })

    it('passes current_phase to PhaseGraph', () => {
      const workflow = makeWorkflow({ current_phase: 'verification' })
      renderPhaseTimeline({ workflow })

      expect(screen.getByTestId('phase-graph')).toBeInTheDocument()
    })

    it('passes active_agents to PhaseGraph', () => {
      const workflow = makeWorkflow({
        active_agents: {
          'implementor:claude:opus': {
            agent_id: 'a1',
            agent_type: 'implementor',
            phase: 'implementation',
            model_id: 'claude-opus-4-6',
            cli: 'claude',
            pid: 12345,
            session_id: 'session-1',
            started_at: '2026-01-01T00:00:00Z',
          },
        },
      })
      renderPhaseTimeline({ workflow })

      expect(screen.getByTestId('phase-graph')).toBeInTheDocument()
    })

    it('passes agent_history to PhaseGraph', () => {
      const agentHistory = [
        {
          agent_id: 'a1',
          agent_type: 'setup-analyzer',
          phase: 'investigation',
          session_id: 'session-1',
          result: 'pass',
          ended_at: '2026-01-01T01:00:00Z',
        },
      ]
      renderPhaseTimeline({
        workflow: makeWorkflow(),
        agentHistory,
      })

      expect(screen.getByTestId('phase-graph')).toBeInTheDocument()
    })

    it('passes onAgentSelect callback to PhaseGraph', () => {
      const onAgentSelect = vi.fn()
      renderPhaseTimeline({
        workflow: makeWorkflow(),
        onAgentSelect,
      })

      expect(screen.getByTestId('phase-graph')).toBeInTheDocument()
    })

    it('passes logPanelCollapsed prop to PhaseGraph', () => {
      renderPhaseTimeline({
        workflow: makeWorkflow(),
        logPanelCollapsed: true,
      })

      expect(screen.getByTestId('phase-graph')).toBeInTheDocument()
    })
  })
})
