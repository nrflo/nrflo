import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { PhaseGraph } from './PhaseGraph'
import type { PhaseGraphProps } from './types'
import type { PhaseState, ActiveAgentV4, AgentHistoryEntry } from '@/types/workflow'

const mockFitView = vi.fn()

vi.mock('@xyflow/react', async () => {
  const actual = await vi.importActual('@xyflow/react')
  return {
    ...actual,
    ReactFlow: ({ children }: { children: React.ReactNode }) => (
      <div data-testid="react-flow">{children}</div>
    ),
    Background: () => <div data-testid="background" />,
    Controls: () => <div data-testid="controls" />,
    useReactFlow: () => ({ fitView: mockFitView }),
  }
})

vi.mock('./layout', () => ({
  getLayoutedElements: (nodes: any[], edges: any[], _expanded: any, _isMobile?: boolean) => Promise.resolve({ nodes, edges }),
  BASE_HEIGHT: 110,
}))

vi.mock('./AgentFlowNode', () => ({
  AgentFlowNode: ({ data }: { data: any }) => (
    <div data-testid={`agent-node-${data.agentKey}`}>{data.phaseName}</div>
  ),
}))

vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => false,
}))

function makePhaseState(overrides: Partial<PhaseState> = {}): PhaseState {
  return { status: 'pending', ...overrides }
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

/** Flush pending microtasks (async layout Promise) */
async function flushLayout() {
  await act(async () => {})
}

describe('PhaseGraph', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  describe('FitViewOnChange', () => {
    it('calls fitView with correct options after 200ms on mount', async () => {
      const props = makeProps({
        phases: { investigation: makePhaseState({ status: 'in_progress' }) },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation' }),
        },
      })

      render(<PhaseGraph {...props} />)
      await flushLayout()

      expect(mockFitView).not.toHaveBeenCalled()
      act(() => { vi.advanceTimersByTime(200) })
      expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
    })

    it('calls fitView again when nodes change (workflow starts)', async () => {
      const props = makeProps({
        phases: { investigation: makePhaseState({ status: 'pending' }) },
        phaseOrder: ['investigation'],
      })

      const { rerender } = render(<PhaseGraph {...props} />)
      await flushLayout()
      act(() => { vi.advanceTimersByTime(200) })
      vi.clearAllMocks()

      rerender(<PhaseGraph {...makeProps({
        phases: { investigation: makePhaseState({ status: 'in_progress' }) },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation' }),
        },
      })} />)
      await flushLayout()

      act(() => { vi.advanceTimersByTime(200) })
      expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
    })

    it('calls fitView when phase transitions to next phase', async () => {
      const props = makeProps({
        phases: {
          investigation: makePhaseState({ status: 'in_progress' }),
          implementation: makePhaseState({ status: 'pending' }),
        },
        phaseOrder: ['investigation', 'implementation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation' }),
        },
      })

      const { rerender } = render(<PhaseGraph {...props} />)
      await flushLayout()
      act(() => { vi.advanceTimersByTime(200) })
      vi.clearAllMocks()

      rerender(<PhaseGraph {...makeProps({
        phases: {
          investigation: makePhaseState({ status: 'completed' }),
          implementation: makePhaseState({ status: 'in_progress' }),
        },
        phaseOrder: ['investigation', 'implementation'],
        activeAgents: {
          'implementor:claude:opus': makeAgent({ agent_type: 'implementor', phase: 'implementation' }),
        },
        agentHistory: [makeHistory({ phase: 'investigation', agent_type: 'setup-analyzer' })],
      })} />)
      await flushLayout()

      act(() => { vi.advanceTimersByTime(200) })
      expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
    })

    it('does not call fitView when nodes remain the same', async () => {
      const props = makeProps({
        phases: { investigation: makePhaseState({ status: 'in_progress' }) },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer', phase: 'investigation', context_left: 80,
          }),
        },
      })

      const { rerender } = render(<PhaseGraph {...props} />)
      await flushLayout()
      act(() => { vi.advanceTimersByTime(200) })
      const initialCallCount = mockFitView.mock.calls.length

      // Update context_left (doesn't change nodeKey)
      rerender(<PhaseGraph {...makeProps({
        phases: { investigation: makePhaseState({ status: 'in_progress' }) },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({
            agent_type: 'setup-analyzer', phase: 'investigation', context_left: 75,
          }),
        },
      })} />)
      await flushLayout()

      act(() => { vi.advanceTimersByTime(100) })
      expect(mockFitView).toHaveBeenCalledTimes(initialCallCount)
    })
  })

  describe('nodeKey stability', () => {
    it('changes nodeKey when node count changes', async () => {
      const props = makeProps({
        phases: { investigation: makePhaseState({ status: 'in_progress' }) },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation' }),
        },
      })

      const { rerender } = render(<PhaseGraph {...props} />)
      await flushLayout()
      act(() => { vi.advanceTimersByTime(200) })
      vi.clearAllMocks()

      rerender(<PhaseGraph {...makeProps({
        phases: { investigation: makePhaseState({ status: 'in_progress' }) },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation' }),
          'setup-analyzer:claude:haiku': makeAgent({
            agent_type: 'setup-analyzer', phase: 'investigation', model_id: 'claude-haiku-4-5',
          }),
        },
      })} />)
      await flushLayout()

      act(() => { vi.advanceTimersByTime(200) })
      expect(mockFitView).toHaveBeenCalled()
    })
  })

  describe('edge cases', () => {
    it('shows empty state when no phases exist', async () => {
      render(<PhaseGraph {...makeProps({ phases: {}, phaseOrder: [] })} />)
      await flushLayout()
      expect(screen.getByText('No workflow phases defined')).toBeInTheDocument()
      expect(screen.queryByTestId('react-flow')).not.toBeInTheDocument()
    })

    it('renders with pending phases only', async () => {
      render(<PhaseGraph {...makeProps({
        phases: {
          investigation: makePhaseState({ status: 'pending' }),
          implementation: makePhaseState({ status: 'pending' }),
        },
        phaseOrder: ['investigation', 'implementation'],
      })} />)
      await flushLayout()

      act(() => { vi.advanceTimersByTime(200) })
      expect(mockFitView).toHaveBeenCalled()
      expect(screen.getByTestId('react-flow')).toBeInTheDocument()
    })

    it('renders with skipped phases', async () => {
      render(<PhaseGraph {...makeProps({
        phases: {
          investigation: makePhaseState({ status: 'completed' }),
          'test-design': makePhaseState({ status: 'skipped' }),
          implementation: makePhaseState({ status: 'pending' }),
        },
        phaseOrder: ['investigation', 'test-design', 'implementation'],
        agentHistory: [makeHistory({ phase: 'investigation' })],
      })} />)
      await flushLayout()

      act(() => { vi.advanceTimersByTime(200) })
      expect(mockFitView).toHaveBeenCalled()
    })

    it('handles multiple agents in same layer', async () => {
      render(<PhaseGraph {...makeProps({
        phases: { implementation: makePhaseState({ status: 'in_progress' }) },
        phaseOrder: ['implementation'],
        activeAgents: {
          'implementor-be:claude:opus': makeAgent({ agent_type: 'implementor-be', phase: 'implementation' }),
          'implementor-fe:claude:sonnet': makeAgent({
            agent_type: 'implementor-fe', phase: 'implementation', model_id: 'claude-sonnet-4-5',
          }),
        },
      })} />)
      await flushLayout()

      act(() => { vi.advanceTimersByTime(200) })
      expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
      expect(screen.getByTestId('react-flow')).toBeInTheDocument()
    })
  })

  describe('ReactFlow integration', () => {
    it('renders ReactFlow with Background and Controls', async () => {
      render(<PhaseGraph {...makeProps({
        phases: { investigation: makePhaseState({ status: 'pending' }) },
        phaseOrder: ['investigation'],
      })} />)
      await flushLayout()

      expect(screen.getByTestId('react-flow')).toBeInTheDocument()
      expect(screen.getByTestId('background')).toBeInTheDocument()
      expect(screen.getByTestId('controls')).toBeInTheDocument()
    })
  })

  describe('onAgentSelect callback', () => {
    it('renders graph with onAgentSelect callback', async () => {
      render(<PhaseGraph {...makeProps({
        phases: { investigation: makePhaseState({ status: 'in_progress' }) },
        phaseOrder: ['investigation'],
        activeAgents: {
          'setup-analyzer:claude:sonnet': makeAgent({ agent_type: 'setup-analyzer', phase: 'investigation' }),
        },
        onAgentSelect: vi.fn(),
      })} />)
      await flushLayout()

      expect(screen.getByTestId('react-flow')).toBeInTheDocument()
    })
  })

  describe('completed workflow with history', () => {
    it('centers graph when showing completed workflow', async () => {
      render(<PhaseGraph {...makeProps({
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
      })} />)
      await flushLayout()

      act(() => { vi.advanceTimersByTime(200) })
      expect(mockFitView).toHaveBeenCalledWith({ padding: 0.3, duration: 200 })
    })
  })
})
