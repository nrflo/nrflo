import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowState } from '@/types/workflow'

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
    status: 'active',
    phases: {},
    phase_order: [],
    ...overrides,
  }
}

const defaultProps = {
  ticketId: 'T-1',
  hasWorkflow: true,
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
}

describe('WorkflowTabContent — workflow_final_result banner', () => {
  it('shows result text when completed with workflow_final_result', () => {
    const state = makeState({
      status: 'completed',
      workflow_final_result: 'Implementation complete: auth middleware added',
    })
    render(<WorkflowTabContent {...defaultProps} displayedState={state} />)
    expect(screen.getByText('Implementation complete: auth middleware added')).toBeInTheDocument()
  })

  it('shows result text for project_completed status', () => {
    const state = makeState({
      status: 'project_completed',
      workflow_final_result: 'Refactor done — all tests pass',
    })
    render(<WorkflowTabContent {...defaultProps} displayedState={state} />)
    expect(screen.getByText('Refactor done — all tests pass')).toBeInTheDocument()
  })

  it('does not show result banner when workflow_final_result is absent', () => {
    const state = makeState({ status: 'completed' })
    render(<WorkflowTabContent {...defaultProps} displayedState={state} />)
    // Completion banner shows, but no result-specific text
    expect(screen.getByText('Completed')).toBeInTheDocument()
    // No stray banner text from an empty result field
    expect(screen.queryByText(/Implementation complete/)).not.toBeInTheDocument()
  })

  it('does not show result banner for active workflow even with workflow_final_result set', () => {
    const state = makeState({
      status: 'active',
      workflow_final_result: 'Should not show yet',
    })
    render(<WorkflowTabContent {...defaultProps} displayedState={state} />)
    expect(screen.queryByText('Should not show yet')).not.toBeInTheDocument()
  })

  it('does not show result banner for failed workflow', () => {
    const state = makeState({
      status: 'failed',
      workflow_final_result: 'Should not show on failure',
    })
    render(<WorkflowTabContent {...defaultProps} displayedState={state} />)
    expect(screen.queryByText('Should not show on failure')).not.toBeInTheDocument()
  })

  it('renders result as markdown with a collapsed <details> section', () => {
    const state = makeState({
      status: 'completed',
      workflow_final_result: '## Digest\n\n<details><summary>Sources (1)</summary>\n\n| A |\n|---|\n| 1 |\n\n</details>',
    })
    const { container } = render(<WorkflowTabContent {...defaultProps} displayedState={state} />)
    expect(screen.getByRole('heading', { level: 2, name: 'Digest' })).toBeInTheDocument()
    const details = container.querySelector('details')
    expect(details).not.toBeNull()
    expect(details?.open).toBe(false)
    expect(details?.querySelector('table')).not.toBeNull()
  })
})
