import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { Table, TableBody } from '@/components/ui/Table'
import { renderWithQuery } from '@/test/utils'
import { SystemAgentRunDelegationGroup } from './SystemAgentRunDelegationGroup'
import type { SystemAgentRunDelegationGroup as DelegationGroup } from '@/lib/systemAgentRunGroups'
import type { SystemAgentRun } from '@/types/systemAgentRuns'

function makeWorker(overrides: Partial<SystemAgentRun> = {}): SystemAgentRun {
  return {
    kind: 'agent_session',
    session_id: 'worker-1',
    agent_type: 'executor',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeGroup(overrides: Partial<DelegationGroup> = {}): DelegationGroup {
  const workers = overrides.workers ?? [makeWorker()]
  return {
    kind: 'delegation_group',
    delegation_id: 'delegation-abc',
    caller_session_id: 'caller-session-123',
    delegate_tier: 'executor',
    fanout: 2,
    workers,
    input_tokens: 100,
    output_tokens: 50,
    cost_estimate: 0.0123,
    status: 'completed',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderGroup(group: DelegationGroup) {
  return renderWithQuery(
    <MemoryRouter>
      <Table>
        <TableBody>
          <SystemAgentRunDelegationGroup group={group} />
        </TableBody>
      </Table>
    </MemoryRouter>
  )
}

describe('SystemAgentRunDelegationGroup', () => {
  it('renders header tier, "n of N" workers, caller link, and aggregate tokens/cost, with workers hidden by default', () => {
    renderGroup(
      makeGroup({
        workers: [makeWorker({ session_id: 'w1' })],
        fanout: 2,
      })
    )

    expect(screen.getByText('executor')).toBeInTheDocument()
    expect(screen.getByText('1 of 2 workers')).toBeInTheDocument()
    expect(screen.getByText('100 in / 50 out')).toBeInTheDocument()
    expect(screen.getByText('$0.0123')).toBeInTheDocument()
    expect(screen.getByText('completed')).toBeInTheDocument()

    const callerLink = screen.getByRole('link', { name: /caller-s/ })
    expect(callerLink).toHaveAttribute('href', '/project-workflows')

    // Worker rows are collapsed by default: only the header's "executor"
    // tier badge is present, and the toggle still reads "Expand".
    expect(screen.getByRole('button', { name: /expand/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /collapse/i })).not.toBeInTheDocument()
  })

  it('reveals worker rows when the chevron is clicked, and hides them again on a second click', async () => {
    const user = userEvent.setup()
    renderGroup(
      makeGroup({
        workers: [makeWorker({ session_id: 'w1' }), makeWorker({ session_id: 'w2' })],
        fanout: 2,
      })
    )

    const toggle = screen.getByRole('button', { name: /expand/i })
    await user.click(toggle)

    // Header tier badge + 2 worker agent-label cells.
    expect(screen.getAllByText('executor')).toHaveLength(3)
    expect(screen.getByRole('button', { name: /collapse/i })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /collapse/i }))
    expect(screen.queryByRole('button', { name: /^collapse$/i })).not.toBeInTheDocument()
  })

  it('links to the ticket workflow tab when the anchor worker carries a ticket_id', () => {
    renderGroup(
      makeGroup({
        workers: [makeWorker({ ticket_id: 'ticket-xyz' })],
      })
    )
    expect(screen.getByRole('link', { name: /caller-s/ })).toHaveAttribute(
      'href',
      '/tickets/ticket-xyz?tab=workflow'
    )
  })
})
