import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConflictResolverBanner } from './ConflictResolverBanner'
import type { AgentSession, AgentHistoryEntry } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'

function makeSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 'sess-1',
    project_id: 'proj-1',
    ticket_id: 'tick-1',
    workflow_instance_id: 'inst-1',
    phase: '_conflict_resolution',
    workflow: '_conflict_resolution',
    agent_type: 'conflict-resolver',
    status: 'running',
    message_count: 0,
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeHistoryEntry(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'agent-1',
    agent_type: 'conflict-resolver',
    phase: '_conflict_resolution',
    ...overrides,
  }
}

describe('ConflictResolverBanner', () => {
  const onAgentSelect = vi.fn()

  it('renders nothing when no conflict-resolver session exists', () => {
    const { container } = render(
      <ConflictResolverBanner sessions={[]} agentHistory={[]} onAgentSelect={onAgentSelect} />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when sessions/agentHistory contain other agent types', () => {
    const otherSession = makeSession({ agent_type: 'implementor', status: 'running' })
    const otherHistory = makeHistoryEntry({ agent_type: 'qa-verifier', result: 'fail' })
    const { container } = render(
      <ConflictResolverBanner
        sessions={[otherSession]}
        agentHistory={[otherHistory]}
        onAgentSelect={onAgentSelect}
      />
    )
    expect(container.firstChild).toBeNull()
  })

  it('shows amber running banner when conflict-resolver session is running', () => {
    render(
      <ConflictResolverBanner
        sessions={[makeSession({ status: 'running' })]}
        agentHistory={[]}
        onAgentSelect={onAgentSelect}
      />
    )
    expect(screen.getByText('Resolving Merge Conflict...')).toBeInTheDocument()
  })

  it('shows green resolved banner when session result is pass', () => {
    render(
      <ConflictResolverBanner
        sessions={[makeSession({ status: 'completed', result: 'pass' })]}
        agentHistory={[]}
        onAgentSelect={onAgentSelect}
      />
    )
    expect(screen.getByText('Merge Conflict Resolved')).toBeInTheDocument()
  })

  it('shows orange failed banner when session result is fail', () => {
    render(
      <ConflictResolverBanner
        sessions={[makeSession({ status: 'failed', result: 'fail' })]}
        agentHistory={[]}
        onAgentSelect={onAgentSelect}
      />
    )
    expect(screen.getByText('Merge Conflict Unresolved')).toBeInTheDocument()
    expect(screen.getByText('(branch preserved for manual merge)')).toBeInTheDocument()
  })

  it('shows green resolved banner from agentHistory (ticket-scoped path)', () => {
    render(
      <ConflictResolverBanner
        sessions={[]}
        agentHistory={[makeHistoryEntry({ result: 'pass' })]}
        onAgentSelect={onAgentSelect}
      />
    )
    expect(screen.getByText('Merge Conflict Resolved')).toBeInTheDocument()
  })

  it('shows orange failed banner from agentHistory (ticket-scoped path)', () => {
    render(
      <ConflictResolverBanner
        sessions={[]}
        agentHistory={[makeHistoryEntry({ result: 'fail' })]}
        onAgentSelect={onAgentSelect}
      />
    )
    expect(screen.getByText('Merge Conflict Unresolved')).toBeInTheDocument()
  })

  describe('click behavior', () => {
    it('calls onAgentSelect with running session data on click', async () => {
      const user = userEvent.setup()
      const session = makeSession({ status: 'running' })
      render(
        <ConflictResolverBanner
          sessions={[session]}
          agentHistory={[]}
          onAgentSelect={onAgentSelect}
        />
      )
      await user.click(screen.getByText('Resolving Merge Conflict...'))
      expect(onAgentSelect).toHaveBeenCalledWith<[SelectedAgentData]>({
        phaseName: '_conflict_resolution',
        session,
      })
    })

    it('calls onAgentSelect with completed session data on click', async () => {
      const user = userEvent.setup()
      const session = makeSession({ status: 'completed', result: 'pass' })
      render(
        <ConflictResolverBanner
          sessions={[session]}
          agentHistory={[]}
          onAgentSelect={onAgentSelect}
        />
      )
      await user.click(screen.getByText('Merge Conflict Resolved'))
      expect(onAgentSelect).toHaveBeenCalledWith<[SelectedAgentData]>({
        phaseName: '_conflict_resolution',
        session,
      })
    })

    it('calls onAgentSelect with historyEntry data when only agentHistory has resolver', async () => {
      const user = userEvent.setup()
      const entry = makeHistoryEntry({ result: 'fail' })
      render(
        <ConflictResolverBanner
          sessions={[]}
          agentHistory={[entry]}
          onAgentSelect={onAgentSelect}
        />
      )
      await user.click(screen.getByText('Merge Conflict Unresolved'))
      expect(onAgentSelect).toHaveBeenCalledWith<[SelectedAgentData]>({
        phaseName: '_conflict_resolution',
        historyEntry: entry,
      })
    })
  })

  it('running session takes priority over completed session', () => {
    const running = makeSession({ id: 'run', status: 'running' })
    const completed = makeSession({ id: 'done', status: 'completed', result: 'pass' })
    render(
      <ConflictResolverBanner
        sessions={[running, completed]}
        agentHistory={[]}
        onAgentSelect={onAgentSelect}
      />
    )
    expect(screen.getByText('Resolving Merge Conflict...')).toBeInTheDocument()
  })
})
