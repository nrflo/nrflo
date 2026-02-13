import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AgentFlowNode } from './AgentFlowNode'
import type { AgentFlowNodeData } from './types'
import type { ActiveAgentV4, AgentHistoryEntry, AgentSession } from '@/types/workflow'

// Mock @xyflow/react Handle component (renders nothing)
vi.mock('@xyflow/react', () => ({
  Handle: () => null,
  Position: { Top: 'top', Bottom: 'bottom' },
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

function makeSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 's1',
    project_id: 'proj1',
    ticket_id: 'T-1',
    workflow_instance_id: 'wi1',
    phase: 'implementation',
    workflow: 'feature',
    agent_type: 'implementor',
    model_id: 'claude-sonnet-4-5',
    status: 'running',
    message_count: 5,
    raw_output_size: 0,
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:03:00Z',
    ...overrides,
  }
}

function makeData(overrides: Partial<AgentFlowNodeData> = {}): AgentFlowNodeData {
  return {
    agentKey: 'impl:claude:sonnet',
    phaseName: 'implementation',
    phaseIndex: 0,
    agent: makeAgent(),
    onToggleExpand: vi.fn(),
    ...overrides,
  }
}

describe('AgentFlowNode', () => {
  // Context-left badge: presence and text
  it('shows context_left when agent has it set', () => {
    const data = makeData({ agent: makeAgent({ context_left: 72 }) })
    render(<AgentFlowNode data={data} />)
    expect(screen.getByText('72%')).toBeInTheDocument()
  })

  it('hides context_left when agent has no context_left', () => {
    const data = makeData({ agent: makeAgent({ context_left: undefined }) })
    render(<AgentFlowNode data={data} />)
    expect(screen.queryByText(/%$/)).not.toBeInTheDocument()
  })

  it('shows context_left from history entry when no active agent', () => {
    const data = makeData({
      agent: undefined,
      historyEntry: makeHistory({ context_left: 45 }),
    })
    render(<AgentFlowNode data={data} />)
    expect(screen.getByText('45%')).toBeInTheDocument()
  })

  it('shows context_left at 0%', () => {
    const data = makeData({ agent: makeAgent({ context_left: 0 }) })
    render(<AgentFlowNode data={data} />)
    expect(screen.getByText('0%')).toBeInTheDocument()
  })

  // Context-left badge: positioning (ticket nrworkflow-30efa6)
  it('positions context_left badge at absolute top-right corner', () => {
    const data = makeData({ agent: makeAgent({ context_left: 60 }) })
    render(<AgentFlowNode data={data} />)
    const badge = screen.getByText('60%')
    expect(badge.className).toContain('absolute')
    expect(badge.className).toContain('top-1')
    expect(badge.className).toContain('right-1')
  })

  // Context-left badge: doubled text size (ticket nrworkflow-30efa6)
  it('renders context_left badge with doubled text size (text-base)', () => {
    const data = makeData({ agent: makeAgent({ context_left: 50 }) })
    render(<AgentFlowNode data={data} />)
    const badge = screen.getByText('50%')
    expect(badge.className).toContain('text-base')
  })

  it('renders context_left badge with monospace font', () => {
    const data = makeData({ agent: makeAgent({ context_left: 80 }) })
    render(<AgentFlowNode data={data} />)
    const badge = screen.getByText('80%')
    expect(badge.className).toContain('font-mono')
  })

  // Context-left badge: color thresholds
  it('shows green styling for context_left > 50%', () => {
    const data = makeData({ agent: makeAgent({ context_left: 75 }) })
    render(<AgentFlowNode data={data} />)
    const badge = screen.getByText('75%')
    expect(badge.className).toContain('bg-green-100')
  })

  it('shows yellow styling for context_left 26-50%', () => {
    const data = makeData({ agent: makeAgent({ context_left: 40 }) })
    render(<AgentFlowNode data={data} />)
    const badge = screen.getByText('40%')
    expect(badge.className).toContain('bg-yellow-100')
  })

  it('shows red styling for context_left <= 25%', () => {
    const data = makeData({ agent: makeAgent({ context_left: 15 }) })
    render(<AgentFlowNode data={data} />)
    const badge = screen.getByText('15%')
    expect(badge.className).toContain('bg-red-100')
  })

  it('shows red at boundary value 25', () => {
    const data = makeData({ agent: makeAgent({ context_left: 25 }) })
    render(<AgentFlowNode data={data} />)
    const badge = screen.getByText('25%')
    expect(badge.className).toContain('bg-red-100')
  })

  it('shows yellow at boundary value 50', () => {
    const data = makeData({ agent: makeAgent({ context_left: 50 }) })
    render(<AgentFlowNode data={data} />)
    const badge = screen.getByText('50%')
    expect(badge.className).toContain('bg-yellow-100')
  })

  // Card has relative positioning for absolute badge
  it('card button has relative class for absolute badge positioning', () => {
    const data = makeData({ agent: makeAgent({ context_left: 55 }) })
    render(<AgentFlowNode data={data} />)
    const button = screen.getByRole('button')
    expect(button.className).toContain('relative')
  })

  // Phase label display
  it('shows phase name with underscores replaced by spaces', () => {
    const data = makeData({ phaseName: 'test_design' })
    render(<AgentFlowNode data={data} />)
    expect(screen.getByText('test design')).toBeInTheDocument()
  })

  // Model name extraction
  it('extracts model name from model_id', () => {
    const data = makeData({ agent: makeAgent({ model_id: 'claude-sonnet-4-5' }) })
    render(<AgentFlowNode data={data} />)
    expect(screen.getByText('5')).toBeInTheDocument()
  })

  // Duration display
  it('shows duration for completed history entry', () => {
    const data = makeData({
      agent: undefined,
      historyEntry: makeHistory({
        started_at: '2026-01-01T00:00:00Z',
        ended_at: '2026-01-01T00:02:30Z',
      }),
    })
    render(<AgentFlowNode data={data} />)
    expect(screen.getByText('2m 30s')).toBeInTheDocument()
  })

  // Message count badge
  it('shows message count badge when session has messages', () => {
    const data = makeData({
      session: makeSession({ message_count: 8 }),
    })
    render(<AgentFlowNode data={data} />)
    expect(screen.getByText('8 msgs')).toBeInTheDocument()
  })

  it('shows singular msg for 1 message', () => {
    const data = makeData({
      session: makeSession({ message_count: 1 }),
    })
    render(<AgentFlowNode data={data} />)
    expect(screen.getByText('1 msg')).toBeInTheDocument()
  })

  it('hides message count when session has 0 messages', () => {
    const data = makeData({
      session: makeSession({ message_count: 0 }),
    })
    render(<AgentFlowNode data={data} />)
    expect(screen.queryByText(/\d+ msgs?/)).not.toBeInTheDocument()
  })

  // Click handler
  it('calls onToggleExpand when clicked', () => {
    const onToggle = vi.fn()
    const data = makeData({ onToggleExpand: onToggle })
    render(<AgentFlowNode data={data} />)
    screen.getByRole('button').click()
    expect(onToggle).toHaveBeenCalledTimes(1)
  })

  // Pending/skipped/completed placeholder phases
  it('renders pending phase placeholder', () => {
    const data = makeData({ agent: undefined, isPending: true })
    render(<AgentFlowNode data={data} />)
    expect(screen.getByText('pending')).toBeInTheDocument()
  })

  it('renders skipped phase placeholder', () => {
    const data = makeData({ agent: undefined, isSkipped: true })
    render(<AgentFlowNode data={data} />)
    expect(screen.getByText('skipped')).toBeInTheDocument()
  })

  it('renders completed phase placeholder', () => {
    const data = makeData({ agent: undefined, isCompleted: true })
    render(<AgentFlowNode data={data} />)
    expect(screen.getByText('completed')).toBeInTheDocument()
  })

  it('renders error phase placeholder', () => {
    const data = makeData({ agent: undefined, isError: true })
    render(<AgentFlowNode data={data} />)
    expect(screen.getByText('error')).toBeInTheDocument()
  })

  it('does not show context_left badge on pending placeholder', () => {
    const data = makeData({ agent: undefined, isPending: true })
    render(<AgentFlowNode data={data} />)
    expect(screen.queryByText(/%$/)).not.toBeInTheDocument()
  })

  // Running vs completed styling
  it('applies yellow border for running agent', () => {
    const data = makeData({ agent: makeAgent({ result: undefined }) })
    render(<AgentFlowNode data={data} />)
    const button = screen.getByRole('button')
    expect(button.className).toContain('border-yellow-500')
  })

  it('applies green border for passed agent', () => {
    const data = makeData({
      agent: undefined,
      historyEntry: makeHistory({ result: 'pass' }),
    })
    render(<AgentFlowNode data={data} />)
    const button = screen.getByRole('button')
    expect(button.className).toContain('border-green-500')
  })

  it('applies red border for failed agent', () => {
    const data = makeData({
      agent: undefined,
      historyEntry: makeHistory({ result: 'fail' }),
    })
    render(<AgentFlowNode data={data} />)
    const button = screen.getByRole('button')
    expect(button.className).toContain('border-red-500')
  })

  // Unified card sizing (ticket nrworkflow-40b179)
  describe('unified card sizing', () => {
    it('running card has w-[220px] fixed width', () => {
      const data = makeData({ agent: makeAgent({ result: undefined }) })
      render(<AgentFlowNode data={data} />)
      const button = screen.getByRole('button')
      expect(button.className).toContain('w-[220px]')
    })

    it('completed card has w-[220px] fixed width', () => {
      const data = makeData({
        agent: undefined,
        historyEntry: makeHistory({ result: 'pass' }),
      })
      render(<AgentFlowNode data={data} />)
      const button = screen.getByRole('button')
      expect(button.className).toContain('w-[220px]')
    })

    it('failed card has w-[220px] fixed width', () => {
      const data = makeData({
        agent: undefined,
        historyEntry: makeHistory({ result: 'fail' }),
      })
      render(<AgentFlowNode data={data} />)
      const button = screen.getByRole('button')
      expect(button.className).toContain('w-[220px]')
    })

    it('pending placeholder has w-[220px] fixed width', () => {
      const data = makeData({ agent: undefined, isPending: true })
      const { container } = render(<AgentFlowNode data={data} />)
      const card = container.querySelector('.w-\\[220px\\]')
      expect(card).toBeInTheDocument()
      expect(card?.className).toContain('w-[220px]')
    })

    it('skipped placeholder has w-[220px] fixed width', () => {
      const data = makeData({ agent: undefined, isSkipped: true })
      const { container } = render(<AgentFlowNode data={data} />)
      const card = container.querySelector('.w-\\[220px\\]')
      expect(card).toBeInTheDocument()
      expect(card?.className).toContain('w-[220px]')
    })

    it('completed placeholder has w-[220px] fixed width', () => {
      const data = makeData({ agent: undefined, isCompleted: true })
      const { container } = render(<AgentFlowNode data={data} />)
      const card = container.querySelector('.w-\\[220px\\]')
      expect(card).toBeInTheDocument()
      expect(card?.className).toContain('w-[220px]')
    })

    it('error placeholder has w-[220px] fixed width', () => {
      const data = makeData({ agent: undefined, isError: true })
      const { container } = render(<AgentFlowNode data={data} />)
      const card = container.querySelector('.w-\\[220px\\]')
      expect(card).toBeInTheDocument()
      expect(card?.className).toContain('w-[220px]')
    })

    it('running card has min-h-[90px] minimum height', () => {
      const data = makeData({ agent: makeAgent({ result: undefined }) })
      render(<AgentFlowNode data={data} />)
      const button = screen.getByRole('button')
      expect(button.className).toContain('min-h-[90px]')
    })

    it('completed card has min-h-[90px] minimum height', () => {
      const data = makeData({
        agent: undefined,
        historyEntry: makeHistory({ result: 'pass' }),
      })
      render(<AgentFlowNode data={data} />)
      const button = screen.getByRole('button')
      expect(button.className).toContain('min-h-[90px]')
    })

    it('pending placeholder has min-h-[90px] minimum height', () => {
      const data = makeData({ agent: undefined, isPending: true })
      const { container } = render(<AgentFlowNode data={data} />)
      const card = container.querySelector('.min-h-\\[90px\\]')
      expect(card).toBeInTheDocument()
      expect(card?.className).toContain('min-h-[90px]')
    })

    it('skipped placeholder has min-h-[90px] minimum height', () => {
      const data = makeData({ agent: undefined, isSkipped: true })
      const { container } = render(<AgentFlowNode data={data} />)
      const card = container.querySelector('.min-h-\\[90px\\]')
      expect(card).toBeInTheDocument()
      expect(card?.className).toContain('min-h-[90px]')
    })

    it('all card variants use same fixed width for alignment', () => {
      // Test that all card states render with the same width class
      const variants = [
        { label: 'running', data: makeData({ agent: makeAgent({ result: undefined }) }) },
        { label: 'completed', data: makeData({ agent: undefined, historyEntry: makeHistory({ result: 'pass' }) }) },
        { label: 'failed', data: makeData({ agent: undefined, historyEntry: makeHistory({ result: 'fail' }) }) },
        { label: 'pending', data: makeData({ agent: undefined, isPending: true }) },
        { label: 'skipped', data: makeData({ agent: undefined, isSkipped: true }) },
        { label: 'error', data: makeData({ agent: undefined, isError: true }) },
      ]

      variants.forEach(({ label }) => {
        const { unmount, container } = render(<AgentFlowNode data={variants.find(v => v.label === label)!.data} />)
        const card = container.querySelector('.w-\\[220px\\]')
        expect(card).toBeInTheDocument()
        expect(card?.className).toContain('w-[220px]')
        unmount()
      })
    })

    it('does not use min-w class on any card variant', () => {
      // Ensure old min-w-[180px] and min-w-[220px] are removed
      const variants = [
        { label: 'running', data: makeData({ agent: makeAgent({ result: undefined }) }) },
        { label: 'pending', data: makeData({ agent: undefined, isPending: true }) },
      ]

      variants.forEach(({ data }) => {
        const { unmount, container } = render(<AgentFlowNode data={data} />)
        const card = container.querySelector('[class*="min-w"]')
        expect(card).not.toBeInTheDocument()
        unmount()
      })
    })
  })
})
