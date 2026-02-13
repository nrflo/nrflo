import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowState } from '@/types/workflow'

// Mock heavy child components to focus on container layout
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
    category: 'full',
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

describe('WorkflowTabContent', () => {
  // Main graph container width (ticket nrworkflow-30efa6)
  it('renders main graph area with max-w-6xl (wider ~30%)', () => {
    render(<WorkflowTabContent {...defaultProps} />)
    // The main content div should have max-w-6xl class
    const mainContent = screen.getByTestId('phase-timeline').parentElement!
    expect(mainContent.className).toContain('max-w-6xl')
  })

  it('main graph area does not use old max-w-4xl', () => {
    render(<WorkflowTabContent {...defaultProps} />)
    const mainContent = screen.getByTestId('phase-timeline').parentElement!
    expect(mainContent.className).not.toContain('max-w-4xl')
  })

  it('main content area has flex-1 for flexible layout', () => {
    render(<WorkflowTabContent {...defaultProps} />)
    const mainContent = screen.getByTestId('phase-timeline').parentElement!
    expect(mainContent.className).toContain('flex-1')
  })

  // No workflow state
  it('shows "No workflow configured for this ticket" when hasWorkflow is false and onShowRunDialog provided', () => {
    render(<WorkflowTabContent {...defaultProps} hasWorkflow={false} displayedState={null} />)
    expect(screen.getByText('No workflow configured for this ticket')).toBeInTheDocument()
  })

  it('shows "No workflows in this tab" when hasWorkflow is false and onShowRunDialog is undefined', () => {
    render(
      <WorkflowTabContent
        {...defaultProps}
        hasWorkflow={false}
        displayedState={null}
        onShowRunDialog={undefined}
      />
    )
    expect(screen.getByText('No workflows in this tab')).toBeInTheDocument()
  })

  it('shows "Run Workflow" button when no active phase or orchestration and onShowRunDialog provided', () => {
    render(<WorkflowTabContent {...defaultProps} />)
    expect(screen.getByText('Run Workflow')).toBeInTheDocument()
  })

  it('hides "Run Workflow" button when onShowRunDialog is undefined', () => {
    render(<WorkflowTabContent {...defaultProps} onShowRunDialog={undefined} />)
    expect(screen.queryByText('Run Workflow')).not.toBeInTheDocument()
  })

  it('does not show "Run Workflow" button in empty state when onShowRunDialog is undefined', () => {
    render(
      <WorkflowTabContent
        {...defaultProps}
        hasWorkflow={false}
        displayedState={null}
        onShowRunDialog={undefined}
      />
    )
    expect(screen.queryByText('Run Workflow')).not.toBeInTheDocument()
  })

  it('shows workflow name badge for single workflow', () => {
    render(<WorkflowTabContent {...defaultProps} />)
    expect(screen.getByText('feature')).toBeInTheDocument()
  })

  // Completed workflow stats banner
  it('shows completed banner with stats', () => {
    const state = makeState({
      status: 'completed',
      completed_at: '2026-01-01T05:00:00Z',
      total_duration_sec: 3600,
      total_tokens_used: 150000,
    })
    render(<WorkflowTabContent {...defaultProps} displayedState={state} />)
    expect(screen.getByText('Completed')).toBeInTheDocument()
  })

  // AgentLogPanel visibility
  it('shows AgentLogPanel when there is a selected panel agent', () => {
    render(
      <WorkflowTabContent
        {...defaultProps}
        selectedPanelAgent={{ phaseName: 'implementation' }}
      />
    )
    expect(screen.getByTestId('agent-log-panel')).toBeInTheDocument()
  })

  it('hides AgentLogPanel when no active phase and no selected agent', () => {
    render(<WorkflowTabContent {...defaultProps} />)
    expect(screen.queryByTestId('agent-log-panel')).not.toBeInTheDocument()
  })

  // Stop button
  it('shows Stop button when orchestrated', () => {
    render(<WorkflowTabContent {...defaultProps} isOrchestrated={true} />)
    expect(screen.getByText('Stop')).toBeInTheDocument()
  })

  it('shows Stop button when has active phase', () => {
    render(<WorkflowTabContent {...defaultProps} hasActivePhase={true} />)
    expect(screen.getByText('Stop')).toBeInTheDocument()
  })

  // Full flow: graph container is wider AND phase timeline renders inside it
  it('full flow: wider graph container wraps PhaseTimeline for active workflow', () => {
    render(
      <WorkflowTabContent
        {...defaultProps}
        hasWorkflow={true}
        displayedState={makeState()}
        hasActivePhase={true}
        selectedPanelAgent={{ phaseName: 'implementation' }}
      />
    )
    // PhaseTimeline present
    expect(screen.getByTestId('phase-timeline')).toBeInTheDocument()
    // Container uses max-w-6xl
    const mainContent = screen.getByTestId('phase-timeline').parentElement!
    expect(mainContent.className).toContain('max-w-6xl')
    // AgentLogPanel visible alongside
    expect(screen.getByTestId('agent-log-panel')).toBeInTheDocument()
  })

  // Tab context - Completed tab usage (onShowRunDialog=undefined)
  it('full flow: Completed tab shows workflow without Run Workflow button', () => {
    const completedState = makeState({
      status: 'completed',
      completed_at: '2026-01-01T05:00:00Z',
      total_duration_sec: 3600,
      total_tokens_used: 150000,
    })
    render(
      <WorkflowTabContent
        {...defaultProps}
        displayedState={completedState}
        onShowRunDialog={undefined}
      />
    )
    expect(screen.getByText('Completed')).toBeInTheDocument()
    expect(screen.queryByText('Run Workflow')).not.toBeInTheDocument()
    expect(screen.queryByText('Stop')).not.toBeInTheDocument()
  })

  it('full flow: Active tab can run workflows via Run Workflow button', () => {
    const onShowRunDialog = vi.fn()
    render(
      <WorkflowTabContent
        {...defaultProps}
        hasWorkflow={true}
        onShowRunDialog={onShowRunDialog}
      />
    )
    const runButton = screen.getByText('Run Workflow')
    runButton.click()
    expect(onShowRunDialog).toHaveBeenCalledTimes(1)
  })
})
