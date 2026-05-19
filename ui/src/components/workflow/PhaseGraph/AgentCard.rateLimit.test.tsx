import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AgentCard } from './AgentCard'
import type { ActiveAgentV4 } from '@/types/workflow'

vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

const BASE_TIME = new Date('2026-01-01T00:00:00.000Z')

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-6',
    cli: 'claude',
    model: 'sonnet',
    pid: 12345,
    session_id: 's1',
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('AgentCard — rate-limit state', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(BASE_TIME)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows "Waiting · retry #N · resumes HH:MM:SS" when waiting_for_rate_limit=true', () => {
    const untilTs = new Date(BASE_TIME.getTime() + 65 * 1000).toISOString()
    const agent = makeAgent({
      waiting_for_rate_limit: true,
      rate_limit_until_ts: untilTs,
      rate_limit_retry_count: 2,
    })
    render(<AgentCard agent={agent} />)
    expect(screen.getByText(/Waiting · retry #2 · resumes/)).toBeInTheDocument()
  })

  it('shows formatted countdown from rate_limit_until_ts', () => {
    const untilTs = new Date(BASE_TIME.getTime() + 65 * 1000).toISOString()
    const agent = makeAgent({
      waiting_for_rate_limit: true,
      rate_limit_until_ts: untilTs,
      rate_limit_retry_count: 1,
    })
    render(<AgentCard agent={agent} />)
    // 65 seconds = "1m 5s"
    expect(screen.getByText(/1m 5s/)).toBeInTheDocument()
  })

  it('shows "Resuming…" when rate_limit_until_ts is in the past', () => {
    const untilTs = new Date(BASE_TIME.getTime() - 1000).toISOString()
    const agent = makeAgent({
      waiting_for_rate_limit: true,
      rate_limit_until_ts: untilTs,
      rate_limit_retry_count: 0,
    })
    render(<AgentCard agent={agent} />)
    expect(screen.getByText(/Resuming…/)).toBeInTheDocument()
  })

  it('defaults retry count to 0 when rate_limit_retry_count is undefined', () => {
    const untilTs = new Date(BASE_TIME.getTime() + 30 * 1000).toISOString()
    const agent = makeAgent({
      waiting_for_rate_limit: true,
      rate_limit_until_ts: untilTs,
      rate_limit_retry_count: undefined,
    })
    render(<AgentCard agent={agent} />)
    expect(screen.getByText(/retry #0/)).toBeInTheDocument()
  })

  it('hides elapsed-time row when rate-limited', () => {
    const agent = makeAgent({ waiting_for_rate_limit: true })
    render(<AgentCard agent={agent} />)
    // Timer icon row is replaced by the waiting row
    expect(screen.queryByText(/^\d+[smh]/)).not.toBeInTheDocument()
  })

  it('shows elapsed-time row (not waiting row) when not rate-limited', () => {
    const agent = makeAgent({ waiting_for_rate_limit: false })
    render(<AgentCard agent={agent} />)
    expect(screen.queryByText(/Waiting · retry/)).not.toBeInTheDocument()
  })
})
