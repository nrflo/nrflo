import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { AgentLogPanel } from './AgentLogPanel'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'

vi.mock('@/hooks/useTickets', () => ({
  useSessionMessages: vi.fn(() => ({ data: { messages: [] } })),
}))

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
    project_id: 'p1',
    ticket_id: 't1',
    workflow_instance_id: 'wi1',
    workflow: 'feature',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    status: 'running',
    message_count: 0,
    restart_count: 0,
    started_at: '2026-01-01T00:00:00Z',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    last_messages: [],
    ...overrides,
  }
}

const selectedAgent = {
  phaseName: 'implementation',
  agent: makeAgent(),
  session: makeSession(),
}

const runningAgent = makeAgent({ result: undefined })

describe('AgentLogPanel - Mobile Layout (nrworkflow-395fca)', () => {
  describe('mobile border (border-t on mobile, border-l on desktop)', () => {
    it('detail mode: has border-t for mobile top border', () => {
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
      expect(panel.className).toContain('border-t')
    })

    it('detail mode: has md:border-l for desktop left border', () => {
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
      expect(panel.className).toContain('md:border-l')
    })

    it('detail mode: has md:border-t-0 to cancel mobile top border on desktop', () => {
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
      expect(panel.className).toContain('md:border-t-0')
    })

    it('overview mode: has border-t and md:border-l', () => {
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
      expect(panel.className).toContain('border-t')
      expect(panel.className).toContain('md:border-l')
    })
  })

  describe('mobile expanded height (h-[50vh] cap)', () => {
    it('detail mode expanded: has h-[50vh] for mobile height cap', () => {
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
      expect(panel.className).toContain('h-[50vh]')
    })

    it('detail mode expanded: has md:h-auto for flexible desktop height', () => {
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
      expect(panel.className).toContain('md:h-auto')
    })

    it('overview mode expanded: has h-[50vh]', () => {
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
      expect(panel.className).toContain('h-[50vh]')
    })
  })

  describe('mobile collapsed sizing (h-10 height, md:w-10 desktop width)', () => {
    it('detail mode collapsed: has h-10 for mobile collapsed height', () => {
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
      expect(panel.className).toContain('h-10')
    })

    it('detail mode collapsed: has md:w-10 for desktop collapsed width', () => {
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
      expect(panel.className).toContain('md:w-10')
    })

    it('overview mode collapsed: has h-10 and md:w-10', () => {
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
      expect(panel.className).toContain('h-10')
      expect(panel.className).toContain('md:w-10')
    })
  })

  // TODO(test-writer): collapse button positioning tests removed — button moved to WorkflowTabContent/ProjectWorkflowsPage headers

  describe('collapsed content layout (horizontal mobile, vertical desktop)', () => {
    it('detail mode collapsed: inner content has flex-row for mobile horizontal layout', () => {
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
      // Find the inner collapsed content div (not the panel itself)
      const panel = container.firstChild as HTMLElement
      const innerContent = panel.querySelector('div') as HTMLElement
      expect(innerContent.className).toContain('flex-row')
    })

    it('detail mode collapsed: inner content has md:flex-col for desktop vertical layout', () => {
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
      const innerContent = panel.querySelector('div') as HTMLElement
      expect(innerContent.className).toContain('md:flex-col')
    })

    it('overview mode collapsed: inner content has flex-row and md:flex-col', () => {
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
      const innerContent = panel.querySelector('div') as HTMLElement
      expect(innerContent.className).toContain('flex-row')
      expect(innerContent.className).toContain('md:flex-col')
    })
  })

  describe('writing-mode gated behind md: prefix', () => {
    it('detail mode collapsed: Agent Log text has md:[writing-mode:vertical-lr]', () => {
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
      const span = container.querySelector('span') as HTMLElement
      expect(span.className).toContain('md:[writing-mode:vertical-lr]')
    })

    it('overview mode collapsed: Agent Log text has md:[writing-mode:vertical-lr]', () => {
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
      // Find the "Agent Log" span (last span)
      const spans = container.querySelectorAll('span')
      const agentLogSpan = Array.from(spans).find(s => s.textContent === 'Agent Log')
      expect(agentLogSpan).toBeTruthy()
      expect(agentLogSpan!.className).toContain('md:[writing-mode:vertical-lr]')
    })
  })
})
