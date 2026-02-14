import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { AgentLogPanel } from './AgentLogPanel'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'

// Mock hooks
vi.mock('@/hooks/useTickets', () => ({
  useSessionMessages: vi.fn(() => ({
    data: { messages: [] },
  })),
}))

// Mock AgentLogDetail to avoid QueryClient dependency
vi.mock('./AgentLogDetail', () => ({
  AgentLogDetail: () => <div data-testid="agent-log-detail">Detail</div>,
}))

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

function makeSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 's1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    status: 'running',
    started_at: '2026-01-01T00:00:00Z',
    last_messages: [],
    ...overrides,
  }
}

describe('AgentLogPanel - Width Reduction (nrworkflow-28182f)', () => {
  describe('expanded panel width (detail mode)', () => {
    it('uses w-[280px] for expanded detail mode', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const { container } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      // Find the panel container (first child of the rendered component)
      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('w-[280px]')
    })

    it('does not use flex-1 in expanded detail mode', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const { container } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).not.toContain('flex-1')
    })

    it('does not use min-w-[300px] in expanded detail mode', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const { container } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).not.toContain('min-w-[300px]')
    })

    it('expanded detail panel is narrower than old flex-1 approach', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const { container } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      // Fixed width of 280px is narrower than old flex-1 min-w-[300px]
      expect(panel.className).toContain('w-[280px]')
      // Old: flex-1 min-w-[300px] would grow to fill available space
      // New: w-[280px] is fixed at 280px (~20% smaller than 300px minimum)
    })
  })

  describe('expanded panel width (overview mode)', () => {
    it('uses w-[280px] for expanded overview mode', () => {
      const runningAgent = makeAgent({ result: undefined }) // No result = running

      const { container } = render(
        <AgentLogPanel
          activeAgents={{ 'implementor:claude:sonnet': runningAgent }}
          sessions={[makeSession()]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={null}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('w-[280px]')
    })

    it('does not use flex-1 in expanded overview mode', () => {
      const runningAgent = makeAgent({ result: undefined })

      const { container } = render(
        <AgentLogPanel
          activeAgents={{ 'implementor:claude:sonnet': runningAgent }}
          sessions={[makeSession()]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={null}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).not.toContain('flex-1')
    })

    it('does not use min-w-[300px] in expanded overview mode', () => {
      const runningAgent = makeAgent({ result: undefined })

      const { container } = render(
        <AgentLogPanel
          activeAgents={{ 'implementor:claude:sonnet': runningAgent }}
          sessions={[makeSession()]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={null}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).not.toContain('min-w-[300px]')
    })
  })

  describe('collapsed panel width (both modes)', () => {
    it('uses w-10 for collapsed detail mode', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const { container } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={true}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('w-10')
    })

    it('uses w-10 for collapsed overview mode', () => {
      const runningAgent = makeAgent({ result: undefined })

      const { container } = render(
        <AgentLogPanel
          activeAgents={{ 'implementor:claude:sonnet': runningAgent }}
          sessions={[makeSession()]}
          collapsed={true}
          onToggleCollapse={vi.fn()}
          selectedAgent={null}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('w-10')
    })
  })

  describe('width consistency across modes', () => {
    it('uses same expanded width (w-[280px]) in both detail and overview modes', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const runningAgent = makeAgent({ result: undefined })

      // Detail mode
      const { container: detailContainer } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      // Overview mode
      const { container: overviewContainer } = render(
        <AgentLogPanel
          activeAgents={{ 'implementor:claude:sonnet': runningAgent }}
          sessions={[makeSession()]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={null}
          onAgentSelect={vi.fn()}
        />
      )

      const detailPanel = detailContainer.firstChild as HTMLElement
      const overviewPanel = overviewContainer.firstChild as HTMLElement

      expect(detailPanel.className).toContain('w-[280px]')
      expect(overviewPanel.className).toContain('w-[280px]')
    })

    it('uses same collapsed width (w-10) in both detail and overview modes', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const runningAgent = makeAgent({ result: undefined })

      // Detail mode
      const { container: detailContainer } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={true}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      // Overview mode
      const { container: overviewContainer } = render(
        <AgentLogPanel
          activeAgents={{ 'implementor:claude:sonnet': runningAgent }}
          sessions={[makeSession()]}
          collapsed={true}
          onToggleCollapse={vi.fn()}
          selectedAgent={null}
          onAgentSelect={vi.fn()}
        />
      )

      const detailPanel = detailContainer.firstChild as HTMLElement
      const overviewPanel = overviewContainer.firstChild as HTMLElement

      expect(detailPanel.className).toContain('w-10')
      expect(overviewPanel.className).toContain('w-10')
    })
  })

  describe('transition classes', () => {
    it('has transition-all duration-300 for smooth animation', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const { container } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('transition-all')
      expect(panel.className).toContain('duration-300')
    })

    it('maintains transition classes in overview mode', () => {
      const runningAgent = makeAgent({ result: undefined })

      const { container } = render(
        <AgentLogPanel
          activeAgents={{ 'implementor:claude:sonnet': runningAgent }}
          sessions={[makeSession()]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={null}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('transition-all')
      expect(panel.className).toContain('duration-300')
    })

    it('has ease-in-out timing function', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const { container } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('ease-in-out')
    })
  })

  describe('shrink-0 class', () => {
    it('has shrink-0 to prevent flexbox shrinking', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const { container } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('shrink-0')
    })

    it('maintains shrink-0 in overview mode', () => {
      const runningAgent = makeAgent({ result: undefined })

      const { container } = render(
        <AgentLogPanel
          activeAgents={{ 'implementor:claude:sonnet': runningAgent }}
          sessions={[makeSession()]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={null}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('shrink-0')
    })
  })

  describe('visual regression - old classes removed', () => {
    it('detail mode does not have old flex-1 min-w-[300px] classes', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const { container } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      // These are the OLD classes that should be removed
      expect(panel.className).not.toContain('flex-1')
      expect(panel.className).not.toContain('min-w-[300px]')
      // Should have NEW class
      expect(panel.className).toContain('w-[280px]')
    })

    it('overview mode does not have old flex-1 min-w-[300px] classes', () => {
      const runningAgent = makeAgent({ result: undefined })

      const { container } = render(
        <AgentLogPanel
          activeAgents={{ 'implementor:claude:sonnet': runningAgent }}
          sessions={[makeSession()]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={null}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).not.toContain('flex-1')
      expect(panel.className).not.toContain('min-w-[300px]')
      expect(panel.className).toContain('w-[280px]')
    })
  })

  describe('full flow - width changes across states', () => {
    it('full flow: panel width changes from collapsed (w-10) to expanded (w-[280px])', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const { container, rerender } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={true}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('w-10')

      // Expand panel
      rerender(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      expect(panel.className).toContain('w-[280px]')
      expect(panel.className).not.toContain('w-10')
    })

    it('full flow: panel maintains w-[280px] when switching from detail to overview mode', () => {
      const selectedAgent = {
        phaseName: 'implementation',
        agent: makeAgent(),
        session: makeSession(),
      }

      const runningAgent = makeAgent({ result: undefined })

      const { container, rerender } = render(
        <AgentLogPanel
          activeAgents={{}}
          sessions={[]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={selectedAgent}
          onAgentSelect={vi.fn()}
        />
      )

      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('w-[280px]')

      // Switch to overview mode (deselect agent, add running agent)
      rerender(
        <AgentLogPanel
          activeAgents={{ 'implementor:claude:sonnet': runningAgent }}
          sessions={[makeSession()]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          selectedAgent={null}
          onAgentSelect={vi.fn()}
        />
      )

      // Width should remain w-[280px]
      expect(panel.className).toContain('w-[280px]')
    })
  })
})
