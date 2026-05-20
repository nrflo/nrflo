import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { TicketCard } from './TicketCard'
import type { PendingTicket } from '@/types/ticket'

function renderCard(ticket: PendingTicket) {
  return render(
    <MemoryRouter>
      <TicketCard ticket={ticket} />
    </MemoryRouter>
  )
}

function createMockTicket(overrides: Partial<PendingTicket> = {}): PendingTicket {
  return {
    id: 'TICKET-123',
    title: 'Test ticket',
    description: 'Test description',
    status: 'in_progress',
    priority: 2,
    issue_type: 'feature',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    closed_at: null,
    created_by: 'test-user',
    close_reason: null,
    is_blocked: false,
    ...overrides,
  }
}

const LONG_UNBROKEN = 'A'.repeat(200)

describe('TicketCard - flex overflow guard classes', () => {
  it('title h3 has truncate and break-words', () => {
    const { container } = renderCard(createMockTicket({ title: LONG_UNBROKEN }))
    const h3 = container.querySelector('h3')
    expect(h3).toHaveClass('truncate')
    expect(h3).toHaveClass('break-words')
  })

  it('description p has line-clamp-2 and break-words', () => {
    const { container } = renderCard(createMockTicket({ description: LONG_UNBROKEN }))
    const p = container.querySelector('p.line-clamp-2')
    expect(p).toHaveClass('line-clamp-2')
    expect(p).toHaveClass('break-words')
  })
})
