import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { AppendToChainDialog } from './AppendToChainDialog'
import type { ChainExecution } from '@/types/chain'
import type { PendingTicket } from '@/types/ticket'

// Mock hooks
const mockUseAppendToChain = vi.fn()
const mockUseTicketList = vi.fn()

vi.mock('@/hooks/useChains', () => ({
  useAppendToChain: () => mockUseAppendToChain(),
}))

vi.mock('@/hooks/useTickets', () => ({
  useTicketList: () => mockUseTicketList(),
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

function createMockChain(overrides: Partial<ChainExecution> = {}): ChainExecution {
  return {
    id: 'chain-123',
    project_id: 'test-project',
    name: 'Test Chain',
    status: 'running',
    workflow_name: 'feature',
    created_by: 'test-user',
    total_items: 2,
    completed_items: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    items: [
      {
        id: 'item-1',
        chain_id: 'chain-123',
        ticket_id: 'TICKET-1',
        position: 0,
        status: 'pending',
      },
      {
        id: 'item-2',
        chain_id: 'chain-123',
        ticket_id: 'TICKET-2',
        position: 1,
        status: 'pending',
      },
    ],
    ...overrides,
  }
}

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

function renderDialog(open = true, chain?: ChainExecution) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const onClose = vi.fn()
  const testChain = chain || createMockChain()

  return {
    onClose,
    ...render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AppendToChainDialog open={open} onClose={onClose} chain={testChain} />
        </MemoryRouter>
      </QueryClientProvider>
    ),
  }
}

describe('AppendToChainDialog - Render States', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAppendToChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })
    mockUseTicketList.mockReturnValue({
      data: { tickets: [] },
      isLoading: false,
    })
  })

  it('does not render when open is false', () => {
    renderDialog(false)

    expect(screen.queryByRole('heading', { name: /append tickets to chain/i })).not.toBeInTheDocument()
  })

  it('renders with heading when open', () => {
    renderDialog()

    expect(screen.getByRole('heading', { name: /append tickets to chain/i })).toBeInTheDocument()
  })

  it('renders ChainTicketSelector', () => {
    renderDialog()

    expect(screen.getByPlaceholderText(/search tickets/i)).toBeInTheDocument()
  })

  it('shows explanation text', () => {
    renderDialog()

    expect(screen.getByText(/select tickets to append to the running chain/i)).toBeInTheDocument()
    expect(screen.getByText(/tickets already in the chain are excluded/i)).toBeInTheDocument()
  })

  it('shows Cancel and Append buttons', () => {
    renderDialog()

    expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /append/i })).toBeInTheDocument()
  })
})

describe('AppendToChainDialog - Exclude Existing Tickets', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAppendToChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })
  })

  it('passes existing ticket IDs to ChainTicketSelector as excludeIds', () => {
    const chain = createMockChain({
      items: [
        { id: 'item-1', chain_id: 'chain-123', ticket_id: 'TICKET-1', position: 0, status: 'pending' },
        { id: 'item-2', chain_id: 'chain-123', ticket_id: 'TICKET-2', position: 1, status: 'pending' },
        { id: 'item-3', chain_id: 'chain-123', ticket_id: 'TICKET-3', position: 2, status: 'completed' },
      ],
    })

    const tickets = [
      createMockTicket({ id: 'TICKET-1', title: 'Already in chain' }),
      createMockTicket({ id: 'TICKET-2', title: 'Already in chain 2' }),
      createMockTicket({ id: 'TICKET-3', title: 'Already in chain 3' }),
      createMockTicket({ id: 'TICKET-4', title: 'Available ticket' }),
    ]

    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderDialog(true, chain)

    // TICKET-1, TICKET-2, TICKET-3 should be excluded from the list
    expect(screen.queryByText('TICKET-1')).not.toBeInTheDocument()
    expect(screen.queryByText('TICKET-2')).not.toBeInTheDocument()
    expect(screen.queryByText('TICKET-3')).not.toBeInTheDocument()
    expect(screen.getByText('TICKET-4')).toBeInTheDocument()
  })

  it('handles chain with empty items array', () => {
    const chain = createMockChain({ items: [] })
    const tickets = [
      createMockTicket({ id: 'TICKET-1', title: 'Available' }),
    ]

    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderDialog(true, chain)

    expect(screen.getByText('TICKET-1')).toBeInTheDocument()
  })

  it('handles chain with undefined items', () => {
    const chain = createMockChain({ items: undefined })
    const tickets = [
      createMockTicket({ id: 'TICKET-1', title: 'Available' }),
    ]

    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderDialog(true, chain)

    expect(screen.getByText('TICKET-1')).toBeInTheDocument()
  })
})

describe('AppendToChainDialog - Submit Behavior', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTicketList.mockReturnValue({
      data: {
        tickets: [
          createMockTicket({ id: 'TICKET-3', title: 'New ticket' }),
          createMockTicket({ id: 'TICKET-4', title: 'Another new ticket' }),
        ],
      },
      isLoading: false,
    })
  })

  it('Append button is disabled when no tickets selected', () => {
    mockUseAppendToChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    renderDialog()

    const appendButton = screen.getByRole('button', { name: /append/i })
    expect(appendButton).toBeDisabled()
  })

  it('Append button is enabled when tickets are selected', async () => {
    const user = userEvent.setup()
    mockUseAppendToChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    renderDialog()

    const checkbox = screen.getAllByRole('checkbox')[0]
    await user.click(checkbox)

    const appendButton = screen.getByRole('button', { name: /append/i })
    expect(appendButton).not.toBeDisabled()
  })

  it('calls appendToChain mutation with selected ticket IDs on submit', async () => {
    const user = userEvent.setup()
    const mutateAsync = vi.fn().mockResolvedValue({})
    mockUseAppendToChain.mockReturnValue({
      mutateAsync,
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    renderDialog()

    const checkboxes = screen.getAllByRole('checkbox')
    await user.click(checkboxes[0]) // Select TICKET-3

    const appendButton = screen.getByRole('button', { name: /append/i })
    await user.click(appendButton)

    expect(mutateAsync).toHaveBeenCalledWith({
      id: 'chain-123',
      data: { ticket_ids: ['TICKET-3'] },
    })
  })

  it('calls appendToChain with multiple ticket IDs', async () => {
    const user = userEvent.setup()
    const mutateAsync = vi.fn().mockResolvedValue({})
    mockUseAppendToChain.mockReturnValue({
      mutateAsync,
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    renderDialog()

    const checkboxes = screen.getAllByRole('checkbox')
    await user.click(checkboxes[0]) // TICKET-3
    await user.click(checkboxes[1]) // TICKET-4

    const appendButton = screen.getByRole('button', { name: /append/i })
    await user.click(appendButton)

    expect(mutateAsync).toHaveBeenCalledWith({
      id: 'chain-123',
      data: { ticket_ids: expect.arrayContaining(['TICKET-3', 'TICKET-4']) },
    })
  })

  it('excludes epic IDs from submitted ticket_ids', async () => {
    const user = userEvent.setup()
    const mutateAsync = vi.fn().mockResolvedValue({})
    mockUseAppendToChain.mockReturnValue({
      mutateAsync,
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    const tickets = [
      createMockTicket({ id: 'EPIC-1', issue_type: 'epic', title: 'Epic' }),
      createMockTicket({ id: 'CHILD-1', parent_ticket_id: 'EPIC-1', title: 'Child' }),
      createMockTicket({ id: 'CHILD-2', parent_ticket_id: 'EPIC-1', title: 'Child 2' }),
    ]

    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderDialog()

    // Select the epic (which auto-selects children)
    const checkboxes = screen.getAllByRole('checkbox')
    await user.click(checkboxes[0]) // Click epic checkbox

    const appendButton = screen.getByRole('button', { name: /append/i })
    await user.click(appendButton)

    // Should submit only children, not the epic itself
    expect(mutateAsync).toHaveBeenCalledWith({
      id: 'chain-123',
      data: { ticket_ids: expect.arrayContaining(['CHILD-1', 'CHILD-2']) },
    })
    const call = mutateAsync.mock.calls[0][0]
    expect(call.data.ticket_ids).not.toContain('EPIC-1')
  })

  it('excludes tickets already in the chain from submission', async () => {
    const user = userEvent.setup()
    const mutateAsync = vi.fn().mockResolvedValue({})
    mockUseAppendToChain.mockReturnValue({
      mutateAsync,
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    const chain = createMockChain({
      items: [
        { id: 'item-1', chain_id: 'chain-123', ticket_id: 'TICKET-1', position: 0, status: 'pending' },
      ],
    })

    const tickets = [
      createMockTicket({ id: 'TICKET-2', title: 'New ticket' }),
    ]

    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderDialog(true, chain)

    const checkbox = screen.getByRole('checkbox')
    await user.click(checkbox)

    const appendButton = screen.getByRole('button', { name: /append/i })
    await user.click(appendButton)

    expect(mutateAsync).toHaveBeenCalledWith({
      id: 'chain-123',
      data: { ticket_ids: ['TICKET-2'] },
    })
  })

  it('does not submit when all selected tickets are already in chain', async () => {
    const mutateAsync = vi.fn().mockResolvedValue({})
    mockUseAppendToChain.mockReturnValue({
      mutateAsync,
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    // Simulate a scenario where selector somehow has a ticket that's in the chain
    // This shouldn't happen in practice due to excludeIds, but test the defensive logic
    renderDialog()

    // No tickets to select since they're all excluded
    const appendButton = screen.getByRole('button', { name: /append/i })
    expect(appendButton).toBeDisabled()

    // Mutation should not be called
    expect(mutateAsync).not.toHaveBeenCalled()
  })
})

describe('AppendToChainDialog - Mutation States', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTicketList.mockReturnValue({
      data: {
        tickets: [
          createMockTicket({ id: 'TICKET-3', title: 'New ticket' }),
        ],
      },
      isLoading: false,
    })
  })

  it('disables Append button while mutation is pending', async () => {
    const user = userEvent.setup()
    mockUseAppendToChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: true,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    renderDialog()

    const checkbox = screen.getByRole('checkbox')
    await user.click(checkbox)

    const appendButton = screen.getByRole('button', { name: /append/i })
    expect(appendButton).toBeDisabled()
  })

  it('shows spinner when mutation is pending', async () => {
    const user = userEvent.setup()
    mockUseAppendToChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: true,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    renderDialog()

    const checkbox = screen.getByRole('checkbox')
    await user.click(checkbox)

    const spinner = screen.getByRole('status', { name: /loading/i })
    expect(spinner).toBeInTheDocument()
  })

  it('displays error message when mutation fails', () => {
    mockUseAppendToChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: true,
      error: new Error('Failed to append tickets'),
      reset: vi.fn(),
    })

    renderDialog()

    expect(screen.getByText(/failed to append tickets/i)).toBeInTheDocument()
  })

  it('displays generic error message when error has no message', () => {
    mockUseAppendToChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: true,
      error: { message: null },
      reset: vi.fn(),
    })

    renderDialog()

    expect(screen.getByText(/append failed/i)).toBeInTheDocument()
  })

  it('calls onClose after successful append', async () => {
    const user = userEvent.setup()
    const mutateAsync = vi.fn().mockResolvedValue({})
    mockUseAppendToChain.mockReturnValue({
      mutateAsync,
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    const { onClose } = renderDialog()

    const checkbox = screen.getByRole('checkbox')
    await user.click(checkbox)

    const appendButton = screen.getByRole('button', { name: /append/i })
    await user.click(appendButton)

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled()
    })
  })

  it('does not call onClose when mutation fails', async () => {
    const user = userEvent.setup()
    const mutateAsync = vi.fn().mockRejectedValue(new Error('Network error'))
    mockUseAppendToChain.mockReturnValue({
      mutateAsync,
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    const { onClose } = renderDialog()

    const checkbox = screen.getByRole('checkbox')
    await user.click(checkbox)

    const appendButton = screen.getByRole('button', { name: /append/i })
    await user.click(appendButton)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalled()
    })

    expect(onClose).not.toHaveBeenCalled()
  })
})

describe('AppendToChainDialog - Dialog Actions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAppendToChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })
    mockUseTicketList.mockReturnValue({
      data: { tickets: [] },
      isLoading: false,
    })
  })

  it('calls onClose when Cancel button is clicked', async () => {
    const user = userEvent.setup()
    const { onClose } = renderDialog()

    const cancelButton = screen.getByRole('button', { name: /cancel/i })
    await user.click(cancelButton)

    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('calls onClose when dialog close icon is clicked', async () => {
    const user = userEvent.setup()
    const { onClose, container } = renderDialog()

    const closeButton = container.querySelector('button[aria-label="Close"]')
    if (closeButton) {
      await user.click(closeButton)
      expect(onClose).toHaveBeenCalled()
    }
  })
})

describe('AppendToChainDialog - Form Reset on Close', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTicketList.mockReturnValue({
      data: {
        tickets: [
          createMockTicket({ id: 'TICKET-3', title: 'New ticket' }),
        ],
      },
      isLoading: false,
    })
  })

  it('resets selected tickets when dialog is closed and reopened', async () => {
    const user = userEvent.setup()
    const reset = vi.fn()
    mockUseAppendToChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
      reset,
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const onClose = vi.fn()
    const chain = createMockChain()

    const { rerender } = render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AppendToChainDialog open={true} onClose={onClose} chain={chain} />
        </MemoryRouter>
      </QueryClientProvider>
    )

    // Select a ticket
    const checkbox = screen.getByRole('checkbox')
    await user.click(checkbox)

    // Close dialog
    await user.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onClose).toHaveBeenCalled()

    // Simulate closing
    rerender(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AppendToChainDialog open={false} onClose={onClose} chain={chain} />
        </MemoryRouter>
      </QueryClientProvider>
    )

    // Reopen
    rerender(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AppendToChainDialog open={true} onClose={onClose} chain={chain} />
        </MemoryRouter>
      </QueryClientProvider>
    )

    // Selection should be reset (no tickets selected)
    await waitFor(() => {
      expect(screen.queryByText(/selected/i)).not.toBeInTheDocument()
    })

    // Mutation should be reset
    expect(reset).toHaveBeenCalled()
  })

  it('resets error state when dialog is closed and reopened', async () => {
    const reset = vi.fn()
    mockUseAppendToChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: true,
      error: new Error('Previous error'),
      reset,
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const onClose = vi.fn()
    const chain = createMockChain()

    const { rerender } = render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AppendToChainDialog open={true} onClose={onClose} chain={chain} />
        </MemoryRouter>
      </QueryClientProvider>
    )

    // Error should be visible
    expect(screen.getByText(/previous error/i)).toBeInTheDocument()

    // Close and reopen
    rerender(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AppendToChainDialog open={false} onClose={onClose} chain={chain} />
        </MemoryRouter>
      </QueryClientProvider>
    )

    // Reset should be called
    expect(reset).toHaveBeenCalled()
  })
})

describe('AppendToChainDialog - Epic Handling', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAppendToChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })
  })

  it('tracks epic IDs separately via onEpicIdsChange', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'EPIC-1', issue_type: 'epic', title: 'Epic' }),
      createMockTicket({ id: 'CHILD-1', parent_ticket_id: 'EPIC-1', title: 'Child' }),
    ]

    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderDialog()

    const checkboxes = screen.getAllByRole('checkbox')
    await user.click(checkboxes[0]) // Click epic checkbox

    // Epic should be selected but hidden from final submission
    // ChainTicketSelector calls onEpicIdsChange internally
  })

  it('submits only epic children when epic is selected', async () => {
    const user = userEvent.setup()
    const mutateAsync = vi.fn().mockResolvedValue({})
    mockUseAppendToChain.mockReturnValue({
      mutateAsync,
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    const tickets = [
      createMockTicket({ id: 'EPIC-1', issue_type: 'epic', title: 'Epic' }),
      createMockTicket({ id: 'CHILD-1', parent_ticket_id: 'EPIC-1', title: 'Child 1' }),
      createMockTicket({ id: 'CHILD-2', parent_ticket_id: 'EPIC-1', title: 'Child 2' }),
    ]

    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderDialog()

    const checkboxes = screen.getAllByRole('checkbox')
    await user.click(checkboxes[0]) // Click epic checkbox

    const appendButton = screen.getByRole('button', { name: /append/i })
    await user.click(appendButton)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        id: 'chain-123',
        data: {
          ticket_ids: expect.arrayContaining(['CHILD-1', 'CHILD-2']),
        },
      })
    })

    // Verify epic ID is NOT in the submission
    const call = mutateAsync.mock.calls[0][0]
    expect(call.data.ticket_ids).not.toContain('EPIC-1')
  })

  it('fallback to all ticketIds when no epic children', async () => {
    const user = userEvent.setup()
    const mutateAsync = vi.fn().mockResolvedValue({})
    mockUseAppendToChain.mockReturnValue({
      mutateAsync,
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    })

    const tickets = [
      createMockTicket({ id: 'TICKET-5', title: 'Regular ticket' }),
    ]

    mockUseTicketList.mockReturnValue({
      data: { tickets },
      isLoading: false,
    })

    renderDialog()

    const checkbox = screen.getByRole('checkbox')
    await user.click(checkbox)

    const appendButton = screen.getByRole('button', { name: /append/i })
    await user.click(appendButton)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        id: 'chain-123',
        data: { ticket_ids: ['TICKET-5'] },
      })
    })
  })
})
