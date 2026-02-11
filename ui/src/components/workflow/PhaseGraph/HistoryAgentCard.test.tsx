import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { HistoryAgentCard } from './HistoryAgentCard'
import type { AgentHistoryEntry, AgentSession } from '@/types/workflow'

function makeEntry(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'a1',
    agent_type: 'setup-analyzer',
    model_id: 'claude-sonnet-4-5',
    phase: 'analyzer',
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
    phase: 'analyzer',
    workflow: 'feature',
    agent_type: 'setup-analyzer',
    model_id: 'claude-sonnet-4-5',
    status: 'completed',
    message_count: 5,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:03:00Z',
    ...overrides,
  }
}

describe('HistoryAgentCard', () => {
  // Duration display tests
  it('prefers started_at/ended_at for duration over duration_sec', () => {
    const entry = makeEntry({
      started_at: '2026-01-01T00:00:00Z',
      ended_at: '2026-01-01T00:02:30Z',
      duration_sec: 999, // Should be ignored
    })
    render(<HistoryAgentCard entry={entry} />)
    expect(screen.getByText('2m 30s')).toBeInTheDocument()
  })

  it('falls back to duration_sec when started_at/ended_at are missing', () => {
    const entry = makeEntry({
      started_at: undefined,
      ended_at: undefined,
      duration_sec: 90,
    })
    render(<HistoryAgentCard entry={entry} />)
    expect(screen.getByText('1m 30s')).toBeInTheDocument()
  })

  it('shows 0s when no duration info available', () => {
    const entry = makeEntry({
      started_at: undefined,
      ended_at: undefined,
      duration_sec: undefined,
    })
    render(<HistoryAgentCard entry={entry} />)
    expect(screen.getByText('0s')).toBeInTheDocument()
  })

  it('falls back to duration_sec when only started_at is present', () => {
    const entry = makeEntry({
      started_at: '2026-01-01T00:00:00Z',
      ended_at: undefined,
      duration_sec: 45,
    })
    render(<HistoryAgentCard entry={entry} />)
    expect(screen.getByText('45s')).toBeInTheDocument()
  })

  // Context left display tests
  it('shows context_left when present', () => {
    const entry = makeEntry({ context_left: 60 })
    render(<HistoryAgentCard entry={entry} />)
    expect(screen.getByText('60%')).toBeInTheDocument()
  })

  it('hides context_left when undefined', () => {
    const entry = makeEntry({ context_left: undefined })
    render(<HistoryAgentCard entry={entry} />)
    expect(screen.queryByText(/%$/)).not.toBeInTheDocument()
  })

  it('shows context_left with green styling for > 50%', () => {
    const entry = makeEntry({ context_left: 80 })
    render(<HistoryAgentCard entry={entry} />)
    const badge = screen.getByText('80%')
    expect(badge.className).toContain('bg-green-100')
  })

  it('shows context_left with yellow styling for 26-50%', () => {
    const entry = makeEntry({ context_left: 35 })
    render(<HistoryAgentCard entry={entry} />)
    const badge = screen.getByText('35%')
    expect(badge.className).toContain('bg-yellow-100')
  })

  it('shows context_left with red styling for <= 25%', () => {
    const entry = makeEntry({ context_left: 10 })
    render(<HistoryAgentCard entry={entry} />)
    const badge = screen.getByText('10%')
    expect(badge.className).toContain('bg-red-100')
  })

  it('shows context_left at boundary value 25 as red', () => {
    const entry = makeEntry({ context_left: 25 })
    render(<HistoryAgentCard entry={entry} />)
    const badge = screen.getByText('25%')
    expect(badge.className).toContain('bg-red-100')
  })

  it('shows context_left at boundary value 50 as yellow', () => {
    const entry = makeEntry({ context_left: 50 })
    render(<HistoryAgentCard entry={entry} />)
    const badge = screen.getByText('50%')
    expect(badge.className).toContain('bg-yellow-100')
  })

  // Status icon tests
  it('shows green check for pass result', () => {
    const entry = makeEntry({ result: 'pass' })
    render(<HistoryAgentCard entry={entry} />)
    // CheckCircle is rendered with text-green-500
    const icons = document.querySelectorAll('.text-green-500')
    expect(icons.length).toBeGreaterThan(0)
  })

  it('shows red X for fail result', () => {
    const entry = makeEntry({ result: 'fail' })
    render(<HistoryAgentCard entry={entry} />)
    const icons = document.querySelectorAll('.text-red-500')
    expect(icons.length).toBeGreaterThan(0)
  })

  // Session badge tests
  it('shows message count badge when session has messages', () => {
    const entry = makeEntry()
    const session = makeSession({ message_count: 12 })
    render(<HistoryAgentCard entry={entry} session={session} />)
    expect(screen.getByText('12 msgs')).toBeInTheDocument()
  })

  it('hides message count badge when session has no messages', () => {
    const entry = makeEntry()
    const session = makeSession({ message_count: 0 })
    render(<HistoryAgentCard entry={entry} session={session} />)
    expect(screen.queryByText(/\d+ msgs?/)).not.toBeInTheDocument()
  })

  it('shows singular msg for 1 message', () => {
    const entry = makeEntry()
    const session = makeSession({ message_count: 1 })
    render(<HistoryAgentCard entry={entry} session={session} />)
    expect(screen.getByText('1 msg')).toBeInTheDocument()
  })

  // Combined context_left and duration
  it('shows both duration and context_left together', () => {
    const entry = makeEntry({
      started_at: '2026-01-01T00:00:00Z',
      ended_at: '2026-01-01T00:05:00Z',
      context_left: 42,
    })
    render(<HistoryAgentCard entry={entry} />)
    expect(screen.getByText('5m')).toBeInTheDocument()
    expect(screen.getByText('42%')).toBeInTheDocument()
  })
})
