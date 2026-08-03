/**
 * Model-label tests for AgentLogPanel's multi-agent tab bar.
 *
 * Covers the de-truncation change: tabs show the full model slug (only the
 * cli: prefix is stripped) instead of the last two hyphen segments, and the
 * raw model_id is exposed via a title attribute for a tooltip.
 */
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AgentLogPanel } from './AgentLogPanel'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

vi.mock('@/hooks/useTickets', () => ({
  useSessionMessages: vi.fn(() => ({ data: { messages: [] } })),
}))

vi.mock('./AgentLogDetail', () => ({
  AgentLogDetail: () => <div data-testid="agent-log-detail" />,
}))

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude:opus-5-1m',
    pid: 12345,
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderPanel(props: Partial<React.ComponentProps<typeof AgentLogPanel>> = {}) {
  const defaultProps = {
    activeAgents: {} as Record<string, ActiveAgentV4>,
    sessions: [] as AgentSession[],
    collapsed: false,
    selectedAgent: null as SelectedAgentData | null,
    onAgentSelect: vi.fn(),
  }
  return render(<AgentLogPanel {...defaultProps} {...props} />)
}

describe('AgentLogPanel - tab model label', () => {
  it('shows the full model slug without the last-two-segments truncation', () => {
    renderPanel({
      activeAgents: { 'a1': makeAgent({ phase: 'implement', model_id: 'claude:opus-5-1m' }) },
      sessions: [],
    })
    const tab = screen.getByTestId('agent-tab')
    expect(tab).toHaveTextContent('implement : opus-5-1m')
    expect(tab.textContent).not.toContain('5-1m ')
  })

  it('exposes the raw model_id via a title attribute', () => {
    renderPanel({
      activeAgents: { 'a1': makeAgent({ phase: 'implement', model_id: 'claude:opus-5-1m' }) },
      sessions: [],
    })
    const tab = screen.getByTestId('agent-tab')
    expect(tab).toHaveAttribute('title', 'claude:opus-5-1m')
  })

  it('shows only the phase name when model_id is absent', () => {
    renderPanel({
      activeAgents: { 'a1': makeAgent({ phase: 'implement', model_id: undefined }) },
      sessions: [],
    })
    const tab = screen.getByTestId('agent-tab')
    expect(tab.textContent?.trim()).toBe('implement')
  })
})
