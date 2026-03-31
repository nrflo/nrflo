import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AgentFlowNode } from './AgentFlowNode'
import { useTickingClock } from '@/hooks/useElapsedTime'
import type { AgentFlowNodeData } from './types'
import type { ActiveAgentV4, AgentHistoryEntry, AgentSession } from '@/types/workflow'

// Mock @xyflow/react Handle component (renders nothing)
vi.mock('@xyflow/react', () => ({
  Handle: () => null,
  Position: { Top: 'top', Bottom: 'bottom' },
}))

// Mock useTickingClock to verify it is called with the correct argument
vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
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

  it('renders invisible layout placeholder (aria-hidden) when contextLeft is null', () => {
    const data = makeData({ agent: makeAgent({ context_left: undefined }) })
    const { container } = render(<AgentFlowNode data={data} />)
    // Invisible spacer keeps row 1 width stable when there is no context value
    const placeholder = container.querySelector('[aria-hidden="true"]')
    expect(placeholder).toBeInTheDocument()
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

  // Context-left badge: color thresholds
  it('shows green styling for context_left > 50%', () => {
    const data = makeData({ agent: makeAgent({ context_left: 75 }) })
    render(<AgentFlowNode data={data} />)
    const badge = screen.getByText('75%')
    expect(badge.className).toContain('text-green-700')
    expect(badge.className).not.toContain('bg-')
  })

  it('shows yellow styling for context_left 26-50%', () => {
    const data = makeData({ agent: makeAgent({ context_left: 40 }) })
    render(<AgentFlowNode data={data} />)
    const badge = screen.getByText('40%')
    expect(badge.className).toContain('text-yellow-700')
    expect(badge.className).not.toContain('bg-')
  })

  it('shows red styling for context_left <= 25%', () => {
    const data = makeData({ agent: makeAgent({ context_left: 15 }) })
    render(<AgentFlowNode data={data} />)
    const badge = screen.getByText('15%')
    expect(badge.className).toContain('text-red-700')
    expect(badge.className).not.toContain('bg-')
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

  describe('timer hook (useTickingClock)', () => {
    beforeEach(() => vi.mocked(useTickingClock).mockClear())

    it('calls useTickingClock(true) when agent is running (no result)', () => {
      render(<AgentFlowNode data={makeData()} />)
      expect(vi.mocked(useTickingClock)).toHaveBeenCalledWith(true)
    })

    it('calls useTickingClock(false) when agent is completed (has result)', () => {
      const data = makeData({ agent: undefined, historyEntry: makeHistory({ result: 'pass' }) })
      render(<AgentFlowNode data={data} />)
      expect(vi.mocked(useTickingClock)).toHaveBeenCalledWith(false)
    })

    it('calls useTickingClock(false) when phase is pending (no agent)', () => {
      const data = makeData({ agent: undefined, isPending: true })
      render(<AgentFlowNode data={data} />)
      expect(vi.mocked(useTickingClock)).toHaveBeenCalledWith(false)
    })
  })

  // Unified card sizing — all variants use w-[242px] sm:w-[330px] and min-h-[90px]
  it('all card variants use w-[242px] (mobile base) and min-h-[90px]', () => {
    const variants = [
      makeData({ agent: makeAgent({ result: undefined }) }),
      makeData({ agent: undefined, historyEntry: makeHistory({ result: 'pass' }) }),
      makeData({ agent: undefined, historyEntry: makeHistory({ result: 'fail' }) }),
      makeData({ agent: undefined, isPending: true }),
      makeData({ agent: undefined, isSkipped: true }),
      makeData({ agent: undefined, isError: true }),
    ]
    variants.forEach((data) => {
      const { unmount, container } = render(<AgentFlowNode data={data} />)
      const card = container.querySelector('.w-\\[242px\\]')
      expect(card).toBeInTheDocument()
      expect(card?.className).toContain('min-h-[90px]')
      expect(container.querySelector('[class*="min-w"]')).not.toBeInTheDocument()
      unmount()
    })
  })
})
