import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HierarchyTabContent } from './HierarchyTabContent'
import type { TicketWithDeps } from '@/types/ticket'

vi.mock('@/api/tickets', () => ({
  addDependency: vi.fn(),
  removeDependency: vi.fn(),
}))

const baseTicket: TicketWithDeps = {
  id: 'TICK-100',
  title: 'Test ticket',
  description: 'Some description',
  status: 'open',
  priority: 2,
  issue_type: 'feature',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  closed_at: null,
  created_by: 'user',
  close_reason: null,
  blockers: [],
  blocks: [],
}

function renderPage(ticket: TicketWithDeps = baseTicket) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <HierarchyTabContent ticket={ticket} />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

const LONG_UNBROKEN = 'X'.repeat(200)

describe('HierarchyTabContent - flex overflow guard classes', () => {
  describe('blockers row', () => {
    const ticketWithLongBlocker: TicketWithDeps = {
      ...baseTicket,
      blockers: [
        {
          issue_id: 'TICK-100',
          depends_on_id: 'TICK-50',
          depends_on_title: LONG_UNBROKEN,
          type: 'blocks',
          created_at: '2026-01-01T00:00:00Z',
          created_by: 'user',
        },
      ],
    }

    it('blocker Link has min-w-0', () => {
      const { container } = renderPage(ticketWithLongBlocker)
      const link = container.querySelector('a[href="/tickets/TICK-50"]')
      expect(link).toHaveClass('min-w-0')
    })

    it('blocker title span has truncate', () => {
      const { container } = renderPage(ticketWithLongBlocker)
      const link = container.querySelector('a[href="/tickets/TICK-50"]')
      const spans = link?.querySelectorAll('span')
      // spans[0] = id (shrink-0 font-mono), spans[1] = title (truncate)
      expect(spans?.[1]).toHaveClass('truncate')
    })

    it('outer row div has min-w-0', () => {
      const { container } = renderPage(ticketWithLongBlocker)
      // The outer div wrapping Link + remove button
      const link = container.querySelector('a[href="/tickets/TICK-50"]')
      const rowDiv = link?.parentElement
      expect(rowDiv).toHaveClass('min-w-0')
    })
  })

  describe('blocks row', () => {
    const ticketWithLongBlock: TicketWithDeps = {
      ...baseTicket,
      blocks: [
        {
          issue_id: 'TICK-200',
          depends_on_id: 'TICK-100',
          issue_title: LONG_UNBROKEN,
          type: 'blocks',
          created_at: '2026-01-01T00:00:00Z',
          created_by: 'user',
        },
      ],
    }

    it('blocks Link has min-w-0', () => {
      const { container } = renderPage(ticketWithLongBlock)
      const link = container.querySelector('a[href="/tickets/TICK-200"]')
      expect(link).toHaveClass('min-w-0')
    })

    it('blocks title span has truncate', () => {
      const { container } = renderPage(ticketWithLongBlock)
      const link = container.querySelector('a[href="/tickets/TICK-200"]')
      const spans = link?.querySelectorAll('span')
      expect(spans?.[1]).toHaveClass('truncate')
    })
  })

  describe('sibling row', () => {
    const ticketWithLongSibling: TicketWithDeps = {
      ...baseTicket,
      parent_ticket_id: 'EPIC-10',
      siblings: [
        {
          id: 'TICK-101',
          title: LONG_UNBROKEN,
          description: null,
          status: 'open',
          priority: 2,
          issue_type: 'task',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
          closed_at: null,
          created_by: 'user',
          close_reason: null,
        },
      ],
    }

    it('sibling row div has min-w-0', () => {
      const { container } = renderPage(ticketWithLongSibling)
      const siblingLink = container.querySelector('a[href="/tickets/TICK-101"]')
      const rowDiv = siblingLink?.parentElement
      expect(rowDiv).toHaveClass('min-w-0')
    })

    it('sibling title span has truncate', () => {
      const { container } = renderPage(ticketWithLongSibling)
      // The title is a plain span (not inside the link), after the id Link
      const siblingLink = container.querySelector('a[href="/tickets/TICK-101"]')
      const titleSpan = siblingLink?.nextElementSibling
      expect(titleSpan).toHaveClass('truncate')
    })
  })

  describe('children row', () => {
    const ticketWithLongChild: TicketWithDeps = {
      ...baseTicket,
      issue_type: 'epic',
      children: [
        {
          id: 'TICK-201',
          title: LONG_UNBROKEN,
          description: null,
          status: 'open',
          priority: 2,
          issue_type: 'task',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
          closed_at: null,
          created_by: 'user',
          close_reason: null,
        },
      ],
    }

    it('child row div has min-w-0', () => {
      const { container } = renderPage(ticketWithLongChild)
      const childLink = container.querySelector('a[href="/tickets/TICK-201"]')
      const rowDiv = childLink?.parentElement
      expect(rowDiv).toHaveClass('min-w-0')
    })

    it('child title span has truncate', () => {
      const { container } = renderPage(ticketWithLongChild)
      const childLink = container.querySelector('a[href="/tickets/TICK-201"]')
      const titleSpan = childLink?.nextElementSibling
      expect(titleSpan).toHaveClass('truncate')
    })
  })

  describe('parent epic row', () => {
    const ticketWithLongParent: TicketWithDeps = {
      ...baseTicket,
      parent_ticket_id: 'EPIC-10',
      parent_ticket: {
        id: 'EPIC-10',
        title: LONG_UNBROKEN,
        description: null,
        status: 'in_progress',
        priority: 1,
        issue_type: 'epic',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
        closed_at: null,
        created_by: 'user',
        close_reason: null,
      },
    }

    it('parent epic Link has min-w-0', () => {
      const { container } = renderPage(ticketWithLongParent)
      const link = container.querySelector('a[href="/tickets/EPIC-10"]')
      expect(link).toHaveClass('min-w-0')
    })

    it('parent epic title span has truncate', () => {
      const { container } = renderPage(ticketWithLongParent)
      const link = container.querySelector('a[href="/tickets/EPIC-10"]')
      const spans = link?.querySelectorAll('span')
      // spans[0] = id (shrink-0 font-mono), spans[1] = title (truncate)
      expect(spans?.[1]).toHaveClass('truncate')
    })
  })
})
