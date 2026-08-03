import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Table, TableBody } from '@/components/ui/Table'
import { SessionCostRollup } from './SessionCostRollup'
import type { SessionStatsResponse } from '@/types/session'

function makeStats(overrides: Partial<SessionStatsResponse> = {}): SessionStatsResponse {
  return {
    root_session_id: 'session-abc',
    tool_calls: [],
    self_cost_usd: 0.1,
    subtree_cost_usd: 0.5,
    self_tokens: 1000,
    subtree_tokens: 5000,
    ...overrides,
  }
}

function renderRollup(stats?: SessionStatsResponse) {
  return render(
    <Table>
      <TableBody>
        <SessionCostRollup stats={stats} />
      </TableBody>
    </Table>
  )
}

describe('SessionCostRollup', () => {
  it('renders nothing when stats is undefined', () => {
    const { container } = renderRollup(undefined)
    expect(container.querySelector('table')?.textContent).toBe('')
  })

  it('renders self/subtree cost and token rows', () => {
    renderRollup(makeStats({ self_cost_usd: 0.1, subtree_cost_usd: 0.5, self_tokens: 1000, subtree_tokens: 5000 }))
    expect(screen.getByText('Self')).toBeInTheDocument()
    expect(screen.getByText('Subtree')).toBeInTheDocument()
    expect(screen.getByText('$0.1000')).toBeInTheDocument()
    expect(screen.getByText('$0.5000')).toBeInTheDocument()
    expect(screen.getByText('1,000')).toBeInTheDocument()
    expect(screen.getByText('5,000')).toBeInTheDocument()
  })
})
