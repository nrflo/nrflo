import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AgentCard } from './AgentCard'
import { ActiveAgentsPanel } from '../ActiveAgentsPanel'
import type { AgentCardProps } from './types'
import type { ActiveAgentV4 } from '@/types/workflow'

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    pid: 12345,
    started_at: '2026-01-01T00:00:00Z',
    context_left: 50,
    restart_threshold: 25,
    ...overrides,
  }
}

function renderCard(agent: ActiveAgentV4) {
  const props: AgentCardProps = { agent, session: undefined, onExpand: vi.fn(), isExpanded: false }
  return render(<AgentCard {...props} />)
}

function hasWarningIcon(container: HTMLElement): boolean {
  return container.querySelector('svg.h-4.w-4.text-amber-500') !== null
}

describe('AgentCard - restart threshold warning', () => {
  it('shows warning when context_left <= threshold+15', () => {
    const { container } = renderCard(makeAgent({ context_left: 40, restart_threshold: 25, result: undefined }))
    expect(hasWarningIcon(container)).toBe(true)
  })

  it('hides warning when context_left > threshold+15', () => {
    const { container } = renderCard(makeAgent({ context_left: 41, restart_threshold: 25, result: undefined }))
    expect(hasWarningIcon(container)).toBe(false)
  })

  it('hides warning for completed agents even if context near threshold', () => {
    const { container: pass } = renderCard(makeAgent({ context_left: 30, result: 'pass' }))
    const { container: fail } = renderCard(makeAgent({ context_left: 20, result: 'fail' }))
    expect(hasWarningIcon(pass)).toBe(false)
    expect(hasWarningIcon(fail)).toBe(false)
  })

  it('uses custom restart_threshold', () => {
    const { container: warn } = renderCard(makeAgent({ context_left: 60, restart_threshold: 50, result: undefined }))
    const { container: ok } = renderCard(makeAgent({ context_left: 66, restart_threshold: 50, result: undefined }))
    expect(hasWarningIcon(warn)).toBe(true)
    expect(hasWarningIcon(ok)).toBe(false)
  })

  it('falls back to default threshold 25 when restart_threshold is undefined', () => {
    const { container: warn } = renderCard(makeAgent({ context_left: 40, restart_threshold: undefined, result: undefined }))
    const { container: ok } = renderCard(makeAgent({ context_left: 41, restart_threshold: undefined, result: undefined }))
    expect(hasWarningIcon(warn)).toBe(true)
    expect(hasWarningIcon(ok)).toBe(false)
  })

  it('handles undefined/null context_left without warning', () => {
    const { container: undef } = renderCard(makeAgent({ context_left: undefined, result: undefined }))
    const { container: nul } = renderCard(makeAgent({ context_left: null as any, result: undefined }))
    expect(hasWarningIcon(undef)).toBe(false)
    expect(hasWarningIcon(nul)).toBe(false)
  })

  it('warning icon appears inside context badge with percentage', () => {
    const { container } = renderCard(makeAgent({ context_left: 30, restart_threshold: 25, result: undefined }))
    const contextBadge = container.querySelector('.absolute.top-1.right-1')
    expect(contextBadge?.textContent).toContain('30%')
    expect(hasWarningIcon(container)).toBe(true)
  })
})

describe('ActiveAgentsPanel - restart threshold warning', () => {
  function hasPanelWarning(): boolean {
    return document.querySelector('svg.h-3.w-3.text-amber-500') !== null
  }

  function countWarnings(): number {
    return document.querySelectorAll('svg.h-3.w-3.text-amber-500').length
  }

  it('shows warning for running agent near threshold', () => {
    render(<ActiveAgentsPanel agents={{
      'impl:claude:sonnet': makeAgent({ context_left: 30, restart_threshold: 25, result: undefined }),
    }} />)
    expect(hasPanelWarning()).toBe(true)
  })

  it('filters completed agents from panel', () => {
    render(<ActiveAgentsPanel agents={{
      'impl:claude:sonnet': makeAgent({ context_left: 30, result: 'pass' }),
    }} />)
    expect(screen.queryByText('implementor')).not.toBeInTheDocument()
  })

  it('shows warnings per-agent independently', () => {
    render(<ActiveAgentsPanel agents={{
      'impl:claude:sonnet': makeAgent({ agent_id: 'a1', agent_type: 'implementor', context_left: 30, restart_threshold: 25, result: undefined }),
      'tester:claude:opus': makeAgent({ agent_id: 'a2', agent_type: 'tester', context_left: 70, restart_threshold: 25, result: undefined }),
    }} />)
    expect(countWarnings()).toBe(1)
  })

  it('uses per-agent restart_threshold', () => {
    render(<ActiveAgentsPanel agents={{
      'impl:claude:sonnet': makeAgent({ agent_id: 'a1', agent_type: 'implementor', context_left: 45, restart_threshold: 35, result: undefined }),
      'tester:claude:opus': makeAgent({ agent_id: 'a2', agent_type: 'tester', context_left: 45, restart_threshold: 20, result: undefined }),
    }} />)
    // Agent 1: 45 <= 50 => warning; Agent 2: 45 > 35 => no warning
    expect(countWarnings()).toBe(1)
  })
})
