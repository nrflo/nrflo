import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowState, ActiveAgentV4 } from '@/types/workflow'

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

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    model: 'sonnet',
    pid: 12345,
    session_id: 's1',
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const baseProps = {
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
  activeAgents: {} as Record<string, ActiveAgentV4>,
  sessions: [],
  logPanelCollapsed: false,
  onToggleLogPanel: vi.fn(),
  selectedPanelAgent: null,
  onAgentSelect: vi.fn(),
  onStop: vi.fn(),
  stopPending: false,
}

describe('WorkflowTabContent - collapse toggle in header', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('visibility conditions', () => {
    it('shows toggle when hasActivePhase is true', () => {
      render(<WorkflowTabContent {...baseProps} hasActivePhase={true} />)
      expect(screen.getByTitle('Collapse agent log')).toBeInTheDocument()
    })

    it('shows toggle when selectedPanelAgent is set (even if no active phase)', () => {
      const agent = { phaseName: 'implementation', agent: makeAgent() }
      render(<WorkflowTabContent {...baseProps} hasActivePhase={false} selectedPanelAgent={agent} />)
      expect(screen.getByTitle('Collapse agent log')).toBeInTheDocument()
    })

    it('hides toggle when neither hasActivePhase nor selectedPanelAgent', () => {
      render(<WorkflowTabContent {...baseProps} hasActivePhase={false} selectedPanelAgent={null} />)
      expect(screen.queryByTitle('Collapse agent log')).not.toBeInTheDocument()
      expect(screen.queryByTitle('Expand agent log')).not.toBeInTheDocument()
    })
  })

  describe('title and state', () => {
    it('has title "Collapse agent log" when panel is expanded', () => {
      render(<WorkflowTabContent {...baseProps} hasActivePhase={true} logPanelCollapsed={false} />)
      expect(screen.getByTitle('Collapse agent log')).toBeInTheDocument()
    })

    it('has title "Expand agent log" when panel is collapsed', () => {
      render(<WorkflowTabContent {...baseProps} hasActivePhase={true} logPanelCollapsed={true} />)
      expect(screen.getByTitle('Expand agent log')).toBeInTheDocument()
    })
  })

  describe('click behavior', () => {
    it('calls onToggleLogPanel when toggle is clicked', async () => {
      const user = userEvent.setup()
      const onToggleLogPanel = vi.fn()
      render(<WorkflowTabContent {...baseProps} hasActivePhase={true} onToggleLogPanel={onToggleLogPanel} />)

      await user.click(screen.getByTitle('Collapse agent log'))

      expect(onToggleLogPanel).toHaveBeenCalledTimes(1)
    })

    it('clicking toggle when collapsed calls onToggleLogPanel', async () => {
      const user = userEvent.setup()
      const onToggleLogPanel = vi.fn()
      render(
        <WorkflowTabContent
          {...baseProps}
          hasActivePhase={true}
          logPanelCollapsed={true}
          onToggleLogPanel={onToggleLogPanel}
        />
      )

      await user.click(screen.getByTitle('Expand agent log'))

      expect(onToggleLogPanel).toHaveBeenCalledTimes(1)
    })
  })

  describe('toggle coexists with other header controls', () => {
    it('toggle visible alongside Stop button when isOrchestrated and hasActivePhase', () => {
      render(
        <WorkflowTabContent
          {...baseProps}
          hasActivePhase={true}
          isOrchestrated={true}
          logPanelCollapsed={false}
        />
      )

      expect(screen.getByTitle('Collapse agent log')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })
  })
})
