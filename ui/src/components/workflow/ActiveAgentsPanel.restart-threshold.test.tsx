import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ActiveAgentsPanel } from './ActiveAgentsPanel'
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

function hasWarningIcon(): boolean {
  const icon = document.querySelector('svg.h-3.w-3.text-amber-500')
  return icon !== null
}

function countWarningIcons(): number {
  const icons = document.querySelectorAll('svg.h-3.w-3.text-amber-500')
  return icons.length
}

describe('ActiveAgentsPanel - restart threshold warning', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('warning icon in context badge', () => {
    it('shows AlertTriangle warning for running agent when context_left <= threshold+15', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 40, restart_threshold: 25, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      expect(hasWarningIcon()).toBe(true)
    })

    it('shows warning when context_left exactly equals threshold+15', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 40, restart_threshold: 25, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      expect(hasWarningIcon()).toBe(true)
    })

    it('shows warning when context_left is at threshold', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 25, restart_threshold: 25, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      expect(hasWarningIcon()).toBe(true)
    })

    it('shows warning when context_left is below threshold', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 20, restart_threshold: 25, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      expect(hasWarningIcon()).toBe(true)
    })

    it('does NOT show warning when context_left > threshold+15', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 41, restart_threshold: 25, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      expect(hasWarningIcon()).toBe(false)
    })

    it('does NOT show warning when context_left is far above threshold', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 70, restart_threshold: 25, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      expect(hasWarningIcon()).toBe(false)
    })
  })

  describe('warning not shown for completed agents', () => {
    it('does NOT show warning for completed agent even if context near threshold', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 30, restart_threshold: 25, result: 'pass' }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      // Panel only shows running agents, so completed agent won't be displayed
      expect(screen.queryByText('implementor')).not.toBeInTheDocument()
    })

    it('panel does not render agents with result set', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 25, restart_threshold: 25, result: 'fail' }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      // Panel filters out completed agents
      expect(screen.queryByText('implementor')).not.toBeInTheDocument()
    })
  })

  describe('custom restart_threshold values', () => {
    it('uses custom restart_threshold when provided', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 60, restart_threshold: 50, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      // 60 <= 65 => warning shown
      expect(hasWarningIcon()).toBe(true)
    })

    it('does NOT show warning when context > custom_threshold+15', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 66, restart_threshold: 50, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      // 66 > 65 => no warning
      expect(hasWarningIcon()).toBe(false)
    })

    it('handles low restart_threshold values', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 16, restart_threshold: 5, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      // 16 <= 20 => warning
      expect(hasWarningIcon()).toBe(true)
    })

    it('handles high restart_threshold values', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 90, restart_threshold: 80, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      // 90 <= 95 => warning
      expect(hasWarningIcon()).toBe(true)
    })
  })

  describe('fallback to default threshold', () => {
    it('uses default threshold of 25 when restart_threshold is undefined', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 40, restart_threshold: undefined, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      // Falls back to 25, so 40 <= 40 => warning
      expect(hasWarningIcon()).toBe(true)
    })

    it('does NOT show warning when restart_threshold is undefined and context > 40', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 41, restart_threshold: undefined, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      // Falls back to 25, so 41 > 40 => no warning
      expect(hasWarningIcon()).toBe(false)
    })

    it('uses default threshold of 25 when restart_threshold is null', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 30, restart_threshold: null as any, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      expect(hasWarningIcon()).toBe(true)
    })
  })

  describe('edge cases', () => {
    it('does NOT show warning when context_left is undefined', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: undefined, restart_threshold: 25, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      expect(hasWarningIcon()).toBe(false)
    })

    it('does NOT show warning when context_left is null', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: null as any, restart_threshold: 25, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      expect(hasWarningIcon()).toBe(false)
    })

    it('handles context_left = 0', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 0, restart_threshold: 25, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      expect(hasWarningIcon()).toBe(true)
    })

    it('handles context_left = 100', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 100, restart_threshold: 25, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      expect(hasWarningIcon()).toBe(false)
    })
  })

  describe('warning icon positioning', () => {
    it('warning icon appears inside context badge', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 30, restart_threshold: 25, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      // Find the context badge
      const contextBadge = screen.getByText(/30% ctx/).closest('span')
      expect(contextBadge).toBeInTheDocument()

      expect(contextBadge?.querySelector('svg')).toBeTruthy()
    })

    it('context badge shows both warning icon and percentage text', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({ context_left: 28, restart_threshold: 25, result: undefined }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      const contextText = screen.getByText(/28% ctx/)
      expect(contextText).toBeInTheDocument()

      const contextBadge = contextText.closest('span')
      expect(contextBadge?.querySelector('svg')).toBeTruthy()
    })
  })

  describe('multiple running agents', () => {
    it('shows warning for agents near threshold, not for others', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({
          agent_id: 'a1',
          agent_type: 'implementor',
          context_left: 30,
          restart_threshold: 25,
          result: undefined,
        }),
        'tester:claude:opus': makeAgent({
          agent_id: 'a2',
          agent_type: 'tester',
          context_left: 70,
          restart_threshold: 25,
          result: undefined,
        }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      // Both agents should be in the panel
      expect(screen.getByText('implementor')).toBeInTheDocument()
      expect(screen.getByText('tester')).toBeInTheDocument()

      // Only one warning icon should exist (for implementor with context_left 30)
      const count = countWarningIcons()
      expect(count).toBe(1)
    })

    it('each agent uses its own restart_threshold for warning evaluation', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({
          agent_id: 'a1',
          agent_type: 'implementor',
          context_left: 45,
          restart_threshold: 35,
          result: undefined,
        }),
        'tester:claude:opus': makeAgent({
          agent_id: 'a2',
          agent_type: 'tester',
          context_left: 45,
          restart_threshold: 20,
          result: undefined,
        }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      // Agent 1: 45 <= 50 => warning
      // Agent 2: 45 > 35 => no warning
      const count = countWarningIcons()
      expect(count).toBe(1)
    })

    it('shows multiple warnings when multiple agents are near threshold', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({
          agent_id: 'a1',
          agent_type: 'implementor',
          context_left: 30,
          restart_threshold: 25,
          result: undefined,
        }),
        'tester:claude:opus': makeAgent({
          agent_id: 'a2',
          agent_type: 'tester',
          context_left: 28,
          restart_threshold: 25,
          result: undefined,
        }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      const count = countWarningIcons()
      expect(count).toBe(2)
    })

    it('no warnings when all agents have sufficient context', () => {
      const agents = {
        'impl:claude:sonnet': makeAgent({
          agent_id: 'a1',
          agent_type: 'implementor',
          context_left: 60,
          restart_threshold: 25,
          result: undefined,
        }),
        'tester:claude:opus': makeAgent({
          agent_id: 'a2',
          agent_type: 'tester',
          context_left: 75,
          restart_threshold: 25,
          result: undefined,
        }),
      }

      render(<ActiveAgentsPanel agents={agents} />)

      const count = countWarningIcons()
      expect(count).toBe(0)
    })
  })
})
