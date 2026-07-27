import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowState, WorkflowInstanceStatus } from '@/types/workflow'

vi.mock('@/components/workflow/PhaseTimeline', () => ({
  PhaseTimeline: () => <div data-testid="phase-timeline">PhaseTimeline</div>,
}))
vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: () => <div data-testid="agent-log-panel">AgentLogPanel</div>,
}))
vi.mock('@/components/workflow/ConflictResolverBanner', () => ({
  ConflictResolverBanner: () => null,
}))
vi.mock('@/components/workflow/PlanApprovalBanner', () => ({
  PlanApprovalBanner: ({ instanceId, status }: { instanceId: string; status: string }) => (
    <div data-testid="plan-approval-banner" data-instance-id={instanceId} data-status={status} />
  ),
}))

function makeState(overrides: Partial<WorkflowState> = {}): WorkflowState {
  return {
    workflow: 'feature',
    version: 4,
    instance_id: 'inst-1',
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

const PLAN_SUSPENDED: WorkflowInstanceStatus[] = ['planning', 'waiting_input', 'waiting_approval']
const NOT_PLAN_SUSPENDED: WorkflowInstanceStatus[] = ['active', 'waiting', 'completed']

describe('WorkflowTabContent — plan approval banner', () => {
  it.each(PLAN_SUSPENDED)('mounts PlanApprovalBanner for status=%s', (status) => {
    render(<WorkflowTabContent {...defaultProps} displayedState={makeState({ status })} />)
    const banner = screen.getByTestId('plan-approval-banner')
    expect(banner).toBeInTheDocument()
    expect(banner).toHaveAttribute('data-instance-id', 'inst-1')
    expect(banner).toHaveAttribute('data-status', status)
  })

  it.each(NOT_PLAN_SUSPENDED)('does not mount PlanApprovalBanner for status=%s', (status) => {
    render(<WorkflowTabContent {...defaultProps} displayedState={makeState({ status })} onContinue={vi.fn()} />)
    expect(screen.queryByTestId('plan-approval-banner')).not.toBeInTheDocument()
  })

  it('does not mount PlanApprovalBanner when instance_id is missing, even for a plan-suspended status', () => {
    render(<WorkflowTabContent {...defaultProps} displayedState={makeState({ status: 'planning', instance_id: undefined })} />)
    expect(screen.queryByTestId('plan-approval-banner')).not.toBeInTheDocument()
  })

  // Guard against regressing the pre-existing waiting/failed/completed banners
  // sharing this slot (see WorkflowTabContent.test.tsx for the base coverage).
  it('still shows WorkflowPauseControls for status=waiting and not the plan banner', () => {
    render(
      <WorkflowTabContent
        {...defaultProps}
        displayedState={makeState({ status: 'waiting' })}
        onContinue={vi.fn()}
      />
    )
    expect(screen.queryByTestId('plan-approval-banner')).not.toBeInTheDocument()
  })

  it('still shows the Completed banner for status=completed and not the plan banner', () => {
    render(
      <WorkflowTabContent
        {...defaultProps}
        displayedState={makeState({ status: 'completed', completed_at: '2026-01-01T00:00:00Z' })}
      />
    )
    expect(screen.getByText('Completed')).toBeInTheDocument()
    expect(screen.queryByTestId('plan-approval-banner')).not.toBeInTheDocument()
  })
})
