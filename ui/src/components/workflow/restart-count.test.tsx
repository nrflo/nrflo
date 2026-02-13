import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AgentFlowNode } from './PhaseGraph/AgentFlowNode'
import { AgentCard } from './PhaseGraph/AgentCard'
import { HistoryAgentCard } from './PhaseGraph/HistoryAgentCard'
import { ActiveAgentsPanel } from './ActiveAgentsPanel'
import { AgentHistoryCard } from './AgentHistoryCard'
import { AgentBadge } from './AgentBadge'
import type { ActiveAgentV4, AgentHistoryEntry } from '@/types/workflow'
import type { AgentFlowNodeData } from './PhaseGraph/types'

// Mock @xyflow/react Handle component (renders nothing)
vi.mock('@xyflow/react', () => ({
  Handle: () => null,
  Position: { Top: 'top', Bottom: 'bottom' },
}))

function makeActiveAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
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
    context_left: 60,
    restart_count: 0,
    restart_threshold: 25,
    ...overrides,
  }
}

function makeHistoryEntry(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'h1',
    agent_type: 'setup-analyzer',
    model_id: 'claude-sonnet-4-5',
    phase: 'investigation',
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T00:05:00Z',
    result: 'pass',
    duration_sec: 300,
    context_left: 45,
    restart_count: 0,
    restart_threshold: 25,
    ...overrides,
  }
}

describe('Restart Count Badge - All Components', () => {
  describe('AgentFlowNode', () => {
    it('hides restart count badge when restart_count is 0', () => {
      const data: AgentFlowNodeData = {
        agentKey: 'impl:claude:sonnet',
        phaseName: 'implementation',
        phaseIndex: 0,
        agent: makeActiveAgent({ restart_count: 0 }),
        onToggleExpand: vi.fn(),
      }
      const { container } = render(<AgentFlowNode data={data} />)
      expect(container.textContent).not.toMatch(/↻/)
    })

    it('hides restart count badge when restart_count is undefined', () => {
      const data: AgentFlowNodeData = {
        agentKey: 'impl:claude:sonnet',
        phaseName: 'implementation',
        phaseIndex: 0,
        agent: makeActiveAgent({ restart_count: undefined }),
        onToggleExpand: vi.fn(),
      }
      const { container } = render(<AgentFlowNode data={data} />)
      expect(container.textContent).not.toMatch(/↻/)
    })

    it('shows restart count badge when restart_count > 0', () => {
      const data: AgentFlowNodeData = {
        agentKey: 'impl:claude:sonnet',
        phaseName: 'implementation',
        phaseIndex: 0,
        agent: makeActiveAgent({ restart_count: 2 }),
        onToggleExpand: vi.fn(),
      }
      render(<AgentFlowNode data={data} />)
      expect(screen.getByText(/↻2/)).toBeInTheDocument()
    })

    it('shows restart count badge with red styling', () => {
      const data: AgentFlowNodeData = {
        agentKey: 'impl:claude:sonnet',
        phaseName: 'implementation',
        phaseIndex: 0,
        agent: makeActiveAgent({ restart_count: 3 }),
        onToggleExpand: vi.fn(),
      }
      const { container } = render(<AgentFlowNode data={data} />)
      const badge = container.querySelector('.bg-red-100')
      expect(badge).toBeInTheDocument()
      expect(badge?.textContent).toContain('↻3')
    })

    it('positions restart count badge at top-left corner', () => {
      const data: AgentFlowNodeData = {
        agentKey: 'impl:claude:sonnet',
        phaseName: 'implementation',
        phaseIndex: 0,
        agent: makeActiveAgent({ restart_count: 1 }),
        onToggleExpand: vi.fn(),
      }
      const { container } = render(<AgentFlowNode data={data} />)
      const badge = container.querySelector('.absolute.top-1.left-1')
      expect(badge).toBeInTheDocument()
      expect(badge?.textContent).toContain('↻1')
    })

    it('shows restart count for history entry when agent is undefined', () => {
      const data: AgentFlowNodeData = {
        agentKey: 'analyzer:claude:sonnet',
        phaseName: 'investigation',
        phaseIndex: 0,
        historyEntry: makeHistoryEntry({ restart_count: 2 }),
        onToggleExpand: vi.fn(),
      }
      render(<AgentFlowNode data={data} />)
      expect(screen.getByText(/↻2/)).toBeInTheDocument()
    })

    it('handles high restart counts', () => {
      const data: AgentFlowNodeData = {
        agentKey: 'impl:claude:sonnet',
        phaseName: 'implementation',
        phaseIndex: 0,
        agent: makeActiveAgent({ restart_count: 15 }),
        onToggleExpand: vi.fn(),
      }
      render(<AgentFlowNode data={data} />)
      expect(screen.getByText(/↻15/)).toBeInTheDocument()
    })
  })

  describe('AgentCard', () => {
    it('hides restart count badge when restart_count is 0', () => {
      const agent = makeActiveAgent({ restart_count: 0 })
      const { container } = render(<AgentCard agent={agent} />)
      expect(container.textContent).not.toMatch(/↻/)
    })

    it('hides restart count badge when restart_count is undefined', () => {
      const agent = makeActiveAgent({ restart_count: undefined })
      const { container } = render(<AgentCard agent={agent} />)
      expect(container.textContent).not.toMatch(/↻/)
    })

    it('shows restart count badge when restart_count > 0', () => {
      const agent = makeActiveAgent({ restart_count: 1 })
      render(<AgentCard agent={agent} />)
      expect(screen.getByText(/↻1/)).toBeInTheDocument()
    })

    it('shows restart count badge with red styling', () => {
      const agent = makeActiveAgent({ restart_count: 2 })
      const { container } = render(<AgentCard agent={agent} />)
      const badge = container.querySelector('.bg-red-100')
      expect(badge).toBeInTheDocument()
      expect(badge?.textContent).toContain('↻2')
    })

    it('positions restart count badge at top-left corner (absolute)', () => {
      const agent = makeActiveAgent({ restart_count: 1 })
      const { container } = render(<AgentCard agent={agent} />)
      const badge = container.querySelector('.absolute.top-1.left-1')
      expect(badge).toBeInTheDocument()
      expect(badge?.textContent).toContain('↻1')
    })
  })

  describe('HistoryAgentCard', () => {
    it('hides restart count when restart_count is 0', () => {
      const entry = makeHistoryEntry({ restart_count: 0 })
      const { container } = render(<HistoryAgentCard entry={entry} />)
      expect(container.textContent).not.toMatch(/↻/)
    })

    it('hides restart count when restart_count is undefined', () => {
      const entry = makeHistoryEntry({ restart_count: undefined })
      const { container } = render(<HistoryAgentCard entry={entry} />)
      expect(container.textContent).not.toMatch(/↻/)
    })

    it('shows restart count inline when restart_count > 0', () => {
      const entry = makeHistoryEntry({ restart_count: 2 })
      render(<HistoryAgentCard entry={entry} />)
      expect(screen.getByText(/↻2/)).toBeInTheDocument()
    })

    it('shows restart count with red styling', () => {
      const entry = makeHistoryEntry({ restart_count: 1 })
      const { container } = render(<HistoryAgentCard entry={entry} />)
      const badge = container.querySelector('.bg-red-100')
      expect(badge).toBeInTheDocument()
      expect(badge?.textContent).toContain('↻1')
    })

    it('displays restart count inline next to model name', () => {
      const entry = makeHistoryEntry({ restart_count: 3 })
      const { container } = render(<HistoryAgentCard entry={entry} />)
      // Restart count should be within the same parent as the model name
      const statusAndModel = container.querySelector('.flex.items-center.gap-1')
      expect(statusAndModel?.textContent).toContain('↻3')
    })
  })

  describe('ActiveAgentsPanel', () => {
    it('hides restart count when restart_count is 0', () => {
      const agents = {
        'impl:claude:sonnet': makeActiveAgent({ restart_count: 0, result: undefined }),
      }
      const { container } = render(<ActiveAgentsPanel agents={agents} />)
      expect(container.textContent).not.toMatch(/↻/)
    })

    it('hides restart count when restart_count is undefined', () => {
      const agents = {
        'impl:claude:sonnet': makeActiveAgent({ restart_count: undefined, result: undefined }),
      }
      const { container } = render(<ActiveAgentsPanel agents={agents} />)
      expect(container.textContent).not.toMatch(/↻/)
    })

    it('shows restart count inline when restart_count > 0', () => {
      const agents = {
        'impl:claude:sonnet': makeActiveAgent({ restart_count: 2, result: undefined }),
      }
      render(<ActiveAgentsPanel agents={agents} />)
      expect(screen.getByText(/↻2/)).toBeInTheDocument()
    })

    it('shows restart count with red styling', () => {
      const agents = {
        'impl:claude:sonnet': makeActiveAgent({ restart_count: 1, result: undefined }),
      }
      const { container } = render(<ActiveAgentsPanel agents={agents} />)
      const badge = container.querySelector('.bg-red-100')
      expect(badge).toBeInTheDocument()
      expect(badge?.textContent).toContain('↻1')
    })

    it('displays restart count inline next to agent_type', () => {
      const agents = {
        'impl:claude:sonnet': makeActiveAgent({ restart_count: 4, result: undefined }),
      }
      render(<ActiveAgentsPanel agents={agents} />)
      const agentType = screen.getByText('implementor')
      const parent = agentType.parentElement
      expect(parent?.textContent).toContain('↻4')
    })

    it('shows restart count for multiple agents independently', () => {
      const agents = {
        'impl:claude:sonnet': makeActiveAgent({
          agent_id: 'a1',
          agent_type: 'implementor',
          restart_count: 2,
          result: undefined,
        }),
        'tester:claude:opus': makeActiveAgent({
          agent_id: 'a2',
          agent_type: 'tester',
          restart_count: 1,
          result: undefined,
        }),
      }
      render(<ActiveAgentsPanel agents={agents} />)
      expect(screen.getByText(/↻2/)).toBeInTheDocument()
      expect(screen.getByText(/↻1/)).toBeInTheDocument()
    })
  })

  describe('AgentHistoryCard', () => {
    it('hides restart count when restart_count is 0', () => {
      const agent = makeHistoryEntry({ restart_count: 0 })
      const { container } = render(<AgentHistoryCard agent={agent} />)
      expect(container.textContent).not.toMatch(/↻/)
    })

    it('hides restart count when restart_count is undefined', () => {
      const agent = makeHistoryEntry({ restart_count: undefined })
      const { container } = render(<AgentHistoryCard agent={agent} />)
      expect(container.textContent).not.toMatch(/↻/)
    })

    it('shows restart count inline when restart_count > 0', () => {
      const agent = makeHistoryEntry({ restart_count: 2 })
      render(<AgentHistoryCard agent={agent} />)
      expect(screen.getByText(/↻2/)).toBeInTheDocument()
    })

    it('shows restart count with red styling', () => {
      const agent = makeHistoryEntry({ restart_count: 1 })
      const { container } = render(<AgentHistoryCard agent={agent} />)
      const badge = container.querySelector('.bg-red-100')
      expect(badge).toBeInTheDocument()
      expect(badge?.textContent).toContain('↻1')
    })

    it('displays restart count after model badge', () => {
      const agent = makeHistoryEntry({ restart_count: 3 })
      render(<AgentHistoryCard agent={agent} />)
      // Restart count should appear after agent_type and model_id
      const agentType = screen.getByText('setup-analyzer')
      const parent = agentType.parentElement
      expect(parent?.textContent).toContain('↻3')
    })
  })

  describe('AgentBadge', () => {
    it('hides restart count when restart_count is 0', () => {
      const agent = makeHistoryEntry({ restart_count: 0 })
      const { container } = render(<AgentBadge agent={agent} expanded={false} onToggle={vi.fn()} />)
      expect(container.textContent).not.toMatch(/↻/)
    })

    it('hides restart count when restart_count is undefined', () => {
      const agent = makeHistoryEntry({ restart_count: undefined })
      const { container } = render(<AgentBadge agent={agent} expanded={false} onToggle={vi.fn()} />)
      expect(container.textContent).not.toMatch(/↻/)
    })

    it('shows restart count inline when restart_count > 0', () => {
      const agent = makeHistoryEntry({ restart_count: 2 })
      render(<AgentBadge agent={agent} expanded={false} onToggle={vi.fn()} />)
      expect(screen.getByText(/↻2/)).toBeInTheDocument()
    })

    it('shows restart count with red text color', () => {
      const agent = makeHistoryEntry({ restart_count: 1 })
      const { container } = render(<AgentBadge agent={agent} expanded={false} onToggle={vi.fn()} />)
      const restartIndicator = container.querySelector('.text-red-600')
      expect(restartIndicator).toBeInTheDocument()
      expect(restartIndicator?.textContent).toContain('↻1')
    })

    it('displays restart count inline within the badge', () => {
      const agent = makeHistoryEntry({ restart_count: 4 })
      render(<AgentBadge agent={agent} expanded={false} onToggle={vi.fn()} />)
      // Badge is a single inline element, restart count should be inside
      const badge = screen.getByRole('button')
      expect(badge.textContent).toContain('setup-analyzer')
      expect(badge.textContent).toContain('↻4')
    })
  })

  describe('Cross-component consistency', () => {
    it('all components use the same restart indicator symbol ↻', () => {
      // Test that all components use the circular arrow symbol
      const activeAgent = makeActiveAgent({ restart_count: 1, result: undefined })
      const historyEntry = makeHistoryEntry({ restart_count: 1 })

      const { container: c1 } = render(<AgentFlowNode data={{ agentKey: 'test:claude:sonnet', phaseName: 'test', phaseIndex: 0, agent: activeAgent, onToggleExpand: vi.fn() }} />)
      const { container: c2 } = render(<AgentCard agent={activeAgent} />)
      const { container: c3 } = render(<HistoryAgentCard entry={historyEntry} />)
      const { container: c4 } = render(<ActiveAgentsPanel agents={{ 'a:c:s': activeAgent }} />)
      const { container: c5 } = render(<AgentHistoryCard agent={historyEntry} />)
      const { container: c6 } = render(<AgentBadge agent={historyEntry} expanded={false} onToggle={vi.fn()} />)

      expect(c1.textContent).toContain('↻1')
      expect(c2.textContent).toContain('↻1')
      expect(c3.textContent).toContain('↻1')
      expect(c4.textContent).toContain('↻1')
      expect(c5.textContent).toContain('↻1')
      expect(c6.textContent).toContain('↻1')
    })

    it('all components show red color for restart count', () => {
      const activeAgent = makeActiveAgent({ restart_count: 1, result: undefined })
      const historyEntry = makeHistoryEntry({ restart_count: 1 })

      const { container: c1 } = render(<AgentFlowNode data={{ agentKey: 'test:claude:sonnet', phaseName: 'test', phaseIndex: 0, agent: activeAgent, onToggleExpand: vi.fn() }} />)
      const { container: c2 } = render(<AgentCard agent={activeAgent} />)
      const { container: c3 } = render(<HistoryAgentCard entry={historyEntry} />)
      const { container: c4 } = render(<ActiveAgentsPanel agents={{ 'a:c:s': activeAgent }} />)
      const { container: c5 } = render(<AgentHistoryCard agent={historyEntry} />)
      const { container: c6 } = render(<AgentBadge agent={historyEntry} expanded={false} onToggle={vi.fn()} />)

      // All should have red styling (either bg-red or text-red)
      expect(c1.querySelector('.bg-red-100, .text-red-600, .text-red-700, .text-red-400')).toBeInTheDocument()
      expect(c2.querySelector('.bg-red-100, .text-red-600, .text-red-700, .text-red-400')).toBeInTheDocument()
      expect(c3.querySelector('.bg-red-100, .text-red-600, .text-red-700, .text-red-400')).toBeInTheDocument()
      expect(c4.querySelector('.bg-red-100, .text-red-600, .text-red-700, .text-red-400')).toBeInTheDocument()
      expect(c5.querySelector('.bg-red-100, .text-red-600, .text-red-700, .text-red-400')).toBeInTheDocument()
      expect(c6.querySelector('.bg-red-100, .text-red-600, .text-red-700, .text-red-400')).toBeInTheDocument()
    })

    it('all components hide restart count when 0', () => {
      const activeAgent = makeActiveAgent({ restart_count: 0, result: undefined })
      const historyEntry = makeHistoryEntry({ restart_count: 0 })

      const { container: c1 } = render(<AgentFlowNode data={{ agentKey: 'test:claude:sonnet', phaseName: 'test', phaseIndex: 0, agent: activeAgent, onToggleExpand: vi.fn() }} />)
      const { container: c2 } = render(<AgentCard agent={activeAgent} />)
      const { container: c3 } = render(<HistoryAgentCard entry={historyEntry} />)
      const { container: c4 } = render(<ActiveAgentsPanel agents={{ 'a:c:s': activeAgent }} />)
      const { container: c5 } = render(<AgentHistoryCard agent={historyEntry} />)
      const { container: c6 } = render(<AgentBadge agent={historyEntry} expanded={false} onToggle={vi.fn()} />)

      expect(c1.textContent).not.toMatch(/↻/)
      expect(c2.textContent).not.toMatch(/↻/)
      expect(c3.textContent).not.toMatch(/↻/)
      expect(c4.textContent).not.toMatch(/↻/)
      expect(c5.textContent).not.toMatch(/↻/)
      expect(c6.textContent).not.toMatch(/↻/)
    })
  })

  describe('Badge positioning and layout', () => {
    it('AgentFlowNode: restart count badge does not overlap context badge', () => {
      const data: AgentFlowNodeData = {
        agentKey: 'impl:claude:sonnet',
        phaseName: 'implementation',
        phaseIndex: 0,
        agent: makeActiveAgent({ restart_count: 2, context_left: 30 }),
        onToggleExpand: vi.fn(),
      }
      const { container } = render(<AgentFlowNode data={data} />)

      const restartBadge = container.querySelector('.absolute.top-1.left-1')
      const contextBadge = container.querySelector('.absolute.top-1.right-1')

      expect(restartBadge).toBeInTheDocument()
      expect(contextBadge).toBeInTheDocument()
      expect(restartBadge?.textContent).toContain('↻2')
      expect(contextBadge?.textContent).toContain('30%')
    })

    it('AgentCard: restart count badge does not overlap context badge', () => {
      const agent = makeActiveAgent({ restart_count: 1, context_left: 45 })
      const { container } = render(<AgentCard agent={agent} />)

      const restartBadge = container.querySelector('.absolute.top-1.left-1')
      const contextBadge = container.querySelector('.absolute.top-1.right-1')

      expect(restartBadge).toBeInTheDocument()
      expect(contextBadge).toBeInTheDocument()
      expect(restartBadge?.textContent).toContain('↻1')
      expect(contextBadge?.textContent).toContain('45%')
    })
  })

  describe('Edge cases', () => {
    it('handles restart_count = 1 (first restart)', () => {
      const data: AgentFlowNodeData = {
        agentKey: 'impl:claude:sonnet',
        phaseName: 'implementation',
        phaseIndex: 0,
        agent: makeActiveAgent({ restart_count: 1 }),
        onToggleExpand: vi.fn(),
      }
      render(<AgentFlowNode data={data} />)
      expect(screen.getByText(/↻1/)).toBeInTheDocument()
    })

    it('handles very high restart counts', () => {
      const data: AgentFlowNodeData = {
        agentKey: 'impl:claude:sonnet',
        phaseName: 'implementation',
        phaseIndex: 0,
        agent: makeActiveAgent({ restart_count: 99 }),
        onToggleExpand: vi.fn(),
      }
      render(<AgentFlowNode data={data} />)
      expect(screen.getByText(/↻99/)).toBeInTheDocument()
    })

    it('handles null restart_count as undefined (no badge)', () => {
      const agent = makeActiveAgent({ restart_count: null as any })
      const { container } = render(<AgentCard agent={agent} />)
      expect(container.textContent).not.toMatch(/↻/)
    })
  })
})
