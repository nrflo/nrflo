import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AgentCard } from './AgentCard'
import type { ActiveAgentV4 } from '@/types/workflow'

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

describe('AgentCard', () => {
  it('shows context_left indicator when present', () => {
    const agent = makeAgent({ context_left: 72 })
    render(<AgentCard agent={agent} />)
    expect(screen.getByText('72%')).toBeInTheDocument()
  })

  it('hides context_left indicator when undefined', () => {
    const agent = makeAgent({ context_left: undefined })
    render(<AgentCard agent={agent} />)
    expect(screen.queryByText(/%$/)).not.toBeInTheDocument()
  })

  it('shows context_left with green styling for > 50%', () => {
    const agent = makeAgent({ context_left: 75 })
    render(<AgentCard agent={agent} />)
    const badge = screen.getByText('75%')
    expect(badge.className).toContain('bg-green-100')
  })

  it('shows context_left with yellow styling for 26-50%', () => {
    const agent = makeAgent({ context_left: 40 })
    render(<AgentCard agent={agent} />)
    const badge = screen.getByText('40%')
    expect(badge.className).toContain('bg-yellow-100')
  })

  it('shows context_left with red styling for <= 25%', () => {
    const agent = makeAgent({ context_left: 15 })
    render(<AgentCard agent={agent} />)
    const badge = screen.getByText('15%')
    expect(badge.className).toContain('bg-red-100')
  })

  it('displays elapsed time from started_at for running agent', () => {
    const agent = makeAgent({
      started_at: '2026-01-01T00:00:00Z',
      ended_at: undefined,
      result: undefined,
    })
    render(<AgentCard agent={agent} />)
    // Running agent: formatElapsedTime(started_at) uses current time as end
    // We can't test exact value, but elapsed time element should exist
    const timerText = screen.getByText((content) =>
      /\d+[smh]/.test(content)
    )
    expect(timerText).toBeInTheDocument()
  })

  it('displays elapsed time from started_at to ended_at for completed agent', () => {
    const agent = makeAgent({
      started_at: '2026-01-01T00:00:00Z',
      ended_at: '2026-01-01T00:02:30Z',
      result: 'pass',
    })
    render(<AgentCard agent={agent} />)
    expect(screen.getByText('2m 30s')).toBeInTheDocument()
  })

  it('shows 0s when started_at is missing', () => {
    const agent = makeAgent({
      started_at: undefined,
      result: 'pass',
    })
    render(<AgentCard agent={agent} />)
    expect(screen.getByText('0s')).toBeInTheDocument()
  })

  it('shows context_left for completed agent', () => {
    const agent = makeAgent({
      result: 'pass',
      ended_at: '2026-01-01T00:01:00Z',
      context_left: 30,
    })
    render(<AgentCard agent={agent} />)
    expect(screen.getByText('30%')).toBeInTheDocument()
  })

  it('shows context_left at 0%', () => {
    const agent = makeAgent({ context_left: 0 })
    render(<AgentCard agent={agent} />)
    expect(screen.getByText('0%')).toBeInTheDocument()
    const badge = screen.getByText('0%')
    expect(badge.className).toContain('bg-red-100')
  })

  it('calls onExpand when clicked', async () => {
    const onExpand = vi.fn()
    const agent = makeAgent()
    render(<AgentCard agent={agent} onExpand={onExpand} />)
    const button = screen.getByRole('button')
    button.click()
    expect(onExpand).toHaveBeenCalledTimes(1)
  })

  // Context-left badge positioning and sizing (ticket nrworkflow-30efa6)
  it('positions context_left badge at top-right corner', () => {
    const agent = makeAgent({ context_left: 60 })
    render(<AgentCard agent={agent} />)
    const badge = screen.getByText('60%')
    expect(badge.className).toContain('absolute')
    expect(badge.className).toContain('top-1')
    expect(badge.className).toContain('right-1')
  })

  it('renders context_left badge with doubled text size (text-lg)', () => {
    const agent = makeAgent({ context_left: 50 })
    render(<AgentCard agent={agent} />)
    const badge = screen.getByText('50%')
    expect(badge.className).toContain('text-lg')
  })

  it('renders context_left badge with monospace font', () => {
    const agent = makeAgent({ context_left: 80 })
    render(<AgentCard agent={agent} />)
    const badge = screen.getByText('80%')
    expect(badge.className).toContain('font-mono')
  })

  it('parent button has relative positioning for absolute badge', () => {
    const agent = makeAgent({ context_left: 55 })
    render(<AgentCard agent={agent} />)
    const button = screen.getByRole('button')
    expect(button.className).toContain('relative')
  })
})
