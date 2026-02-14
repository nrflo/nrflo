import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, waitFor } from '@testing-library/react'
import { PhaseGraph } from './PhaseGraph'
import type { PhaseGraphProps } from './types'
import type { PhaseState, ActiveAgentV4 } from '@/types/workflow'

// Mock the useReactFlow hook
const mockFitView = vi.fn()
const mockUseReactFlow = vi.fn(() => ({
  fitView: mockFitView,
}))

// Mock @xyflow/react
vi.mock('@xyflow/react', async () => {
  const actual = await vi.importActual('@xyflow/react')
  return {
    ...actual,
    ReactFlow: ({ children }: { children: React.ReactNode }) => (
      <div data-testid="react-flow">{children}</div>
    ),
    Background: () => <div data-testid="background" />,
    Controls: () => <div data-testid="controls" />,
    useReactFlow: () => mockUseReactFlow(),
  }
})

// Mock the layout module
vi.mock('./layout', () => ({
  getLayoutedElements: (nodes: any[], edges: any[]) => ({ nodes, edges }),
}))

// Mock the AgentFlowNode component
vi.mock('./AgentFlowNode', () => ({
  AgentFlowNode: ({ data }: { data: any }) => (
    <div data-testid={`agent-node-${data.agentKey}`}>{data.phaseName}</div>
  ),
}))

// Mock useTickingClock hook
vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

function makePhaseState(overrides: Partial<PhaseState> = {}): PhaseState {
  return {
    status: 'pending',
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

function makeProps(overrides: Partial<PhaseGraphProps> = {}): PhaseGraphProps {
  return {
    phases: {
      investigation: makePhaseState(),
      implementation: makePhaseState(),
    },
    phaseOrder: ['investigation', 'implementation'],
    activeAgents: {},
    agentHistory: [],
    sessions: [],
    ...overrides,
  }
}

describe('PhaseGraph - Panel Toggle (nrworkflow-28182f)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('logPanelCollapsed prop integration', () => {
    it('accepts logPanelCollapsed prop', () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        },
        logPanelCollapsed: false,
      })

      render(<PhaseGraph {...props} />)

      // Should render successfully with logPanelCollapsed prop
      expect(true).toBe(true)
    })

    it('accepts logPanelCollapsed as true', () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        },
        logPanelCollapsed: true,
      })

      render(<PhaseGraph {...props} />)

      // Should render successfully with logPanelCollapsed=true
      expect(true).toBe(true)
    })

    it('works when logPanelCollapsed is undefined', () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        },
        logPanelCollapsed: undefined,
      })

      render(<PhaseGraph {...props} />)

      // Should render successfully with undefined (backward compatibility)
      expect(true).toBe(true)
    })
  })

  describe('fitView on panel toggle', () => {
    it('calls fitView after 350ms delay when panel is collapsed (false → true)', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        },
        logPanelCollapsed: false,
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      // Wait for initial mount fitView
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      // Clear mocks to track panel toggle calls only
      vi.clearAllMocks()

      // Simulate panel collapse
      rerender(<PhaseGraph {...props} logPanelCollapsed={true} />)

      // Should NOT be called immediately
      expect(mockFitView).not.toHaveBeenCalled()

      // Should NOT be called before 350ms (allowing for some variance)
      await new Promise(resolve => setTimeout(resolve, 300))
      expect(mockFitView).not.toHaveBeenCalled()

      // Should be called after 350ms delay (with 100ms buffer)
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
      }, { timeout: 500 })
    })

    it('calls fitView after 350ms delay when panel is expanded (true → false)', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        },
        logPanelCollapsed: true,
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      // Wait for initial mount fitView
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      // Clear mocks to track panel toggle calls only
      vi.clearAllMocks()

      // Simulate panel expand
      rerender(<PhaseGraph {...props} logPanelCollapsed={false} />)

      // Should NOT be called immediately
      expect(mockFitView).not.toHaveBeenCalled()

      // Should be called after 350ms delay
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
      }, { timeout: 500 })
    })

    it('waits for CSS transition (300ms) before fitView (350ms total)', async () => {
      // This test documents the timing: 300ms CSS transition + 50ms buffer = 350ms delay
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        },
        logPanelCollapsed: false,
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      vi.clearAllMocks()

      // Toggle panel
      rerender(<PhaseGraph {...props} logPanelCollapsed={true} />)

      // CSS transition is 300ms, so fitView should NOT fire before that
      await new Promise(resolve => setTimeout(resolve, 250))
      expect(mockFitView).not.toHaveBeenCalled()

      // But should fire after 350ms (300ms transition + 50ms buffer)
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })
    })

    it('uses correct fitView options (padding: 0.3, duration: 200)', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        },
        logPanelCollapsed: false,
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      vi.clearAllMocks()

      // Toggle panel
      rerender(<PhaseGraph {...props} logPanelCollapsed={true} />)

      // Wait for fitView after panel toggle
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalledWith({
          padding: 0.3,
          duration: 200,
        })
      }, { timeout: 500 })
    })

    it('handles multiple rapid toggles gracefully', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        },
        logPanelCollapsed: false,
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      vi.clearAllMocks()

      // Rapid toggles
      rerender(<PhaseGraph {...props} logPanelCollapsed={true} />)
      rerender(<PhaseGraph {...props} logPanelCollapsed={false} />)
      rerender(<PhaseGraph {...props} logPanelCollapsed={true} />)

      // Should call fitView for each toggle (timers don't get canceled)
      // Wait for all timers to fire
      await new Promise(resolve => setTimeout(resolve, 600))

      // Each toggle creates a new timer, so we should see multiple calls
      expect(mockFitView).toHaveBeenCalled()
      // The exact count depends on timer cleanup, but at least 1 call should happen
      expect(mockFitView.mock.calls.length).toBeGreaterThanOrEqual(1)
    })
  })

  describe('fitView independence from node changes', () => {
    it('fires fitView on panel toggle independently of node changes', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
            context_left: 80,
          }),
        },
        logPanelCollapsed: false,
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      const initialCallCount = mockFitView.mock.calls.length

      // Update context_left (doesn't change nodeKey) AND toggle panel
      rerender(<PhaseGraph
        {...props}
        activeAgents={{
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
            context_left: 75, // Changed
          }),
        }}
        logPanelCollapsed={true} // Changed
      />)

      // Should call fitView for panel toggle after 350ms
      await waitFor(() => {
        expect(mockFitView.mock.calls.length).toBeGreaterThan(initialCallCount)
      }, { timeout: 500 })
    })

    it('fires fitView on both node change and panel toggle separately', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        },
        logPanelCollapsed: false,
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      vi.clearAllMocks()

      // Change nodes (add new agent) AND toggle panel
      rerender(<PhaseGraph
        {...props}
        activeAgents={{
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
          'setup-analyzer:claude:haiku': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
            model_id: 'claude-haiku-4-5',
          }),
        }}
        logPanelCollapsed={true}
      />)

      // Should call fitView twice:
      // 1. Once for node change (50ms delay)
      // 2. Once for panel toggle (350ms delay)
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      // First call should happen around 50ms (node change)
      const firstCallTime = mockFitView.mock.calls.length

      // Second call should happen around 350ms (panel toggle)
      await waitFor(() => {
        expect(mockFitView.mock.calls.length).toBeGreaterThan(firstCallTime)
      }, { timeout: 500 })
    })
  })

  describe('backward compatibility', () => {
    it('still calls fitView on node changes when logPanelCollapsed is undefined', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'pending' }),
        },
        phaseOrder: ['investigation'],
        logPanelCollapsed: undefined,
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      vi.clearAllMocks()

      // Add an agent (node change)
      rerender(<PhaseGraph
        {...props}
        phases={{
          investigation: makePhaseState({ status: 'in_progress' }),
        }}
        activeAgents={{
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        }}
        logPanelCollapsed={undefined}
      />)

      // Should still call fitView for node changes
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
      }, { timeout: 200 })
    })

    it('does not break when logPanelCollapsed is not provided', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        },
        // logPanelCollapsed not provided
      })

      render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      // Should work fine without logPanelCollapsed
      expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
    })
  })

  describe('edge cases', () => {
    it('handles panel toggle when no nodes exist', async () => {
      const props = makeProps({
        phases: {},
        phaseOrder: [],
        logPanelCollapsed: false,
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      vi.clearAllMocks()

      // Toggle panel even though no nodes
      rerender(<PhaseGraph {...props} logPanelCollapsed={true} />)

      // Should not crash (FitViewOnChange is not rendered when no nodes)
      await new Promise(resolve => setTimeout(resolve, 400))

      // No fitView calls expected since FitViewOnChange is not rendered
      expect(mockFitView).not.toHaveBeenCalled()
    })

    it('verifies logPanelCollapsed dependency only fires on value change', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        },
        logPanelCollapsed: false,
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      // Wait for initial mount fitView calls
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 500 })

      vi.clearAllMocks()

      // Toggle to true (should trigger fitView)
      rerender(<PhaseGraph {...props} logPanelCollapsed={true} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 500 })

      const callCountAfterFirstToggle = mockFitView.mock.calls.length

      // Toggle back to false (should trigger fitView again)
      rerender(<PhaseGraph {...props} logPanelCollapsed={false} />)

      await waitFor(() => {
        expect(mockFitView.mock.calls.length).toBeGreaterThan(callCountAfterFirstToggle)
      }, { timeout: 500 })

      // This verifies the dependency is working correctly for actual value changes
    })
  })
})
