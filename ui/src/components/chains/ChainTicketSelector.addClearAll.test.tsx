import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { ChainTicketSelector } from './ChainTicketSelector'
import type { PendingTicket } from '@/types/ticket'

const mockUseTicketList = vi.fn()
vi.mock('@/hooks/useTickets', () => ({
  useTicketList: (params?: unknown) => mockUseTicketList(params),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

function createMockTicket(overrides: Partial<PendingTicket> = {}): PendingTicket {
  return {
    id: 'TICKET-1',
    title: 'Test Ticket',
    description: '',
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

function renderSelector(
  selectedIds: string[] = [],
  onChange = vi.fn(),
  extras: { onEpicIdsChange?: ReturnType<typeof vi.fn>; excludeIds?: Set<string> } = {}
) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return {
    onChange,
    ...render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <ChainTicketSelector
            selectedIds={selectedIds}
            onChange={onChange}
            onEpicIdsChange={extras.onEpicIdsChange}
            excludeIds={extras.excludeIds}
          />
        </MemoryRouter>
      </QueryClientProvider>
    ),
  }
}

describe('ChainTicketSelector - Add All button', () => {
  beforeEach(() => vi.clearAllMocks())

  it('is visible when filtered list is non-empty', () => {
    mockUseTicketList.mockReturnValue({
      data: { tickets: [createMockTicket({ id: 'T-1', title: 'Task One' })] },
      isLoading: false,
    })
    renderSelector()
    expect(screen.getByRole('button', { name: /add all/i })).toBeInTheDocument()
  })

  it('is not rendered when filtered list is empty', () => {
    mockUseTicketList.mockReturnValue({ data: { tickets: [] }, isLoading: false })
    renderSelector()
    expect(screen.queryByRole('button', { name: /add all/i })).not.toBeInTheDocument()
  })

  it('selects all visible tickets when clicked', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'T-1', title: 'Alpha' }),
      createMockTicket({ id: 'T-2', title: 'Beta' }),
      createMockTicket({ id: 'T-3', title: 'Gamma' }),
    ]
    mockUseTicketList.mockReturnValue({ data: { tickets }, isLoading: false })
    const { onChange } = renderSelector([])

    await user.click(screen.getByRole('button', { name: /add all/i }))

    expect(onChange).toHaveBeenCalledWith(['T-1', 'T-2', 'T-3'])
  })

  it('merges with existing selection, no duplicates', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'T-1', title: 'Alpha' }),
      createMockTicket({ id: 'T-2', title: 'Beta' }),
    ]
    mockUseTicketList.mockReturnValue({ data: { tickets }, isLoading: false })
    const { onChange } = renderSelector(['T-1'])

    await user.click(screen.getByRole('button', { name: /add all/i }))

    expect(onChange).toHaveBeenCalledWith(['T-1', 'T-2'])
  })

  it('only selects filtered results when search is active', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'T-1', title: 'Auth login' }),
      createMockTicket({ id: 'T-2', title: 'Billing fix' }),
      createMockTicket({ id: 'T-3', title: 'Auth logout' }),
    ]
    mockUseTicketList.mockReturnValue({ data: { tickets }, isLoading: false })
    const { onChange } = renderSelector([])

    await user.type(screen.getByPlaceholderText(/search tickets/i), 'auth')
    await user.click(screen.getByRole('button', { name: /add all/i }))

    expect(onChange).toHaveBeenCalledWith(['T-1', 'T-3'])
  })

  it('respects excludeIds — excluded tickets are not added', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'T-1', title: 'Included' }),
      createMockTicket({ id: 'T-2', title: 'Excluded' }),
      createMockTicket({ id: 'T-3', title: 'Also Included' }),
    ]
    mockUseTicketList.mockReturnValue({ data: { tickets }, isLoading: false })
    const { onChange } = renderSelector([], vi.fn(), { excludeIds: new Set(['T-2']) })

    await user.click(screen.getByRole('button', { name: /add all/i }))

    expect(onChange).toHaveBeenCalledWith(['T-1', 'T-3'])
  })
})

describe('ChainTicketSelector - Add All with Epics', () => {
  beforeEach(() => vi.clearAllMocks())

  it('selects epic + its children and calls onEpicIdsChange', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'EPIC-1', issue_type: 'epic', title: 'Big Epic' }),
      createMockTicket({ id: 'CHILD-A', parent_ticket_id: 'EPIC-1', title: 'Task A' }),
      createMockTicket({ id: 'CHILD-B', parent_ticket_id: 'EPIC-1', title: 'Task B' }),
      createMockTicket({ id: 'T-1', title: 'Regular ticket' }),
    ]
    mockUseTicketList.mockReturnValue({ data: { tickets }, isLoading: false })
    const onEpicIdsChange = vi.fn()
    const { onChange } = renderSelector([], vi.fn(), { onEpicIdsChange })

    await user.click(screen.getByRole('button', { name: /add all/i }))

    // Epic + its children + regular ticket should all be in selection
    const calledWith: string[] = onChange.mock.calls[0][0]
    expect(calledWith).toContain('EPIC-1')
    expect(calledWith).toContain('CHILD-A')
    expect(calledWith).toContain('CHILD-B')
    expect(calledWith).toContain('T-1')
    expect(onEpicIdsChange).toHaveBeenCalledWith(['EPIC-1'])
  })

  it('does not add epic children already in selection twice', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'EPIC-1', issue_type: 'epic', title: 'Epic' }),
      createMockTicket({ id: 'CHILD-A', parent_ticket_id: 'EPIC-1', title: 'Task A' }),
    ]
    mockUseTicketList.mockReturnValue({ data: { tickets }, isLoading: false })
    const { onChange } = renderSelector(['CHILD-A'])

    await user.click(screen.getByRole('button', { name: /add all/i }))

    const calledWith: string[] = onChange.mock.calls[0][0]
    const childACount = calledWith.filter((id) => id === 'CHILD-A').length
    expect(childACount).toBe(1)
  })
})

describe('ChainTicketSelector - Clear All button', () => {
  beforeEach(() => vi.clearAllMocks())

  it('is not visible when nothing is selected', () => {
    mockUseTicketList.mockReturnValue({
      data: { tickets: [createMockTicket({ id: 'T-1' })] },
      isLoading: false,
    })
    renderSelector([])
    expect(screen.queryByRole('button', { name: /clear all/i })).not.toBeInTheDocument()
  })

  it('is visible when at least one ticket is selected', () => {
    mockUseTicketList.mockReturnValue({
      data: { tickets: [createMockTicket({ id: 'T-1' })] },
      isLoading: false,
    })
    renderSelector(['T-1'])
    expect(screen.getByRole('button', { name: /clear all/i })).toBeInTheDocument()
  })

  it('calls onChange with empty array on click', async () => {
    const user = userEvent.setup()
    const tickets = [
      createMockTicket({ id: 'T-1', title: 'Alpha' }),
      createMockTicket({ id: 'T-2', title: 'Beta' }),
    ]
    mockUseTicketList.mockReturnValue({ data: { tickets }, isLoading: false })
    const { onChange } = renderSelector(['T-1', 'T-2'])

    await user.click(screen.getByRole('button', { name: /clear all/i }))

    expect(onChange).toHaveBeenCalledWith([])
  })

  it('calls onEpicIdsChange with empty array on click', async () => {
    const user = userEvent.setup()
    mockUseTicketList.mockReturnValue({
      data: {
        tickets: [
          createMockTicket({ id: 'EPIC-1', issue_type: 'epic' }),
          createMockTicket({ id: 'CHILD-1', parent_ticket_id: 'EPIC-1' }),
        ],
      },
      isLoading: false,
    })
    const onEpicIdsChange = vi.fn()
    renderSelector(['EPIC-1', 'CHILD-1'], vi.fn(), { onEpicIdsChange })

    await user.click(screen.getByRole('button', { name: /clear all/i }))

    expect(onEpicIdsChange).toHaveBeenCalledWith([])
  })

  it('resets search input on click', async () => {
    const user = userEvent.setup()
    mockUseTicketList.mockReturnValue({
      data: { tickets: [createMockTicket({ id: 'T-1', title: 'Feature' })] },
      isLoading: false,
    })
    // Render with selection so Clear All is visible
    renderSelector(['T-1'])

    const searchInput = screen.getByPlaceholderText(/search tickets/i)
    await user.type(searchInput, 'auth')
    expect(searchInput).toHaveValue('auth')

    await user.click(screen.getByRole('button', { name: /clear all/i }))

    expect(searchInput).toHaveValue('')
  })
})
