import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { ChainTicketSelector } from './ChainTicketSelector'
import type { PendingTicket } from '@/types/ticket'

// Mock useTicketList hook
const mockUseTicketList = vi.fn()
vi.mock('@/hooks/useTickets', () => ({
  useTicketList: (params?: any) => mockUseTicketList(params),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) => {
    const store = {
      currentProject: 'test-project',
      projectsLoaded: true,
    }
    return selector(store)
  }),
}))

function createMockTicket(overrides: Partial<PendingTicket> = {}): PendingTicket {
  return {
    id: 'TICKET-1',
    title: 'Test Ticket',
    description: 'Test description',
    status: 'open',
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

function renderSelector(selectedIds: string[] = [], onChange = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return {
    onChange,
    ...render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <ChainTicketSelector selectedIds={selectedIds} onChange={onChange} />
        </MemoryRouter>
      </QueryClientProvider>
    ),
  }
}

describe('ChainTicketSelector - Render States', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders loading state', () => {
    mockUseTicketList.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    const { container } = renderSelector()

    expect(container.querySelector('[class*="animate-spin"]')).toBeInTheDocument()
  })

  it('shows no tickets message when ticket list is empty', () => {
    mockUseTicketList.mockReturnValue({
      data: { tickets: [] },
      isLoading: false,
    })

    renderSelector()

    expect(screen.getByText(/no open tickets found/i)).toBeInTheDocument()
  })

  it('renders ticket list when tickets are loaded', () => {
    const tickets = [
      createMockTicket({ id: 'TICKET-1', title: 'First Ticket' }),
      createMockTicket({ id: 'TICKET-2', title: 'Second Ticket' }),
    ]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderSelector()

    expect(screen.getByText('TICKET-1')).toBeInTheDocument()
    expect(screen.getByText('First Ticket')).toBeInTheDocument()
    expect(screen.getByText('TICKET-2')).toBeInTheDocument()
    expect(screen.getByText('Second Ticket')).toBeInTheDocument()
  })
})

describe('ChainTicketSelector - Fetch Open Tickets Only', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('fetches only open tickets via useTicketList', () => {
    mockUseTicketList.mockReturnValue({
      data: { tickets: [] },
      isLoading: false,
    })

    renderSelector()

    expect(mockUseTicketList).toHaveBeenCalledWith({ status: 'open' })
  })
})

describe('ChainTicketSelector - Search Filter', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders search input', () => {
    mockUseTicketList.mockReturnValue({
      data: { tickets: [] },
      isLoading: false,
    })

    renderSelector()

    expect(screen.getByPlaceholderText(/search tickets/i)).toBeInTheDocument()
  })

  it('filters tickets by ID', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'TICKET-100', title: 'Alpha' }),
      createMockTicket({ id: 'TICKET-200', title: 'Beta' }),
      createMockTicket({ id: 'TICKET-300', title: 'Gamma' }),
    ]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderSelector()

    const searchInput = screen.getByPlaceholderText(/search tickets/i)
    await user.type(searchInput, '200')

    expect(screen.queryByText('TICKET-100')).not.toBeInTheDocument()
    expect(screen.getByText('TICKET-200')).toBeInTheDocument()
    expect(screen.queryByText('TICKET-300')).not.toBeInTheDocument()
  })

  it('filters tickets by title (case-insensitive)', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'TICKET-1', title: 'Add Login Feature' }),
      createMockTicket({ id: 'TICKET-2', title: 'Fix Logout Bug' }),
      createMockTicket({ id: 'TICKET-3', title: 'Update Documentation' }),
    ]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderSelector()

    const searchInput = screen.getByPlaceholderText(/search tickets/i)
    await user.type(searchInput, 'logout')

    expect(screen.queryByText(/add login feature/i)).not.toBeInTheDocument()
    expect(screen.getByText(/fix logout bug/i)).toBeInTheDocument()
    expect(screen.queryByText(/update documentation/i)).not.toBeInTheDocument()
  })

  it('shows all tickets when search is cleared', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'TICKET-1', title: 'Alpha' }),
      createMockTicket({ id: 'TICKET-2', title: 'Beta' }),
    ]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderSelector()

    const searchInput = screen.getByPlaceholderText(/search tickets/i)
    await user.type(searchInput, 'Beta')

    expect(screen.queryByText('TICKET-1')).not.toBeInTheDocument()
    expect(screen.getByText('TICKET-2')).toBeInTheDocument()

    await user.clear(searchInput)

    expect(screen.getByText('TICKET-1')).toBeInTheDocument()
    expect(screen.getByText('TICKET-2')).toBeInTheDocument()
  })

  it('shows no tickets message when search has no matches', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'TICKET-1', title: 'Alpha' }),
    ]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderSelector()

    const searchInput = screen.getByPlaceholderText(/search tickets/i)
    await user.type(searchInput, 'nonexistent')

    expect(screen.getByText(/no open tickets found/i)).toBeInTheDocument()
  })
})

describe('ChainTicketSelector - Selection Behavior', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('displays selected count', () => {
    mockUseTicketList.mockReturnValue({
      data: { tickets: [createMockTicket()] },
      isLoading: false,
    })

    renderSelector(['TICKET-1', 'TICKET-2', 'TICKET-3'])

    expect(screen.getByText(/3 tickets selected/i)).toBeInTheDocument()
  })

  it('uses singular form when only one ticket selected', () => {
    mockUseTicketList.mockReturnValue({
      data: { tickets: [createMockTicket()] },
      isLoading: false,
    })

    renderSelector(['TICKET-1'])

    expect(screen.getByText(/1 ticket selected/i)).toBeInTheDocument()
  })

  it('does not show count when no tickets selected', () => {
    mockUseTicketList.mockReturnValue({
      data: { tickets: [createMockTicket()] },
      isLoading: false,
    })

    renderSelector([])

    expect(screen.queryByText(/selected/i)).not.toBeInTheDocument()
  })

  it('renders checkboxes for tickets', () => {
    const tickets = [
      createMockTicket({ id: 'TICKET-1' }),
      createMockTicket({ id: 'TICKET-2' }),
    ]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderSelector()

    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes).toHaveLength(2)
  })

  it('checks checkbox for selected tickets', () => {
    const tickets = [
      createMockTicket({ id: 'TICKET-1' }),
      createMockTicket({ id: 'TICKET-2' }),
    ]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderSelector(['TICKET-1'])

    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes[0]).toBeChecked()
    expect(checkboxes[1]).not.toBeChecked()
  })

  it('calls onChange with added ticket ID when checkbox is checked', async () => {
    const user = userEvent.setup()
    const tickets = [createMockTicket({ id: 'TICKET-1' })]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    const { onChange } = renderSelector([])

    const checkbox = screen.getByRole('checkbox')
    await user.click(checkbox)

    expect(onChange).toHaveBeenCalledWith(['TICKET-1'])
  })

  it('calls onChange with removed ticket ID when checkbox is unchecked', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'TICKET-1' }),
      createMockTicket({ id: 'TICKET-2' }),
    ]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    const { onChange } = renderSelector(['TICKET-1', 'TICKET-2'])

    const checkboxes = screen.getAllByRole('checkbox')
    await user.click(checkboxes[0]) // Uncheck TICKET-1

    expect(onChange).toHaveBeenCalledWith(['TICKET-2'])
  })

  it('toggles multiple tickets correctly', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'TICKET-1' }),
      createMockTicket({ id: 'TICKET-2' }),
      createMockTicket({ id: 'TICKET-3' }),
    ]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    const { onChange } = renderSelector([])

    const checkboxes = screen.getAllByRole('checkbox')

    // Select first ticket
    await user.click(checkboxes[0])
    expect(onChange).toHaveBeenCalledWith(['TICKET-1'])

    // Select third ticket (with first already selected)
    cleanup()
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })
    const { onChange: onChange2 } = renderSelector(['TICKET-1'])
    const checkboxes2 = screen.getAllByRole('checkbox')
    await user.click(checkboxes2[2])
    expect(onChange2).toHaveBeenCalledWith(['TICKET-1', 'TICKET-3'])
  })
})

describe('ChainTicketSelector - Ticket Display', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('displays ticket ID in monospace font', () => {
    const tickets = [createMockTicket({ id: 'TICKET-ABC' })]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderSelector()

    const ticketId = screen.getByText('TICKET-ABC')
    expect(ticketId).toHaveClass('font-mono')
  })

  it('displays ticket title', () => {
    const tickets = [createMockTicket({ title: 'Implement New Feature' })]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderSelector()

    expect(screen.getByText('Implement New Feature')).toBeInTheDocument()
  })

  it('displays ticket status badge', () => {
    const tickets = [createMockTicket({ status: 'open' })]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderSelector()

    expect(screen.getByText('open')).toBeInTheDocument()
  })

  it('truncates long ticket titles', () => {
    const tickets = [createMockTicket({ title: 'Very long ticket title that should be truncated' })]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderSelector()

    const titleElement = screen.getByText(/very long ticket title/i)
    expect(titleElement).toHaveClass('truncate')
  })
})

describe('ChainTicketSelector - Visual Feedback', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('highlights selected ticket rows', () => {
    const tickets = [
      createMockTicket({ id: 'TICKET-1' }),
      createMockTicket({ id: 'TICKET-2' }),
    ]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    const { container } = renderSelector(['TICKET-1'])

    // Find the label containing TICKET-1
    const labels = container.querySelectorAll('label')
    const selectedLabel = Array.from(labels).find((label) =>
      label.textContent?.includes('TICKET-1')
    )

    expect(selectedLabel).toHaveClass('bg-muted')
  })

  it('applies hover styles to ticket rows', () => {
    const tickets = [createMockTicket()]
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    const { container } = renderSelector()

    const label = container.querySelector('label')
    expect(label).toHaveClass('hover:bg-muted/50')
  })
})

describe('ChainTicketSelector - Scrollable Container', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders list in scrollable container with max height', () => {
    const tickets = Array.from({ length: 20 }, (_, i) =>
      createMockTicket({ id: `TICKET-${i + 1}`, title: `Ticket ${i + 1}` })
    )
    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    const { container } = renderSelector()

    const scrollContainer = container.querySelector('.max-h-60.overflow-y-auto')
    expect(scrollContainer).toBeInTheDocument()
  })
})
