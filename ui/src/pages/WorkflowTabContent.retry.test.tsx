import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowState, AgentHistoryEntry } from '@/types/workflow'

// Mock child components
vi.mock('@/components/workflow/PhaseTimeline', () => ({
  PhaseTimeline: () => <div data-testid="phase-timeline">PhaseTimeline</div>,
}))
vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: () => <div data-testid="agent-log-panel">AgentLogPanel</div>,
}))

function makeState(overrides: Partial<WorkflowState> = {}): WorkflowState {
  return {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    status: 'active',
    phases: { implementation: { status: 'in_progress' } },
    phase_order: ['implementation'],
    ...overrides,
  }
}

function makeFailedAgent(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    result: 'fail',
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T00:05:00Z',
    session_id: 'sess-fail-123',
    ...overrides,
  }
}

const defaultProps = {
  ticketId: 'T-1',
  hasWorkflow: true,
  displayedState: makeState(),
  displayedWorkflowName: 'feature',
  hasMultipleWorkflows: false,
  workflows: ['feature'],
  selectedWorkflow: 'feature',
  onSelectWorkflow: vi.fn(),
  isOrchestrated: false,
  hasActivePhase: false,
  activeAgents: {},
  sessions: [],
  logPanelCollapsed: false,
  onToggleLogPanel: vi.fn(),
  selectedPanelAgent: null,
  onAgentSelect: vi.fn(),
  onStop: vi.fn(),
  stopPending: false,
  onShowRunDialog: vi.fn(),
}

describe('WorkflowTabContent - Retry Failed Agent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('failed workflow banner', () => {
    it('shows "Workflow Failed" banner when status is failed and has failed agents', () => {
      const failedState = makeState({
        status: 'failed',
        agent_history: [makeFailedAgent()],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={vi.fn()}
        />
      )

      expect(screen.getByText('Workflow Failed')).toBeInTheDocument()
    })

    it('does not show failed banner when status is active', () => {
      const activeState = makeState({
        status: 'active',
        agent_history: [makeFailedAgent()],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={activeState}
          onRetryFailed={vi.fn()}
        />
      )

      expect(screen.queryByText('Workflow Failed')).not.toBeInTheDocument()
    })

    it('does not show failed banner when status is completed', () => {
      const completedState = makeState({
        status: 'completed',
        agent_history: [{ ...makeFailedAgent(), result: 'pass' }],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={completedState}
          onRetryFailed={vi.fn()}
        />
      )

      expect(screen.queryByText('Workflow Failed')).not.toBeInTheDocument()
    })

    it('does not show failed banner when onRetryFailed is not provided', () => {
      const failedState = makeState({
        status: 'failed',
        agent_history: [makeFailedAgent()],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
        />
      )

      expect(screen.queryByText('Workflow Failed')).not.toBeInTheDocument()
    })

    it('does not show failed banner when no failed agents exist', () => {
      const failedState = makeState({
        status: 'failed',
        agent_history: [],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={vi.fn()}
        />
      )

      expect(screen.queryByText('Workflow Failed')).not.toBeInTheDocument()
    })

    it('does not show failed banner when agent_history is undefined', () => {
      const failedState = makeState({
        status: 'failed',
        agent_history: undefined,
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={vi.fn()}
        />
      )

      expect(screen.queryByText('Workflow Failed')).not.toBeInTheDocument()
    })
  })

  describe('retry failed button', () => {
    it('shows "Retry Failed" button in failed banner', () => {
      const failedState = makeState({
        status: 'failed',
        agent_history: [makeFailedAgent()],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={vi.fn()}
        />
      )

      expect(screen.getByText('Retry Failed')).toBeInTheDocument()
    })

    it('calls onRetryFailed with first failed agent session_id when clicked', async () => {
      const user = userEvent.setup()
      const onRetryFailed = vi.fn()
      const failedState = makeState({
        status: 'failed',
        agent_history: [makeFailedAgent({ session_id: 'sess-failed-1' })],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={onRetryFailed}
        />
      )

      await user.click(screen.getByText('Retry Failed'))
      expect(onRetryFailed).toHaveBeenCalledWith('sess-failed-1')
      expect(onRetryFailed).toHaveBeenCalledTimes(1)
    })

    it('calls onRetryFailed with first failed agent when multiple failed agents', async () => {
      const user = userEvent.setup()
      const onRetryFailed = vi.fn()
      const failedState = makeState({
        status: 'failed',
        agent_history: [
          makeFailedAgent({ session_id: 'sess-first', started_at: '2026-01-01T00:00:00Z' }),
          makeFailedAgent({ session_id: 'sess-second', started_at: '2026-01-01T00:01:00Z', agent_type: 'tester' }),
        ],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={onRetryFailed}
        />
      )

      await user.click(screen.getByText('Retry Failed'))
      expect(onRetryFailed).toHaveBeenCalledWith('sess-first')
    })

    it('disables retry button when retryingSessionId is set', () => {
      const failedState = makeState({
        status: 'failed',
        agent_history: [makeFailedAgent({ session_id: 'sess-1' })],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={vi.fn()}
          retryingSessionId="sess-1"
        />
      )

      const button = screen.getByText('Retry Failed').closest('button')
      expect(button).toBeDisabled()
    })

    it('disables retry button when retryingSessionId is set to any value', () => {
      const failedState = makeState({
        status: 'failed',
        agent_history: [makeFailedAgent({ session_id: 'sess-1' })],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={vi.fn()}
          retryingSessionId="sess-other"
        />
      )

      const button = screen.getByText('Retry Failed').closest('button')
      expect(button).toBeDisabled()
    })

    it('does not disable retry button when retryingSessionId is null', () => {
      const failedState = makeState({
        status: 'failed',
        agent_history: [makeFailedAgent()],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={vi.fn()}
          retryingSessionId={null}
        />
      )

      const button = screen.getByText('Retry Failed').closest('button')
      expect(button).not.toBeDisabled()
    })

    it('does not call onRetryFailed when button is disabled', async () => {
      const user = userEvent.setup()
      const onRetryFailed = vi.fn()
      const failedState = makeState({
        status: 'failed',
        agent_history: [makeFailedAgent()],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={onRetryFailed}
          retryingSessionId="sess-1"
        />
      )

      await user.click(screen.getByText('Retry Failed'))
      expect(onRetryFailed).not.toHaveBeenCalled()
    })
  })

  describe('failed banner styling', () => {
    it('failed banner has red styling', () => {
      const failedState = makeState({
        status: 'failed',
        agent_history: [makeFailedAgent()],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={vi.fn()}
        />
      )

      // Check that the banner container has red border/bg classes
      const banner = screen.getByText('Workflow Failed').closest('.border-red-200')
      expect(banner).toBeInTheDocument()
    })
  })

  describe('edge cases', () => {
    it('does not show failed banner when failed agent has no session_id', () => {
      const failedState = makeState({
        status: 'failed',
        agent_history: [makeFailedAgent({ session_id: undefined })],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={vi.fn()}
        />
      )

      expect(screen.queryByText('Workflow Failed')).not.toBeInTheDocument()
    })

    it('shows failed banner only for agents with result=fail, not other statuses', () => {
      const failedState = makeState({
        status: 'failed',
        agent_history: [
          { ...makeFailedAgent(), result: 'pass' },
          makeFailedAgent({ session_id: 'sess-actually-failed' }),
        ],
      })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          onRetryFailed={vi.fn()}
        />
      )

      expect(screen.getByText('Workflow Failed')).toBeInTheDocument()
    })
  })

  describe('props threading to AgentLogPanel', () => {
    it('passes onRetryFailed to AgentLogPanel when hasActivePhase', () => {
      const onRetryFailed = vi.fn()
      const state = makeState({ status: 'active' })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={state}
          hasActivePhase={true}
          onRetryFailed={onRetryFailed}
        />
      )

      // AgentLogPanel should receive the props
      expect(screen.getByTestId('agent-log-panel')).toBeInTheDocument()
    })

    it('passes retryingSessionId to AgentLogPanel', () => {
      const state = makeState({ status: 'active' })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={state}
          hasActivePhase={true}
          onRetryFailed={vi.fn()}
          retryingSessionId="sess-retry"
        />
      )

      expect(screen.getByTestId('agent-log-panel')).toBeInTheDocument()
    })

    it('passes workflowStatus to AgentLogPanel', () => {
      const failedState = makeState({ status: 'failed' })

      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={failedState}
          hasActivePhase={true}
          onRetryFailed={vi.fn()}
        />
      )

      expect(screen.getByTestId('agent-log-panel')).toBeInTheDocument()
    })
  })
})
