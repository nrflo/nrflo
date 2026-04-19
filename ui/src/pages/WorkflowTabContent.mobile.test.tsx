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

function getFlexContainer() {
  const phaseTimeline = screen.getByTestId('phase-timeline')
  return phaseTimeline.parentElement!.parentElement!
}

describe('WorkflowTabContent - Mobile Layout (nrflo-395fca)', () => {
  describe('responsive flex direction', () => {
    it('container has flex-col for mobile stacking', () => {
      render(<WorkflowTabContent {...defaultProps} />)
      const container = getFlexContainer()
      expect(container.className).toContain('flex-col')
    })

    it('container has md:flex-row for desktop side-by-side', () => {
      render(<WorkflowTabContent {...defaultProps} />)
      const container = getFlexContainer()
      expect(container.className).toContain('md:flex-row')
    })

    it('container does not have a bare flex-row (only md:flex-row)', () => {
      render(<WorkflowTabContent {...defaultProps} />)
      const container = getFlexContainer()
      // 'md:flex-row' contains 'flex-row' as substring, so we check the split classes
      const classes = container.className.split(' ')
      expect(classes).not.toContain('flex-row')
    })
  })

  describe('min-h gated behind md: breakpoint', () => {
    it('with active phase: min-h uses md: prefix (not applied on mobile)', () => {
      render(<WorkflowTabContent {...defaultProps} hasActivePhase={true} />)
      const container = getFlexContainer()
      expect(container.className).toContain('md:min-h-')
    })

    it('with active phase: does not have bare min-h-[calc(100vh-280px)] (would apply on mobile)', () => {
      render(<WorkflowTabContent {...defaultProps} hasActivePhase={true} />)
      const container = getFlexContainer()
      const classes = container.className.split(' ')
      expect(classes).not.toContain('min-h-[calc(100vh-280px)]')
    })

    it('without active phase: no min-h class applied', () => {
      render(<WorkflowTabContent {...defaultProps} hasActivePhase={false} />)
      const container = getFlexContainer()
      expect(container.className).not.toContain('min-h-')
    })

    it('with selected panel agent: min-h uses md: prefix', () => {
      const agent = {
        phaseName: 'implementation',
        agent: {
          agent_id: 'a1', agent_type: 'implementor', phase: 'implementation',
          model_id: 'sonnet', cli: 'claude', model: 'sonnet', pid: 1,
          session_id: 's1', started_at: '2026-01-01T00:00:00Z',
        },
        session: undefined,
      }
      render(<WorkflowTabContent {...defaultProps} hasActivePhase={false} selectedPanelAgent={agent} />)
      const container = getFlexContainer()
      expect(container.className).toContain('md:min-h-')
    })
  })
})
