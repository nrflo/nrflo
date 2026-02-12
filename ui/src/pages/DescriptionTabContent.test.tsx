import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { DescriptionTabContent } from './DescriptionTabContent'
import * as ticketsApi from '@/api/tickets'
import type { TicketWithDeps } from '@/types/ticket'

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return {
    ...actual,
    addDependency: vi.fn().mockResolvedValue({ message: 'ok' }),
    removeDependency: vi.fn().mockResolvedValue({ message: 'ok' }),
  }
})

// Mock useTicketSearch used by TicketSearchDropdown
const mockUseTicketSearch = vi.fn()
vi.mock('@/hooks/useTickets', async () => {
  const actual = await vi.importActual('@/hooks/useTickets')
  return {
    ...actual,
    useTicketSearch: (...args: unknown[]) => mockUseTicketSearch(...args),
  }
})

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
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <DescriptionTabContent ticket={ticket} />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('DescriptionTabContent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTicketSearch.mockReturnValue({ data: undefined, isFetching: false })
  })

  it('renders the ticket search dropdown for adding blockers', () => {
    renderPage()
    expect(screen.getByPlaceholderText('Search tickets to add...')).toBeInTheDocument()
  })

  it('renders description text', () => {
    renderPage()
    expect(screen.getByText('Some description')).toBeInTheDocument()
  })

  it('renders "No description" when description is null', () => {
    renderPage({ ...baseTicket, description: null })
    expect(screen.getByText('No description')).toBeInTheDocument()
  })

  it('renders existing blockers with remove buttons', () => {
    const ticket: TicketWithDeps = {
      ...baseTicket,
      blockers: [
        { issue_id: 'TICK-100', depends_on_id: 'TICK-50', type: 'blocks', created_at: '2026-01-01T00:00:00Z', created_by: 'user' },
        { issue_id: 'TICK-100', depends_on_id: 'TICK-51', type: 'blocks', created_at: '2026-01-01T00:00:00Z', created_by: 'user' },
      ],
    }
    renderPage(ticket)

    expect(screen.getByText('TICK-50')).toBeInTheDocument()
    expect(screen.getByText('TICK-51')).toBeInTheDocument()
    expect(screen.getAllByTitle('Remove blocker')).toHaveLength(2)
  })

  it('passes current ticket id and existing blocker ids as excludeIds', async () => {
    const ticket: TicketWithDeps = {
      ...baseTicket,
      blockers: [
        { issue_id: 'TICK-100', depends_on_id: 'TICK-50', type: 'blocks', created_at: '2026-01-01T00:00:00Z', created_by: 'user' },
      ],
    }

    const searchResults = [
      {
        id: 'TICK-100', title: 'Self', status: 'open' as const, priority: 1, issue_type: 'task' as const,
        description: null, created_at: '', updated_at: '', closed_at: null, created_by: '', close_reason: null, is_blocked: false,
      },
      {
        id: 'TICK-50', title: 'Existing blocker', status: 'open' as const, priority: 1, issue_type: 'task' as const,
        description: null, created_at: '', updated_at: '', closed_at: null, created_by: '', close_reason: null, is_blocked: false,
      },
      {
        id: 'TICK-99', title: 'Available ticket', status: 'open' as const, priority: 1, issue_type: 'task' as const,
        description: null, created_at: '', updated_at: '', closed_at: null, created_by: '', close_reason: null, is_blocked: false,
      },
    ]
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: searchResults },
      isFetching: false,
    })

    renderPage(ticket)
    const user = userEvent.setup()
    await user.type(screen.getByPlaceholderText('Search tickets to add...'), 'ti')

    // Current ticket and existing blocker should be excluded
    expect(screen.queryByText('Self')).not.toBeInTheDocument()
    expect(screen.queryByText('Existing blocker')).not.toBeInTheDocument()
    expect(screen.getByText('Available ticket')).toBeInTheDocument()
  })

  it('calls addDependency when selecting a ticket from the dropdown', async () => {
    const searchResults = [
      {
        id: 'TICK-99', title: 'New blocker', status: 'open' as const, priority: 1, issue_type: 'task' as const,
        description: null, created_at: '', updated_at: '', closed_at: null, created_by: '', close_reason: null, is_blocked: false,
      },
    ]
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: searchResults },
      isFetching: false,
    })

    renderPage()
    const user = userEvent.setup()

    await user.type(screen.getByPlaceholderText('Search tickets to add...'), 'new')
    await user.click(screen.getByText('New blocker'))

    await waitFor(() => {
      expect(ticketsApi.addDependency).toHaveBeenCalledWith({
        issue_id: 'TICK-100',
        depends_on_id: 'TICK-99',
      })
    })
  })

  it('calls removeDependency when clicking remove blocker button', async () => {
    const ticket: TicketWithDeps = {
      ...baseTicket,
      blockers: [
        { issue_id: 'TICK-100', depends_on_id: 'TICK-50', type: 'blocks', created_at: '2026-01-01T00:00:00Z', created_by: 'user' },
      ],
    }
    renderPage(ticket)
    const user = userEvent.setup()

    await user.click(screen.getByTitle('Remove blocker'))

    await waitFor(() => {
      expect(ticketsApi.removeDependency).toHaveBeenCalledWith({
        issue_id: 'TICK-100',
        depends_on_id: 'TICK-50',
      })
    })
  })

  it('renders parent epic link when parent_ticket_id is set', () => {
    renderPage({ ...baseTicket, parent_ticket_id: 'EPIC-1' })
    expect(screen.getByText('Parent Epic')).toBeInTheDocument()
    expect(screen.getByText('EPIC-1')).toBeInTheDocument()
  })

  it('does not render parent epic section when no parent', () => {
    renderPage()
    expect(screen.queryByText('Parent Epic')).not.toBeInTheDocument()
  })

  it('renders "Blocks" section when ticket blocks others', () => {
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

  it('does not render "Blocks" section when ticket has no blocks', () => {
    renderPage()
    expect(screen.queryByText('Blocks')).not.toBeInTheDocument()
  })
})
