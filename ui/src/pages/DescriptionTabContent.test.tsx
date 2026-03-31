import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { DescriptionTabContent } from './DescriptionTabContent'
import type { TicketWithDeps } from '@/types/ticket'

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
  return render(
    <MemoryRouter>
      <DescriptionTabContent ticket={ticket} />
    </MemoryRouter>
  )
}

describe('DescriptionTabContent', () => {
  it('renders ticket title as heading', () => {
    renderPage()
    expect(screen.getByText('Test ticket')).toBeInTheDocument()
  })

  it('renders description text', () => {
    renderPage()
    expect(screen.getByText('Some description')).toBeInTheDocument()
  })

  it('renders markdown description as formatted HTML', () => {
    renderPage({ ...baseTicket, description: '# My Heading\n\nSome **bold** text.' })
    expect(screen.getByRole('heading', { level: 1, name: 'My Heading' })).toBeInTheDocument()
  })

  it('renders "No description" when description is null', () => {
    renderPage({ ...baseTicket, description: null })
    expect(screen.getByText('No description')).toBeInTheDocument()
  })

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
    expect(screen.getByText('user')).toBeInTheDocument()
  })

  it('renders status badge', () => {
    renderPage()
    expect(screen.getByText('open')).toBeInTheDocument()
  })

  it('renders close reason when present', () => {
    renderPage({ ...baseTicket, close_reason: 'Duplicate of TICK-50' })
    expect(screen.getByText('Duplicate of TICK-50')).toBeInTheDocument()
  })

  it('does not render close reason section when not present', () => {
    renderPage()
    expect(screen.queryByText('Close reason')).not.toBeInTheDocument()
  })

  it('renders created timestamp', () => {
    renderPage()
    // Using more specific matcher since both created and updated show Jan 1
    expect(screen.getAllByText(/Jan 1, 2026/).length).toBeGreaterThan(0)
  })

  it('renders updated timestamp', () => {
    const ticket: TicketWithDeps = {
      ...baseTicket,
      updated_at: '2026-01-05T12:30:00Z',
    }
    renderPage(ticket)
    expect(screen.getByText(/Jan 5, 2026/)).toBeInTheDocument()
  })

  it('renders closed timestamp when ticket is closed', () => {
    const ticket: TicketWithDeps = {
      ...baseTicket,
      status: 'closed',
      closed_at: '2026-01-10T14:45:00Z',
    }
    renderPage(ticket)
    expect(screen.getByText(/Jan 10, 2026/)).toBeInTheDocument()
  })

  it('does not render closed timestamp when ticket is open', () => {
    renderPage()
    expect(screen.queryByText('Closed')).not.toBeInTheDocument()
  })
})
