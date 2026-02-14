import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { DetailsTabContent } from './DetailsTabContent'
import type { TicketWithDeps } from '@/types/ticket'

const baseTicket: TicketWithDeps = {
  id: 'TICK-100',
  title: 'Test ticket',
  description: 'Some description',
  status: 'open',
  priority: 2,
  issue_type: 'feature',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
  closed_at: null,
  created_by: 'alice',
  close_reason: null,
  blockers: [],
  blocks: [],
}

function renderPage(ticket: TicketWithDeps = baseTicket) {
  return render(
    <MemoryRouter>
      <DetailsTabContent ticket={ticket} />
    </MemoryRouter>
  )
}

describe('DetailsTabContent', () => {
  describe('Basic metadata rendering', () => {
    it('renders priority label', () => {
      renderPage()
      expect(screen.getByText('High')).toBeInTheDocument()
    })

    it('renders issue type', () => {
      renderPage()
      expect(screen.getByText('feature')).toBeInTheDocument()
    })

    it('renders created by', () => {
      renderPage()
      expect(screen.getByText('alice')).toBeInTheDocument()
    })

    it('renders status badge', () => {
      renderPage()
      expect(screen.getByText('open')).toBeInTheDocument()
    })

    it('renders created date', () => {
      renderPage()
      expect(screen.getByText(/Jan 1, 2026/)).toBeInTheDocument()
    })

    it('renders updated date', () => {
      renderPage()
      expect(screen.getByText(/Jan 2, 2026/)).toBeInTheDocument()
    })

    it('renders closed date when ticket is closed', () => {
      const closedTicket: TicketWithDeps = {
        ...baseTicket,
        status: 'closed',
        closed_at: '2026-01-10T12:30:00Z',
      }
      renderPage(closedTicket)
      expect(screen.getByText(/Jan 10, 2026/)).toBeInTheDocument()
    })

    it('does not render closed date when ticket is open', () => {
      renderPage()
      expect(screen.queryByText('Closed')).not.toBeInTheDocument()
    })

    it('renders close reason when present', () => {
      const ticketWithReason: TicketWithDeps = {
        ...baseTicket,
        close_reason: 'Duplicate of TICK-50',
      }
      renderPage(ticketWithReason)
      expect(screen.getByText('Duplicate of TICK-50')).toBeInTheDocument()
    })

    it('does not render close reason section when not present', () => {
      renderPage()
      expect(screen.queryByText('Close reason')).not.toBeInTheDocument()
    })
  })

  describe('Description text', () => {
    it('renders description text', () => {
      renderPage()
      expect(screen.getByText('Some description')).toBeInTheDocument()
    })

    it('renders "No description" when description is null', () => {
      renderPage({ ...baseTicket, description: null })
      expect(screen.getByText('No description')).toBeInTheDocument()
    })
  })

  describe('Dependencies display', () => {
    it('renders blockers when present', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blockers: [
          { issue_id: 'TICK-100', depends_on_id: 'TICK-50', type: 'blocks', created_at: '2026-01-01T00:00:00Z', created_by: 'user' },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('Blocked by')).toBeInTheDocument()
      expect(screen.getByText('TICK-50')).toBeInTheDocument()
    })

    it('renders blocks when present', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blocks: [
          { issue_id: 'TICK-200', depends_on_id: 'TICK-100', type: 'blocks', created_at: '2026-01-01T00:00:00Z', created_by: 'user' },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('Blocks')).toBeInTheDocument()
      expect(screen.getByText('TICK-200')).toBeInTheDocument()
    })

    it('does not render dependencies section when no blockers or blocks', () => {
      renderPage()
      expect(screen.queryByText('Dependencies')).not.toBeInTheDocument()
    })

    it('renders blocker titles when provided', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blockers: [
          {
            issue_id: 'TICK-100',
            depends_on_id: 'TICK-50',
            depends_on_title: 'Fix auth bug',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('Fix auth bug')).toBeInTheDocument()
    })

    it('renders blocker IDs without titles', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blockers: [
          {
            issue_id: 'TICK-100',
            depends_on_id: 'TICK-50',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('TICK-50')).toBeInTheDocument()
    })

    it('renders block titles when provided', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blocks: [
          {
            issue_id: 'TICK-200',
            depends_on_id: 'TICK-100',
            issue_title: 'Deploy to prod',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('Deploy to prod')).toBeInTheDocument()
    })

    it('renders blocker link with correct href', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blockers: [
          {
            issue_id: 'TICK-100',
            depends_on_id: 'TICK-50',
            depends_on_title: 'Fix auth bug',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      const { container } = renderPage(ticket)
      const link = container.querySelector('a[href="/tickets/TICK-50"]')
      expect(link).toBeInTheDocument()
    })

    it('renders blocks link with correct href', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blocks: [
          {
            issue_id: 'TICK-200',
            depends_on_id: 'TICK-100',
            issue_title: 'Deploy to prod',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      const { container } = renderPage(ticket)
      const link = container.querySelector('a[href="/tickets/TICK-200"]')
      expect(link).toBeInTheDocument()
    })
  })
})
