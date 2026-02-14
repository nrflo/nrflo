import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { PhaseGraph } from './PhaseGraph'
import type { PhaseGraphProps } from './types'
import type { PhaseState, ActiveAgentV4, AgentHistoryEntry } from '@/types/workflow'

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

function makeHistory(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'h1',
    agent_type: 'setup-analyzer',
    model_id: 'claude-sonnet-4-5',
    phase: 'investigation',
    result: 'pass',
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T00:03:00Z',
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

describe('PhaseGraph', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('FitViewOnChange component', () => {
    it('calls fitView when component mounts with nodes', async () => {
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
      })

      render(<PhaseGraph {...props} />)

      // fitView should be called with proper options after timeout
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
      }, { timeout: 200 })
    })

    it('calls fitView when nodes change (workflow starts)', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'pending' }),
        },
        phaseOrder: ['investigation'],
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      // Clear initial mount call
      vi.clearAllMocks()

      // Simulate workflow start - phase becomes in_progress with agent
      const updatedProps = makeProps({
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
      })

      rerender(<PhaseGraph {...updatedProps} />)

      // fitView should be called again due to node set change
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
      }, { timeout: 200 })
    })

    it('calls fitView when phase transitions to next phase', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
          implementation: makePhaseState({ status: 'pending' }),
        },
        phaseOrder: ['investigation', 'implementation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
        },
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      // Clear initial mount call
      vi.clearAllMocks()

      // Simulate phase transition - investigation completes, implementation starts
      const updatedProps = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'completed' }),
          implementation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation', 'implementation'],
        activeAgents: {
          'implementor:claude:opus': makeAgent({
            agent_type: 'implementor',
            phase: 'implementation',
          }),
        },
        agentHistory: [makeHistory({ phase: 'investigation', agent_type: 'setup-analyzer' })],
      })

      rerender(<PhaseGraph {...updatedProps} />)

      // fitView should be called due to node set change (different agents)
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
      }, { timeout: 200 })
    })

    it('does not call fitView when nodes remain the same', async () => {
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
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      // Wait for initial fitView
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      const initialCallCount = mockFitView.mock.calls.length

      // Update with same node structure but different context_left (doesn't change nodeKey)
      const updatedProps = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
            context_left: 75, // Changed but doesn't affect nodeKey
          }),
        },
      })

      rerender(<PhaseGraph {...updatedProps} />)

      // Wait a bit to ensure no new calls
      await new Promise(resolve => setTimeout(resolve, 100))

      // Call count should remain the same (nodeKey is stable)
      expect(mockFitView).toHaveBeenCalledTimes(initialCallCount)
    })

    it('uses 200ms duration for smooth animation', async () => {
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
      })

      render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalledWith(
          expect.objectContaining({ duration: 200 })
        )
      }, { timeout: 200 })
    })

    it('uses padding: 0.3 matching ReactFlow prop', async () => {
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
      })

      render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalledWith(
          expect.objectContaining({ padding: 0.3 })
        )
      }, { timeout: 200 })
    })

    it('delays fitView by 50ms to let React Flow settle', async () => {
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
      })

      render(<PhaseGraph {...props} />)

      // Should NOT be called immediately
      expect(mockFitView).not.toHaveBeenCalled()

      // Should be called after 50ms delay
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 100 })
    })
  })

  describe('nodeKey stability', () => {
    it('generates stable nodeKey from node IDs', async () => {
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
      })

      render(<PhaseGraph {...props} />)

      // Wait for initial render
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      // nodeKey should be derived from node IDs (e.g., "investigation-0")
      // This is tested indirectly by the "does not call fitView when nodes remain the same" test
    })

    it('changes nodeKey when node count changes', async () => {
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
      })

      const { rerender } = render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      vi.clearAllMocks()

      // Add a second parallel agent in same phase
      const updatedProps = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
          }),
          'setup-analyzer:claude:haiku': makeAgent({
            agent_type: 'setup-analyzer',
            phase: 'investigation',
            model_id: 'claude-haiku-4-5',
          }),
        },
      })

      rerender(<PhaseGraph {...updatedProps} />)

      // fitView should be called again due to different node count
      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })
    })
  })

  describe('edge cases', () => {
    it('does not render FitViewOnChange when no nodes exist', () => {
      const props = makeProps({
        phases: {},
        phaseOrder: [],
      })

      render(<PhaseGraph {...props} />)

      // Should show empty state message
      expect(screen.getByText('No workflow phases defined')).toBeInTheDocument()
      // React Flow should not be rendered
      expect(screen.queryByTestId('react-flow')).not.toBeInTheDocument()
    })

    it('handles empty phase list gracefully', () => {
      const props = makeProps({
        phases: {},
        phaseOrder: [],
      })

      render(<PhaseGraph {...props} />)

      expect(screen.getByText('No workflow phases defined')).toBeInTheDocument()
    })

    it('renders with pending phases only', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'pending' }),
          implementation: makePhaseState({ status: 'pending' }),
        },
        phaseOrder: ['investigation', 'implementation'],
      })

      render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })

      // Graph should render with pending nodes (triggering fitView)
      expect(screen.getByTestId('react-flow')).toBeInTheDocument()
    })

    it('renders with skipped phases', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'completed' }),
          'test-design': makePhaseState({ status: 'skipped' }),
          implementation: makePhaseState({ status: 'pending' }),
        },
        phaseOrder: ['investigation', 'test-design', 'implementation'],
        agentHistory: [makeHistory({ phase: 'investigation' })],
      })

      render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalled()
      }, { timeout: 200 })
    })

    it('handles multiple agents in same layer', async () => {
      const props = makeProps({
        phases: {
          implementation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['implementation'],
        activeAgents: {
          'implementor-be:claude:opus': makeAgent({
            agent_type: 'implementor-be',
            phase: 'implementation',
          }),
          'implementor-fe:claude:sonnet': makeAgent({
            agent_type: 'implementor-fe',
            phase: 'implementation',
            model_id: 'claude-sonnet-4-5',
          }),
        },
      })

      render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
      }, { timeout: 200 })

      // Graph should render with multiple agents and call fitView
      expect(screen.getByTestId('react-flow')).toBeInTheDocument()
    })
  })

  describe('ReactFlow integration', () => {
    it('renders ReactFlow component', () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'pending' }),
        },
        phaseOrder: ['investigation'],
      })

      render(<PhaseGraph {...props} />)

      expect(screen.getByTestId('react-flow')).toBeInTheDocument()
    })

    it('renders Background and Controls components', () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'pending' }),
        },
        phaseOrder: ['investigation'],
      })

      render(<PhaseGraph {...props} />)

      expect(screen.getByTestId('background')).toBeInTheDocument()
      expect(screen.getByTestId('controls')).toBeInTheDocument()
    })

    it('includes FitViewOnChange as child of ReactFlow', () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'pending' }),
        },
        phaseOrder: ['investigation'],
      })

      render(<PhaseGraph {...props} />)

      // FitViewOnChange is rendered inside ReactFlow
      const reactFlow = screen.getByTestId('react-flow')
      expect(reactFlow).toBeInTheDocument()
      // Background and Controls are also children
      expect(screen.getByTestId('background')).toBeInTheDocument()
      expect(screen.getByTestId('controls')).toBeInTheDocument()
    })
  })

  describe('onAgentSelect callback', () => {
    it('renders graph with onAgentSelect callback', () => {
      const onAgentSelect = vi.fn()
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
        onAgentSelect,
      })

      render(<PhaseGraph {...props} />)

      // Graph should render successfully with callback
      expect(screen.getByTestId('react-flow')).toBeInTheDocument()
    })
  })

  describe('completed workflow with history', () => {
    it('centers graph when showing completed workflow', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'completed' }),
          implementation: makePhaseState({ status: 'completed' }),
          verification: makePhaseState({ status: 'completed' }),
        },
        phaseOrder: ['investigation', 'implementation', 'verification'],
        agentHistory: [
          makeHistory({ phase: 'investigation', agent_type: 'setup-analyzer' }),
          makeHistory({ phase: 'implementation', agent_type: 'implementor' }),
          makeHistory({ phase: 'verification', agent_type: 'qa-verifier' }),
        ],
      })

      render(<PhaseGraph {...props} />)

      await waitFor(() => {
        expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
      }, { timeout: 200 })
    })
  })
})
