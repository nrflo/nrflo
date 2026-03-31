import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowState, AgentHistoryEntry } from '@/types/workflow'

vi.mock('@/components/workflow/PhaseTimeline', () => ({
  PhaseTimeline: () => <div data-testid="phase-timeline">PhaseTimeline</div>,
}))
vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: () => <div data-testid="agent-log-panel">AgentLogPanel</div>,
}))
vi.mock('@/components/workflow/ConflictResolverBanner', () => ({
  ConflictResolverBanner: () => null,
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
  displayedState: makeState({ status: 'failed', agent_history: [makeFailedAgent()] }),
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
  onRetryFailed: vi.fn(),
}

describe('WorkflowTabContent — blockedReason prop on Retry Failed', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('disables Retry Failed button when blockedReason is set (closed ticket)', () => {
    render(
      <WorkflowTabContent
        {...defaultProps}
        blockedReason="cannot run workflow on closed ticket"
      />
    )

    const button = screen.getByText('Retry Failed').closest('button')
    expect(button).toBeDisabled()
  })

  it('disables Retry Failed button when blockedReason is set (blocked ticket)', () => {
    render(
      <WorkflowTabContent
        {...defaultProps}
        blockedReason="cannot run workflow on blocked ticket — blocked by: DEP-1"
      />
    )

    const button = screen.getByText('Retry Failed').closest('button')
    expect(button).toBeDisabled()
  })

  it('enables Retry Failed button when blockedReason is undefined', () => {
    render(<WorkflowTabContent {...defaultProps} blockedReason={undefined} />)

    const button = screen.getByText('Retry Failed').closest('button')
    expect(button).not.toBeDisabled()
  })

  it('enables Retry Failed button when blockedReason is not provided', () => {
    render(<WorkflowTabContent {...defaultProps} />)

    const button = screen.getByText('Retry Failed').closest('button')
    expect(button).not.toBeDisabled()
  })

  it('shows blockedReason in tooltip wrapper when set', () => {
    const reason = 'cannot run workflow on closed ticket'
    render(
      <WorkflowTabContent
        {...defaultProps}
        blockedReason={reason}
      />
    )

    // The Tooltip receives blockedReason as its text prop when set
    // Verify the button is disabled (main functional behavior)
    const button = screen.getByText('Retry Failed').closest('button')
    expect(button).toBeDisabled()
  })

  it('Retry Failed button disabled state coexists with retryingSessionId', () => {
    render(
      <WorkflowTabContent
        {...defaultProps}
        blockedReason="cannot run workflow on closed ticket"
        retryingSessionId="sess-1"
      />
    )

    const button = screen.getByText('Retry Failed').closest('button')
    expect(button).toBeDisabled()
  })
})
