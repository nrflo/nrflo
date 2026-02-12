import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TicketSearchDropdown } from './TicketSearchDropdown'
import type { PendingTicket } from '@/types/ticket'

const mockTickets: PendingTicket[] = [
  {
    id: 'TICK-1',
    title: 'Fix login bug',
    status: 'open',
    priority: 2,
    issue_type: 'bug',
    description: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    closed_at: null,
    created_by: 'user',
    close_reason: null,
    is_blocked: false,
  },
  {
    id: 'TICK-2',
    title: 'Add dashboard',
    status: 'in_progress',
    priority: 1,
    issue_type: 'feature',
    description: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    closed_at: null,
    created_by: 'user',
    close_reason: null,
    is_blocked: false,
  },
  {
    id: 'TICK-3',
    title: 'Old task',
    status: 'closed',
    priority: 3,
    issue_type: 'task',
    description: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    closed_at: '2026-01-02T00:00:00Z',
    created_by: 'user',
    close_reason: 'done',
    is_blocked: false,
  },
]

// Mock the useTicketSearch hook
const mockUseTicketSearch = vi.fn()
vi.mock('@/hooks/useTickets', () => ({
  useTicketSearch: (...args: unknown[]) => mockUseTicketSearch(...args),
}))

function renderDropdown(props: Partial<React.ComponentProps<typeof TicketSearchDropdown>> = {}) {
  const onSelect = props.onSelect ?? vi.fn()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <TicketSearchDropdown onSelect={onSelect} {...props} />
    </QueryClientProvider>
  )
  return { onSelect }
}

describe('TicketSearchDropdown', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTicketSearch.mockReturnValue({ data: undefined, isFetching: false })
  })

  it('renders search input with default placeholder', () => {
    renderDropdown()
    expect(screen.getByPlaceholderText('Search tickets...')).toBeInTheDocument()
  })

  it('renders search input with custom placeholder', () => {
    renderDropdown({ placeholder: 'Find a ticket...' })
    expect(screen.getByPlaceholderText('Find a ticket...')).toBeInTheDocument()
  })

  it('does not show dropdown when query is less than 2 characters', async () => {
    const user = userEvent.setup()
    renderDropdown()

    await user.type(screen.getByPlaceholderText('Search tickets...'), 'a')

    expect(screen.queryByText('No tickets found')).not.toBeInTheDocument()
    expect(screen.queryByText('Fix login bug')).not.toBeInTheDocument()
  })

  it('shows dropdown with results after typing 2+ characters', async () => {
    const user = userEvent.setup()
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: mockTickets },
      isFetching: false,
    })
    renderDropdown()

    await user.type(screen.getByPlaceholderText('Search tickets...'), 'fi')

    expect(screen.getByText('Fix login bug')).toBeInTheDocument()
    expect(screen.getByText('TICK-1')).toBeInTheDocument()
    expect(screen.getByText('open')).toBeInTheDocument()
    expect(screen.getByText('Add dashboard')).toBeInTheDocument()
    expect(screen.getByText('TICK-2')).toBeInTheDocument()
  })

  it('filters out excludeIds from results', async () => {
    const user = userEvent.setup()
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: mockTickets },
      isFetching: false,
    })
    renderDropdown({ excludeIds: ['TICK-1', 'TICK-3'] })

    await user.type(screen.getByPlaceholderText('Search tickets...'), 'ti')

    expect(screen.queryByText('TICK-1')).not.toBeInTheDocument()
    expect(screen.queryByText('TICK-3')).not.toBeInTheDocument()
    expect(screen.getByText('TICK-2')).toBeInTheDocument()
    expect(screen.getByText('Add dashboard')).toBeInTheDocument()
  })

  it('calls onSelect with ticket and clears input on click', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: mockTickets },
      isFetching: false,
    })
    renderDropdown({ onSelect })

    const input = screen.getByPlaceholderText('Search tickets...')
    await user.type(input, 'fix')
    await user.click(screen.getByText('Fix login bug'))

    expect(onSelect).toHaveBeenCalledTimes(1)
    expect(onSelect).toHaveBeenCalledWith(mockTickets[0])
    expect(input).toHaveValue('')
  })

  it('closes dropdown after selection', async () => {
    const user = userEvent.setup()
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: mockTickets },
      isFetching: false,
    })
    renderDropdown()

    const input = screen.getByPlaceholderText('Search tickets...')
    await user.type(input, 'fix')
    expect(screen.getByText('Fix login bug')).toBeInTheDocument()

    await user.click(screen.getByText('Fix login bug'))
    expect(screen.queryByText('Fix login bug')).not.toBeInTheDocument()
  })

  it('closes dropdown on Escape key', async () => {
    const user = userEvent.setup()
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: mockTickets },
      isFetching: false,
    })
    renderDropdown()

    await user.type(screen.getByPlaceholderText('Search tickets...'), 'fix')
    expect(screen.getByText('Fix login bug')).toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(screen.queryByText('Fix login bug')).not.toBeInTheDocument()
  })

  it('closes dropdown on click outside', async () => {
    const user = userEvent.setup()
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: mockTickets },
      isFetching: false,
    })
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <div>
          <button>outside</button>
          <TicketSearchDropdown onSelect={vi.fn()} />
        </div>
      </QueryClientProvider>
    )

    await user.type(screen.getByPlaceholderText('Search tickets...'), 'fix')
    expect(screen.getByText('Fix login bug')).toBeInTheDocument()

    await user.click(screen.getByText('outside'))
    expect(screen.queryByText('Fix login bug')).not.toBeInTheDocument()
  })

  it('shows "No tickets found" when results are empty and not fetching', async () => {
    const user = userEvent.setup()
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: [] },
      isFetching: false,
    })
    renderDropdown()

    await user.type(screen.getByPlaceholderText('Search tickets...'), 'xyz')

    expect(screen.getByText('No tickets found')).toBeInTheDocument()
  })

  it('does not show "No tickets found" while fetching', async () => {
    const user = userEvent.setup()
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: [] },
      isFetching: true,
    })
    renderDropdown()

    await user.type(screen.getByPlaceholderText('Search tickets...'), 'xyz')

    expect(screen.queryByText('No tickets found')).not.toBeInTheDocument()
  })

  it('shows all three status badge styles', async () => {
    const user = userEvent.setup()
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: mockTickets },
      isFetching: false,
    })
    renderDropdown()

    await user.type(screen.getByPlaceholderText('Search tickets...'), 'ti')

    expect(screen.getByText('open')).toBeInTheDocument()
    expect(screen.getByText('in_progress')).toBeInTheDocument()
    expect(screen.getByText('closed')).toBeInTheDocument()
  })

  it('reopens dropdown on focus when query >= 2 chars', async () => {
    const user = userEvent.setup()
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: mockTickets },
      isFetching: false,
    })

    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <div>
          <button>outside</button>
          <TicketSearchDropdown onSelect={vi.fn()} />
        </div>
      </QueryClientProvider>
    )

    const input = screen.getByPlaceholderText('Search tickets...')
    await user.type(input, 'fix')
    expect(screen.getByText('Fix login bug')).toBeInTheDocument()

    // Close by pressing Escape
    await user.keyboard('{Escape}')
    expect(screen.queryByText('Fix login bug')).not.toBeInTheDocument()

    // Trigger focus on input to reopen
    fireEvent.focus(input)
    expect(screen.getByText('Fix login bug')).toBeInTheDocument()
  })

  it('limits displayed results to 10', async () => {
    const user = userEvent.setup()
    const manyTickets: PendingTicket[] = Array.from({ length: 15 }, (_, i) => ({
      id: `TICK-${i + 1}`,
      title: `Ticket ${i + 1}`,
      status: 'open' as const,
      priority: 2,
      issue_type: 'task' as const,
      description: null,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      closed_at: null,
      created_by: 'user',
      close_reason: null,
      is_blocked: false,
    }))
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: manyTickets },
      isFetching: false,
    })
    renderDropdown()

    await user.type(screen.getByPlaceholderText('Search tickets...'), 'ti')

    // First 10 should be visible, 11-15 should not
    for (let i = 1; i <= 10; i++) {
      expect(screen.getByText(`Ticket ${i}`)).toBeInTheDocument()
    }
    for (let i = 11; i <= 15; i++) {
      expect(screen.queryByText(`Ticket ${i}`)).not.toBeInTheDocument()
    }
  })

  it('shows "No tickets found" when all results are excluded', async () => {
    const user = userEvent.setup()
    mockUseTicketSearch.mockReturnValue({
      data: { tickets: mockTickets },
      isFetching: false,
    })
    renderDropdown({ excludeIds: ['TICK-1', 'TICK-2', 'TICK-3'] })

    await user.type(screen.getByPlaceholderText('Search tickets...'), 'ti')

    expect(screen.getByText('No tickets found')).toBeInTheDocument()
  })

  it('passes query to useTicketSearch', async () => {
    const user = userEvent.setup()
    renderDropdown()

    await user.type(screen.getByPlaceholderText('Search tickets...'), 'fix bug')

    expect(mockUseTicketSearch).toHaveBeenCalledWith('fix bug')
  })
})
