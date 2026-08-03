import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { SessionsTable } from './SessionsTable'
import type { SessionListRow } from '@/types/session'

function makeRow(overrides: Partial<SessionListRow> = {}): SessionListRow {
  return {
    session_id: 'session-abcdef1234',
    kind: 'delegate',
    status: 'completed',
    ...overrides,
  }
}

describe('SessionsTable', () => {
  it('shows an empty state with no sessions', () => {
    renderWithQuery(<SessionsTable sessions={[]} onSelect={vi.fn()} />)
    expect(screen.getByText('No sessions')).toBeInTheDocument()
  })

  it('renders row fields: truncated sid, kind badge, agent, model, status, cost', () => {
    renderWithQuery(
      <SessionsTable
        sessions={[
          makeRow({
            session_id: 'session-abcdef1234',
            kind: 'agent',
            agent_type: 'implementor',
            model_id: 'claude-sonnet-4-5',
            status: 'completed',
            cost_estimate: 0.125,
          }),
        ]}
        onSelect={vi.fn()}
      />
    )
    expect(screen.getByText('session-')).toBeInTheDocument()
    expect(screen.getByText('agent')).toBeInTheDocument()
    expect(screen.getByText('implementor')).toBeInTheDocument()
    expect(screen.getByText('claude-sonnet-4-5')).toBeInTheDocument()
    expect(screen.getByText('$0.1250')).toBeInTheDocument()
  })

  it('shows em-dash when agent and cost are missing', () => {
    renderWithQuery(
      <SessionsTable
        sessions={[makeRow({ agent_type: undefined, cost_estimate: undefined })]}
        onSelect={vi.fn()}
      />
    )
    const dashes = screen.getAllByText('—')
    expect(dashes.length).toBeGreaterThan(0)
  })

  it('calls onSelect with the session id when a row is clicked', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    renderWithQuery(
      <SessionsTable sessions={[makeRow({ session_id: 'session-xyz' })]} onSelect={onSelect} />
    )
    await user.click(screen.getByText('session-'))
    expect(onSelect).toHaveBeenCalledWith('session-xyz')
  })

  it('marks the selected row with data-state="selected"', () => {
    renderWithQuery(
      <SessionsTable
        sessions={[makeRow({ session_id: 'session-xyz' })]}
        selectedId="session-xyz"
        onSelect={vi.fn()}
      />
    )
    expect(screen.getByText('session-').closest('tr')).toHaveAttribute('data-state', 'selected')
  })
})
