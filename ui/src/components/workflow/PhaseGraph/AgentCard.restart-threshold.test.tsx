import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { AgentCard } from './AgentCard'
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

function renderCard(agent: ActiveAgentV4, props: Partial<AgentCardProps> = {}) {
  const defaultProps: AgentCardProps = {
    agent,
    session: undefined,
    onExpand: vi.fn(),
    isExpanded: false,
    ...props,
  }
  return render(<AgentCard {...defaultProps} />)
}

function hasWarningIcon(container: HTMLElement): boolean {
  // AlertTriangle icon has h-4 w-4 and text-amber-500 classes
  const icon = container.querySelector('svg.h-4.w-4.text-amber-500')
  return icon !== null
}

describe('AgentCard - restart threshold warning', () => {
  describe('warning icon display for running agents', () => {
    it('shows AlertTriangle warning when running and context_left <= threshold+10', () => {
      const agent = makeAgent({ context_left: 35, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      // context_left (35) <= threshold+10 (35) => warning shown
      expect(hasWarningIcon(container)).toBe(true)
    })

    it('shows warning when context_left exactly equals threshold+10', () => {
      const agent = makeAgent({ context_left: 35, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(true)
    })

    it('shows warning when context_left is 1% above threshold', () => {
      const agent = makeAgent({ context_left: 26, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(true)
    })

    it('shows warning when context_left equals threshold', () => {
      const agent = makeAgent({ context_left: 25, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(true)
    })

    it('shows warning when context_left is below threshold', () => {
      const agent = makeAgent({ context_left: 20, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(true)
    })

    it('does NOT show warning when context_left > threshold+10', () => {
      const agent = makeAgent({ context_left: 36, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      // context_left (36) > threshold+10 (35) => no warning
      expect(hasWarningIcon(container)).toBe(false)
    })

    it('does NOT show warning when context_left is far above threshold', () => {
      const agent = makeAgent({ context_left: 60, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(false)
    })
  })

  describe('warning icon not shown for completed agents', () => {
    it('does NOT show warning when agent has result=pass even if context near threshold', () => {
      const agent = makeAgent({ context_left: 30, restart_threshold: 25, result: 'pass' })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(false)
    })

    it('does NOT show warning when agent has result=fail', () => {
      const agent = makeAgent({ context_left: 20, restart_threshold: 25, result: 'fail' })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(false)
    })

    it('does NOT show warning when agent has result=continue', () => {
      const agent = makeAgent({ context_left: 25, restart_threshold: 25, result: 'continue' })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(false)
    })
  })

  describe('custom restart_threshold values', () => {
    it('uses custom restart_threshold when provided', () => {
      const agent = makeAgent({ context_left: 55, restart_threshold: 50, result: undefined })
      const { container } = renderCard(agent)

      // context_left (55) <= threshold+10 (60) => warning shown
      expect(hasWarningIcon(container)).toBe(true)
    })

    it('does NOT show warning when context > custom_threshold+10', () => {
      const agent = makeAgent({ context_left: 61, restart_threshold: 50, result: undefined })
      const { container } = renderCard(agent)

      // context_left (61) > threshold+10 (60) => no warning
      expect(hasWarningIcon(container)).toBe(false)
    })

    it('handles restart_threshold=1 (shows warning when context_left <= 11)', () => {
      const agent = makeAgent({ context_left: 11, restart_threshold: 1, result: undefined })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(true)
    })

    it('handles restart_threshold=99 (shows warning when context_left=100)', () => {
      const agent = makeAgent({ context_left: 100, restart_threshold: 99, result: undefined })
      const { container } = renderCard(agent)

      // 100 <= 109 => warning shown
      expect(hasWarningIcon(container)).toBe(true)
    })
  })

  describe('fallback to default threshold', () => {
    it('uses default threshold of 25 when restart_threshold is undefined', () => {
      const agent = makeAgent({ context_left: 35, restart_threshold: undefined, result: undefined })
      const { container } = renderCard(agent)

      // Falls back to 25, so 35 <= 35 => warning shown
      expect(hasWarningIcon(container)).toBe(true)
    })

    it('does NOT show warning when restart_threshold is undefined and context > 35', () => {
      const agent = makeAgent({ context_left: 36, restart_threshold: undefined, result: undefined })
      const { container } = renderCard(agent)

      // Falls back to 25, so 36 > 35 => no warning
      expect(hasWarningIcon(container)).toBe(false)
    })

    it('uses default threshold of 25 when restart_threshold is null', () => {
      const agent = makeAgent({ context_left: 30, restart_threshold: null as any, result: undefined })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(true)
    })
  })

  describe('edge cases', () => {
    it('does NOT show warning when context_left is undefined', () => {
      const agent = makeAgent({ context_left: undefined, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(false)
    })

    it('does NOT show warning when context_left is null', () => {
      const agent = makeAgent({ context_left: null as any, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(false)
    })

    it('handles context_left = 0', () => {
      const agent = makeAgent({ context_left: 0, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(true)
    })

    it('handles context_left = 100', () => {
      const agent = makeAgent({ context_left: 100, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      expect(hasWarningIcon(container)).toBe(false)
    })
  })

  describe('warning icon positioning', () => {
    it('warning icon appears inside context badge', () => {
      const agent = makeAgent({ context_left: 30, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      // Context badge is an absolute positioned span containing context_left and warning icon
      const contextBadge = container.querySelector('.absolute.top-1.right-1')
      expect(contextBadge).toBeInTheDocument()
      expect(contextBadge?.textContent).toContain('30%')
      expect(hasWarningIcon(container)).toBe(true)
    })

    it('context badge shows both warning icon and percentage', () => {
      const agent = makeAgent({ context_left: 28, restart_threshold: 25, result: undefined })
      const { container } = renderCard(agent)

      const contextBadge = container.querySelector('.absolute.top-1.right-1')
      const percentage = contextBadge?.textContent

      expect(hasWarningIcon(container)).toBe(true)
      expect(percentage).toContain('28%')
    })
  })

  describe('multiple agents with different thresholds', () => {
    it('correctly evaluates warning for each agent independently', () => {
      const { container: container1 } = renderCard(
        makeAgent({ context_left: 30, restart_threshold: 25, result: undefined })
      )
      const { container: container2 } = renderCard(
        makeAgent({ context_left: 30, restart_threshold: 50, result: undefined })
      )

      // Agent 1: 30 <= 35 => warning shown
      expect(hasWarningIcon(container1)).toBe(true)

      // Agent 2: 30 <= 60 => warning shown
      expect(hasWarningIcon(container2)).toBe(true)
    })

    it('one agent shows warning, another does not', () => {
      const { container: container1 } = renderCard(
        makeAgent({ context_left: 30, restart_threshold: 25, result: undefined })
      )
      const { container: container2 } = renderCard(
        makeAgent({ context_left: 70, restart_threshold: 25, result: undefined })
      )

      // Agent 1: 30 <= 35 => warning
      expect(hasWarningIcon(container1)).toBe(true)

      // Agent 2: 70 > 35 => no warning
      expect(hasWarningIcon(container2)).toBe(false)
    })
  })
})
