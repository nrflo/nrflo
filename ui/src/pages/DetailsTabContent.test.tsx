import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { DetailsTabContent } from './DetailsTabContent'
import type { TicketWithDeps, Ticket } from '@/types/ticket'

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

const childTicket1: Ticket = {
  id: 'TICK-101',
  title: 'First child ticket',
  description: 'Child description',
  status: 'open',
  priority: 1,
  issue_type: 'task',
  created_at: '2026-01-03T00:00:00Z',
  updated_at: '2026-01-03T00:00:00Z',
  closed_at: null,
  created_by: 'bob',
  close_reason: null,
}

const childTicket2: Ticket = {
  id: 'TICK-102',
  title: 'Second child ticket',
  description: null,
  status: 'in_progress',
  priority: 3,
  issue_type: 'bug',
  created_at: '2026-01-04T00:00:00Z',
  updated_at: '2026-01-04T00:00:00Z',
  closed_at: null,
  created_by: 'charlie',
  close_reason: null,
}

const childTicket3: Ticket = {
  id: 'TICK-103',
  title: 'Third child ticket',
  description: null,
  status: 'closed',
  priority: 4,
  issue_type: 'task',
  created_at: '2026-01-05T00:00:00Z',
  updated_at: '2026-01-05T00:00:00Z',
  closed_at: '2026-01-06T00:00:00Z',
  created_by: 'dave',
  close_reason: 'done',
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

  describe('Parent epic link', () => {
    it('renders parent epic link when parent_ticket_id is set', () => {
      const ticketWithParent: TicketWithDeps = {
        ...baseTicket,
        parent_ticket_id: 'EPIC-1',
      }
      renderPage(ticketWithParent)
      expect(screen.getByText('Parent Epic')).toBeInTheDocument()
      const link = screen.getByText('EPIC-1')
      expect(link).toBeInTheDocument()
      expect(link.closest('a')).toHaveAttribute('href', '/tickets/EPIC-1')
    })

    it('does not render parent epic section when no parent', () => {
      renderPage()
      expect(screen.queryByText('Parent Epic')).not.toBeInTheDocument()
    })
  })

  describe('Children section', () => {
    it('renders Children section for epic ticket with children', () => {
      const epicTicket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'epic',
        children: [childTicket1, childTicket2],
      }
      renderPage(epicTicket)
      expect(screen.getByText('Children')).toBeInTheDocument()
    })

    it('renders all child tickets in Children section', () => {
      const epicTicket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'epic',
        children: [childTicket1, childTicket2, childTicket3],
      }
      renderPage(epicTicket)
      expect(screen.getByText('TICK-101')).toBeInTheDocument()
      expect(screen.getByText('First child ticket')).toBeInTheDocument()
      expect(screen.getByText('TICK-102')).toBeInTheDocument()
      expect(screen.getByText('Second child ticket')).toBeInTheDocument()
      expect(screen.getByText('TICK-103')).toBeInTheDocument()
      expect(screen.getByText('Third child ticket')).toBeInTheDocument()
    })

    it('renders status badge for each child ticket', () => {
      const epicTicket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'epic',
        children: [childTicket1, childTicket2, childTicket3],
      }
      renderPage(epicTicket)
      // Status badges: 'open', 'in progress', 'closed'
      expect(screen.getAllByText('open')).toHaveLength(2) // baseTicket + childTicket1
      expect(screen.getByText('in progress')).toBeInTheDocument()
      expect(screen.getByText('closed')).toBeInTheDocument()
    })

    it('renders priority label for each child ticket', () => {
      const epicTicket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'epic',
        children: [childTicket1, childTicket2, childTicket3],
      }
      renderPage(epicTicket)
      expect(screen.getByText('Critical')).toBeInTheDocument() // priority 1
      expect(screen.getByText('Medium')).toBeInTheDocument() // priority 3
      expect(screen.getByText('Low')).toBeInTheDocument() // priority 4
    })

    it('child ticket IDs link to correct detail pages', () => {
      const epicTicket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'epic',
        children: [childTicket1, childTicket2],
      }
      renderPage(epicTicket)

      const link1 = screen.getByText('TICK-101').closest('a')
      expect(link1).toHaveAttribute('href', '/tickets/TICK-101')

      const link2 = screen.getByText('TICK-102').closest('a')
      expect(link2).toHaveAttribute('href', '/tickets/TICK-102')
    })

    it('does not render Children section for epic ticket with empty children array', () => {
      const epicTicket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'epic',
        children: [],
      }
      renderPage(epicTicket)
      expect(screen.queryByText('Children')).not.toBeInTheDocument()
    })

    it('does not render Children section for epic ticket with undefined children', () => {
      const epicTicket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'epic',
        children: undefined,
      }
      renderPage(epicTicket)
      expect(screen.queryByText('Children')).not.toBeInTheDocument()
    })

    it('does not render Children section for non-epic ticket even with children', () => {
      const featureTicket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'feature',
        children: [childTicket1, childTicket2],
      }
      renderPage(featureTicket)
      expect(screen.queryByText('Children')).not.toBeInTheDocument()
    })

    it('does not render Children section for task ticket', () => {
      const taskTicket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'task',
      }
      renderPage(taskTicket)
      expect(screen.queryByText('Children')).not.toBeInTheDocument()
    })

    it('does not render Children section for bug ticket', () => {
      const bugTicket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'bug',
      }
      renderPage(bugTicket)
      expect(screen.queryByText('Children')).not.toBeInTheDocument()
    })
  })

  describe('Full flow: epic with parent and children', () => {
    it('renders both parent epic link and children section', () => {
      const epicTicket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'epic',
        parent_ticket_id: 'MEGA-EPIC-1',
        children: [childTicket1],
      }
      renderPage(epicTicket)

      // Parent epic link
      expect(screen.getByText('Parent Epic')).toBeInTheDocument()
      expect(screen.getByText('MEGA-EPIC-1')).toBeInTheDocument()

      // Children section
      expect(screen.getByText('Children')).toBeInTheDocument()
      expect(screen.getByText('TICK-101')).toBeInTheDocument()
      expect(screen.getByText('First child ticket')).toBeInTheDocument()
    })
  })
})
