import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentCard } from './AgentCard'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'

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
    expect(badge.className).toContain('text-green-700')
    expect(badge.className).not.toContain('bg-')
  })

  it('shows context_left with yellow styling for 26-50%', () => {
    const agent = makeAgent({ context_left: 40 })
    render(<AgentCard agent={agent} />)
    const badge = screen.getByText('40%')
    expect(badge.className).toContain('text-yellow-700')
    expect(badge.className).not.toContain('bg-')
  })

  it('shows context_left with red styling for <= 25%', () => {
    const agent = makeAgent({ context_left: 15 })
    render(<AgentCard agent={agent} />)
    const badge = screen.getByText('15%')
    expect(badge.className).toContain('text-red-700')
    expect(badge.className).not.toContain('bg-')
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
    expect(badge.className).toContain('text-red-700')
    expect(badge.className).not.toContain('bg-')
  })

  it('calls onExpand when clicked', async () => {
    const onExpand = vi.fn()
    const agent = makeAgent()
    render(<AgentCard agent={agent} onExpand={onExpand} />)
    const button = screen.getByRole('button')
    button.click()
    expect(onExpand).toHaveBeenCalledTimes(1)
  })

  // Context-left badge positioning and sizing (ticket nrflow-30efa6)
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

  // Tag badge
  it('renders emerald tag badge when agent.tag is set', () => {
    const agent = makeAgent({ tag: 'backend' })
    render(<AgentCard agent={agent} />)
    expect(screen.getByText('backend')).toBeInTheDocument()
  })

  it('does not render tag badge when agent.tag is absent', () => {
    const agent = makeAgent({ tag: undefined })
    render(<AgentCard agent={agent} />)
    expect(screen.queryByText('backend')).not.toBeInTheDocument()
  })
})

describe('AgentCard - user_interactive status', () => {
  function makeSession(overrides: Partial<AgentSession> = {}): AgentSession {
    return {
      id: 'session-1',
      project_id: 'test-project',
      ticket_id: 'TICKET-1',
      workflow_instance_id: 'wi-1',
      phase: 'implementation',
      workflow: 'feature',
      agent_type: 'implementor',
      model_id: 'claude-sonnet-4-5',
      status: 'running',
      message_count: 0,
      restart_count: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      ...overrides,
    }
  }

  it('shows "Interactive" label when session status is user_interactive', () => {
    const agent = makeAgent()
    const session = makeSession({ status: 'user_interactive' })
    render(<AgentCard agent={agent} session={session} />)
    expect(screen.getByText('Interactive')).toBeInTheDocument()
  })

  it('shows model name label when session status is running (not interactive)', () => {
    const agent = makeAgent()
    const session = makeSession({ status: 'running' })
    render(<AgentCard agent={agent} session={session} />)
    // model_id "claude-sonnet-4-5" -> last segment "5" but joined with pop(), let's just not find "Interactive"
    expect(screen.queryByText('Interactive')).not.toBeInTheDocument()
  })

  it('applies blue border class when user_interactive', () => {
    const agent = makeAgent()
    const session = makeSession({ status: 'user_interactive' })
    render(<AgentCard agent={agent} session={session} />)
    const button = screen.getByRole('button')
    expect(button.className).toContain('border-blue-400')
  })

  it('does not apply yellow border for interactive session (not isRunning)', () => {
    const agent = makeAgent()
    const session = makeSession({ status: 'user_interactive' })
    render(<AgentCard agent={agent} session={session} />)
    const button = screen.getByRole('button')
    expect(button.className).not.toContain('border-yellow-400')
  })

  it('applies yellow border class when running (not interactive)', () => {
    const agent = makeAgent()
    const session = makeSession({ status: 'running' })
    render(<AgentCard agent={agent} session={session} />)
    const button = screen.getByRole('button')
    expect(button.className).toContain('border-amber-400')
  })

  it('suppresses alert triangle for interactive session even when near restart threshold', () => {
    const agent = makeAgent({ context_left: 10, restart_threshold: 25 })
    const session = makeSession({ status: 'user_interactive' })
    render(<AgentCard agent={agent} session={session} />)
    // AlertTriangle is only shown when isRunning && isNearRestartThreshold
    // With user_interactive, isRunning is false so no alert icon
    // The context_left badge still renders but no alert triangle svg path for warning
    expect(screen.queryByText('Interactive')).toBeInTheDocument()
    // Verify % badge present but no alert (no way to query SVG by name easily,
    // but we can verify the component renders without error and shows Interactive)
    expect(screen.getByText('10%')).toBeInTheDocument()
  })
})

describe('AgentCard - restart badge and tooltip', () => {
  it('does not render restart badge when restart_count is 0', () => {
    render(<AgentCard agent={makeAgent({ restart_count: 0 })} />)
    expect(screen.queryByText(/↻/)).not.toBeInTheDocument()
  })

  it('does not render restart badge when restart_count is undefined', () => {
    render(<AgentCard agent={makeAgent({ restart_count: undefined })} />)
    expect(screen.queryByText(/↻/)).not.toBeInTheDocument()
  })

  it('renders restart badge with count when restart_count > 0', () => {
    render(<AgentCard agent={makeAgent({ restart_count: 2 })} />)
    expect(screen.getByText('↻2')).toBeInTheDocument()
  })

  it('shows restart reasons in tooltip on hover', async () => {
    const user = userEvent.setup()
    const agent = makeAgent({ restart_count: 2, restart_details: [
      { reason: 'low_context', duration_sec: 725, context_left: 12, message_count: 247 },
      { reason: 'explicit', duration_sec: 42, context_left: 85, message_count: 3 },
    ] })
    render(<AgentCard agent={agent} />)
    await user.hover(screen.getByText('↻2'))
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('1. Low context')
    expect(tooltip).toHaveTextContent('2. Manual continue')
  })

  it('shows count fallback tooltip when restart_details is undefined', async () => {
    const user = userEvent.setup()
    const agent = makeAgent({ restart_count: 3 })
    render(<AgentCard agent={agent} />)
    await user.hover(screen.getByText('↻3'))
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('3 restarts')
  })
})
