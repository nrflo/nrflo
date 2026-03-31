import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowState, ActiveAgentV4 } from '@/types/workflow'

// Mock PhaseTimeline to expose logPanelCollapsed prop
const mockPhaseTimeline = vi.fn()
vi.mock('@/components/workflow/PhaseTimeline', () => ({
  PhaseTimeline: (props: any) => {
    mockPhaseTimeline(props)
    return <div data-testid="phase-timeline" data-collapsed={props.logPanelCollapsed}>PhaseTimeline</div>
  },
}))

// Mock AgentLogPanel to verify width classes
const mockAgentLogPanel = vi.fn()
vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: (props: any) => {
    mockAgentLogPanel(props)
    return (
      <div
        data-testid="agent-log-panel"
        data-collapsed={props.collapsed}
        className={props.collapsed ? 'w-10 shrink-0' : 'flex-1 min-w-[280px]'}
      >
        AgentLogPanel
      </div>
    )
  },
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
  hasActivePhase: true,
  activeAgents: { 'implementor:claude:sonnet': makeAgent() },
  sessions: [],
  logPanelCollapsed: false,
  onToggleLogPanel: vi.fn(),
  selectedPanelAgent: null,
  onAgentSelect: vi.fn(),
  onStop: vi.fn(),
  stopPending: false,
  onShowRunDialog: vi.fn(),
}

describe('WorkflowTabContent - Panel Integration (nrworkflow-28182f)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('logPanelCollapsed prop threading', () => {
    it('threads logPanelCollapsed to PhaseTimeline', () => {
      render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      expect(mockPhaseTimeline).toHaveBeenCalledWith(
        expect.objectContaining({
          logPanelCollapsed: false,
        })
      )
    })

    it('threads logPanelCollapsed=true to PhaseTimeline', () => {
      render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)

      expect(mockPhaseTimeline).toHaveBeenCalledWith(
        expect.objectContaining({
          logPanelCollapsed: true,
        })
      )
    })

    it('updates logPanelCollapsed when prop changes', () => {
      const { rerender } = render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      expect(mockPhaseTimeline).toHaveBeenLastCalledWith(
        expect.objectContaining({
          logPanelCollapsed: false,
        })
      )

      rerender(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)

      expect(mockPhaseTimeline).toHaveBeenLastCalledWith(
        expect.objectContaining({
          logPanelCollapsed: true,
        })
      )
    })
  })

  describe('AgentLogPanel width verification', () => {
    it('AgentLogPanel has flex-1 when expanded', () => {
      render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      const panel = screen.getByTestId('agent-log-panel')
      expect(panel.className).toContain('flex-1')
      expect(panel.className).not.toContain('w-10')
    })

    it('AgentLogPanel has w-10 when collapsed', () => {
      render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)

      const panel = screen.getByTestId('agent-log-panel')
      expect(panel.className).toContain('w-10')
      expect(panel.className).not.toContain('flex-1')
    })

    it('AgentLogPanel width changes when logPanelCollapsed toggles', () => {
      const { rerender } = render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      let panel = screen.getByTestId('agent-log-panel')
      expect(panel.className).toContain('flex-1')

      rerender(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)

      panel = screen.getByTestId('agent-log-panel')
      expect(panel.className).toContain('w-10')
    })
  })

  describe('full flow: panel toggle + graph re-center + width change', () => {
    it('full flow: toggling panel passes correct state to both PhaseTimeline and AgentLogPanel', () => {
      const { rerender } = render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      // Initial state: expanded
      expect(mockPhaseTimeline).toHaveBeenLastCalledWith(
        expect.objectContaining({ logPanelCollapsed: false })
      )
      expect(mockAgentLogPanel).toHaveBeenLastCalledWith(
        expect.objectContaining({ collapsed: false })
      )
      const panelExpanded = screen.getByTestId('agent-log-panel')
      expect(panelExpanded.className).toContain('flex-1')

      // Toggle to collapsed
      rerender(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)

      expect(mockPhaseTimeline).toHaveBeenLastCalledWith(
        expect.objectContaining({ logPanelCollapsed: true })
      )
      expect(mockAgentLogPanel).toHaveBeenLastCalledWith(
        expect.objectContaining({ collapsed: true })
      )
      const panelCollapsed = screen.getByTestId('agent-log-panel')
      expect(panelCollapsed.className).toContain('w-10')

      // Toggle back to expanded
      rerender(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      expect(mockPhaseTimeline).toHaveBeenLastCalledWith(
        expect.objectContaining({ logPanelCollapsed: false })
      )
      expect(mockAgentLogPanel).toHaveBeenLastCalledWith(
        expect.objectContaining({ collapsed: false })
      )
      const panelExpandedAgain = screen.getByTestId('agent-log-panel')
      expect(panelExpandedAgain.className).toContain('flex-1')
    })

    it('full flow: panel state controls both graph fitView trigger and panel width simultaneously', async () => {
      const { rerender } = render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      // Verify initial state
      const phaseTimeline = screen.getByTestId('phase-timeline')
      expect(phaseTimeline.getAttribute('data-collapsed')).toBe('false')
      const panel = screen.getByTestId('agent-log-panel')
      expect(panel.getAttribute('data-collapsed')).toBe('false')
      expect(panel.className).toContain('flex-1')

      // Simulate user toggling panel
      rerender(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)

      // Both components should update simultaneously
      const phaseTimelineAfterToggle = screen.getByTestId('phase-timeline')
      expect(phaseTimelineAfterToggle.getAttribute('data-collapsed')).toBe('true')
      const panelAfterToggle = screen.getByTestId('agent-log-panel')
      expect(panelAfterToggle.getAttribute('data-collapsed')).toBe('true')
      expect(panelAfterToggle.className).toContain('w-10')
    })
  })

  describe('with selected agent (detail mode)', () => {
    it('shows AgentLogPanel with flex-1 when agent is selected and panel expanded', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
      }

      render(
        <WorkflowTabContent
          {...defaultProps}
          selectedPanelAgent={selectedAgent}
          logPanelCollapsed={false}
        />
      )

      expect(screen.getByTestId('agent-log-panel')).toBeInTheDocument()
      const panel = screen.getByTestId('agent-log-panel')
      expect(panel.className).toContain('flex-1')
    })

    it('threads logPanelCollapsed to PhaseTimeline when agent is selected', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
      }

      render(
        <WorkflowTabContent
          {...defaultProps}
          selectedPanelAgent={selectedAgent}
          logPanelCollapsed={true}
        />
      )

      expect(mockPhaseTimeline).toHaveBeenCalledWith(
        expect.objectContaining({
          logPanelCollapsed: true,
        })
      )
    })
  })

  describe('without active agents (panel hidden)', () => {
    it('does not render AgentLogPanel when no active agents and no selected agent', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          hasActivePhase={false}
          activeAgents={{}}
          selectedPanelAgent={null}
        />
      )

      expect(screen.queryByTestId('agent-log-panel')).not.toBeInTheDocument()
    })

    it('still threads logPanelCollapsed to PhaseTimeline even when panel is hidden', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          hasActivePhase={false}
          activeAgents={{}}
          selectedPanelAgent={null}
          logPanelCollapsed={true}
        />
      )

      expect(mockPhaseTimeline).toHaveBeenCalledWith(
        expect.objectContaining({
          logPanelCollapsed: true,
        })
      )
    })
  })

  describe('layout integration', () => {
    it('main graph container and agent panel coexist with proper widths', () => {
      render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      // Main graph area - no max-w constraint, just flex-1
      const phaseTimeline = screen.getByTestId('phase-timeline')
      const mainContent = phaseTimeline.parentElement!
      expect(mainContent.className).not.toContain('max-w-6xl')
      expect(mainContent.className).toContain('flex-1')

      // Agent log panel uses flex-1 min-w-[280px]
      const panel = screen.getByTestId('agent-log-panel')
      expect(panel.className).toContain('flex-1')
      expect(panel.className).toContain('min-w-[280px]')
    })

    it('main graph container has no max-w constraint when panel is collapsed', () => {
      render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)

      const phaseTimeline = screen.getByTestId('phase-timeline')
      const mainContent = phaseTimeline.parentElement!
      expect(mainContent.className).not.toContain('max-w-6xl')

      const panel = screen.getByTestId('agent-log-panel')
      expect(panel.className).toContain('w-10')
    })

    it('parent container has flex layout for side-by-side placement', () => {
      render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      // Find the parent flex container
      const phaseTimeline = screen.getByTestId('phase-timeline')
      const mainContent = phaseTimeline.parentElement!
      const flexContainer = mainContent.parentElement!

      expect(flexContainer.className).toContain('flex')
      expect(flexContainer.className).toContain('gap-0')
    })
  })

  describe('prop consistency', () => {
    it('AgentLogPanel receives collapsed prop matching logPanelCollapsed', () => {
      render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)

      expect(mockAgentLogPanel).toHaveBeenCalledWith(
        expect.objectContaining({
          collapsed: true,
        })
      )
    })

    it('AgentLogPanel collapsed prop updates when logPanelCollapsed changes', () => {
      const { rerender } = render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      expect(mockAgentLogPanel).toHaveBeenLastCalledWith(
        expect.objectContaining({ collapsed: false })
      )

      rerender(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)

      expect(mockAgentLogPanel).toHaveBeenLastCalledWith(
        expect.objectContaining({ collapsed: true })
      )
    })

  })

  describe('edge cases', () => {
    it('handles rapid panel toggling without breaking layout', () => {
      const { rerender } = render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      // Rapid toggles
      rerender(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)
      rerender(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)
      rerender(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)
      rerender(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      // Final state should be consistent
      expect(mockPhaseTimeline).toHaveBeenLastCalledWith(
        expect.objectContaining({ logPanelCollapsed: false })
      )
      expect(mockAgentLogPanel).toHaveBeenLastCalledWith(
        expect.objectContaining({ collapsed: false })
      )
      const panel = screen.getByTestId('agent-log-panel')
      expect(panel.className).toContain('flex-1')
    })

    it('handles workflow state changes while panel is toggled', () => {
      const { rerender } = render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)

      // Change workflow state
      const newState = makeState({
        current_phase: 'verification',
        phases: { verification: { status: 'in_progress' } },
      })

      rerender(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={newState}
          logPanelCollapsed={true}
        />
      )

      // Panel state should remain consistent
      expect(mockPhaseTimeline).toHaveBeenLastCalledWith(
        expect.objectContaining({ logPanelCollapsed: true })
      )
      expect(mockAgentLogPanel).toHaveBeenLastCalledWith(
        expect.objectContaining({ collapsed: true })
      )
    })
  })

  describe('acceptance criteria verification', () => {
    it('AC1: toggling panel changes logPanelCollapsed prop to PhaseTimeline (triggers graph re-center)', () => {
      const { rerender } = render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      vi.clearAllMocks()

      // Simulate user clicking toggle
      rerender(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)

      // PhaseTimeline should receive new logPanelCollapsed value
      // This triggers FitViewOnChange useEffect in PhaseGraph
      expect(mockPhaseTimeline).toHaveBeenCalledWith(
        expect.objectContaining({ logPanelCollapsed: true })
      )
    })

    it('AC2: agent log panel fills remaining space with flex-1 min-w-[280px]', () => {
      render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      const panel = screen.getByTestId('agent-log-panel')

      // Panel uses flex-1 to fill remaining space
      expect(panel.className).toContain('flex-1')
      expect(panel.className).toContain('min-w-[280px]')
      // Old min-w-[300px] removed
      expect(panel.className).not.toContain('min-w-[300px]')
    })

    it('full acceptance: toggle panel → PhaseTimeline gets new prop → panel width changes', () => {
      const { rerender } = render(<WorkflowTabContent {...defaultProps} logPanelCollapsed={false} />)

      // Initial: expanded panel with flex-1
      let panel = screen.getByTestId('agent-log-panel')
      expect(panel.className).toContain('flex-1')
      expect(mockPhaseTimeline).toHaveBeenLastCalledWith(
        expect.objectContaining({ logPanelCollapsed: false })
      )

      vi.clearAllMocks()

      // User clicks collapse button
      rerender(<WorkflowTabContent {...defaultProps} logPanelCollapsed={true} />)

      // 1. Panel width changes to w-10 (collapsed)
      panel = screen.getByTestId('agent-log-panel')
      expect(panel.className).toContain('w-10')

      // 2. PhaseTimeline receives logPanelCollapsed=true
      expect(mockPhaseTimeline).toHaveBeenCalledWith(
        expect.objectContaining({ logPanelCollapsed: true })
      )

      // 3. PhaseTimeline → PhaseGraph → FitViewOnChange useEffect fires (tested in PhaseGraph.panel-toggle.test.tsx)
      // This completes the flow: toggle → re-center graph + panel width change
    })
  })
})
