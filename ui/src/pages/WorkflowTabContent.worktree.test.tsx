import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowState } from '@/types/workflow'

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

describe('WorkflowTabContent — worktree display', () => {
  it('does not render worktree path line when worktree_path is absent', () => {
    render(<WorkflowTabContent {...defaultProps} displayedState={makeState()} />)
    expect(screen.queryByText(/\/tmp\/nrflow\/worktrees/)).not.toBeInTheDocument()
  })

  it('does not render worktree path line when worktree_path is empty string', () => {
    render(<WorkflowTabContent {...defaultProps} displayedState={makeState({ worktree_path: '' })} />)
    // Empty string is falsy — should not render the path line
    const spans = screen.queryAllByTitle('')
    expect(spans.filter(el => el.tagName === 'SPAN' && el.classList.contains('truncate'))).toHaveLength(0)
  })

  it('renders worktree path text when worktree_path is present', () => {
    const path = '/tmp/nrflow/worktrees/T-1'
    render(
      <WorkflowTabContent
        {...defaultProps}
        displayedState={makeState({ worktree_path: path })}
      />
    )
    expect(screen.getByText(path)).toBeInTheDocument()
  })

  it('sets title attribute on path span for overflow hint', () => {
    const path = '/tmp/nrflow/worktrees/some-very-long-branch-name'
    render(
      <WorkflowTabContent
        {...defaultProps}
        displayedState={makeState({ worktree_path: path })}
      />
    )
    const span = screen.getByTitle(path)
    expect(span).toBeInTheDocument()
    expect(span).toHaveTextContent(path)
  })

  it('shows branch name in tooltip on hover when branch_name is present', async () => {
    const user = userEvent.setup()
    const path = '/tmp/nrflow/worktrees/T-1'
    const branch = 'T-1'
    render(
      <WorkflowTabContent
        {...defaultProps}
        displayedState={makeState({ worktree_path: path, branch_name: branch })}
      />
    )
    const pathSpan = screen.getByText(path)
    // Hover the tooltip trigger (the wrapping inline-flex span)
    await user.hover(pathSpan)
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent(`Branch: ${branch}`)
  })

  it('does not show branch tooltip text when branch_name is absent', async () => {
    const user = userEvent.setup()
    const path = '/tmp/nrflow/worktrees/T-1'
    render(
      <WorkflowTabContent
        {...defaultProps}
        displayedState={makeState({ worktree_path: path })}
      />
    )
    const pathSpan = screen.getByText(path)
    await user.hover(pathSpan)
    // Tooltip text is empty string — no "Branch:" text rendered
    expect(screen.queryByText(/^Branch:/)).not.toBeInTheDocument()
  })

  it('still renders worktree path for completed workflows', () => {
    const path = '/tmp/nrflow/worktrees/T-99'
    render(
      <WorkflowTabContent
        {...defaultProps}
        displayedState={makeState({ status: 'completed', worktree_path: path, branch_name: 'T-99' })}
      />
    )
    expect(screen.getByText(path)).toBeInTheDocument()
  })
})
