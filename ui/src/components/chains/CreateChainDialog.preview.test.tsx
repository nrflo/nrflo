import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, fireEvent, act } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { CreateChainDialog } from './CreateChainDialog'
import { previewChain } from '@/api/chains'
import type { ChainExecution } from '@/types/chain'
import type { WorkflowDef } from '@/types/workflow'

vi.mock('@/api/chains', () => ({
  previewChain: vi.fn(),
}))

const mockUseCreateChain = vi.fn()
const mockUseUpdateChain = vi.fn()
const mockUseQuery = vi.fn()

vi.mock('@/hooks/useChains', () => ({
  useCreateChain: () => mockUseCreateChain(),
  useUpdateChain: () => mockUseUpdateChain(),
}))

vi.mock('@/hooks/useTickets', () => ({
  useTicketList: () => ({
    data: {
      tickets: [
        { id: 'T-1', title: 'Ticket One' },
        { id: 'T-2', title: 'Ticket Two' },
      ],
    },
    isLoading: false,
  }),
}))

// Expose a button so tests can trigger onChange
vi.mock('./ChainTicketSelector', () => ({
  ChainTicketSelector: ({ selectedIds, onChange }: any) => (
    <div data-testid="chain-ticket-selector">
      <button onClick={() => onChange(['T-1', 'T-2'])}>Select tickets</button>
      Selected: {selectedIds.length}
    </div>
  ),
}))

// Render items so we can assert order
vi.mock('./ChainOrderList', () => ({
  ChainOrderList: ({ items }: any) => (
    <div data-testid="chain-order-list">
      {items.map((item: any) => (
        <span key={item.ticketId} data-ticket={item.ticketId}>
          {item.ticketId}
        </span>
      ))}
    </div>
  ),
}))

vi.mock('@tanstack/react-query', async () => {
  const actual = await vi.importActual('@tanstack/react-query')
  return {
    ...actual,
    useQuery: (options: any) => mockUseQuery(options),
  }
})

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) => {
    const store = { currentProject: 'test-project', projectsLoaded: true }
    return selector(store)
  }),
}))

function makeWorkflowDef(id: string, overrides: Partial<WorkflowDef> = {}): WorkflowDef {
  return {
    id,
    project_id: 'test-project',
    description: `${id} workflow`,
    scope_type: 'ticket',
    phases: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeChain(overrides: Partial<ChainExecution> = {}): ChainExecution {
  return {
    id: 'chain-123',
    project_id: 'test-project',
    name: 'Test Chain',
    status: 'pending',
    workflow_name: 'feature',
    created_by: 'test-user',
    total_items: 2,
    completed_items: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    items: [
      { id: 'item-1', chain_id: 'chain-123', ticket_id: 'T-1', position: 0, status: 'pending' },
      { id: 'item-2', chain_id: 'chain-123', ticket_id: 'T-2', position: 1, status: 'pending' },
    ],
    ...overrides,
  }
}

function renderDialog(open = true, editChain: ChainExecution | null = null) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onClose = vi.fn()
  return {
    onClose,
    ...render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <CreateChainDialog open={open} onClose={onClose} editChain={editChain} />
        </MemoryRouter>
      </QueryClientProvider>
    ),
  }
}

describe('CreateChainDialog - Preview debounce behavior', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    mockUseCreateChain.mockReturnValue({ mutateAsync: vi.fn(), isPending: false, error: null })
    mockUseUpdateChain.mockReturnValue({ mutateAsync: vi.fn(), isPending: false, error: null })
    mockUseQuery.mockReturnValue({
      data: { feature: makeWorkflowDef('feature') },
      isLoading: false,
    })
    vi.mocked(previewChain).mockResolvedValue({
      ticket_ids: ['T-1', 'T-2'],
      deps: {},
      added_by_deps: [],
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('calls previewChain after 300ms debounce on ticket selection change', async () => {
    renderDialog()

    // Use fireEvent (no internal timer dependencies unlike userEvent)
    fireEvent.click(screen.getByRole('button', { name: /select tickets/i }))

    // Before debounce fires — not called yet
    expect(previewChain).not.toHaveBeenCalled()

    // Advance past debounce; wrap in act to flush React state updates
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300)
    })

    expect(previewChain).toHaveBeenCalledWith({ ticket_ids: ['T-1', 'T-2'] })
  })

  it('does not call previewChain when no tickets are selected', async () => {
    renderDialog()

    // No selection triggered — empty array short-circuits in fetchPreview
    await vi.advanceTimersByTimeAsync(500)

    expect(previewChain).not.toHaveBeenCalled()
  })

  it('renders ChainOrderList after preview resolves', async () => {
    renderDialog()

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /select tickets/i }))
      await vi.advanceTimersByTimeAsync(300)
    })

    expect(screen.getByTestId('chain-order-list')).toBeInTheDocument()
  })

  it('includes ordered_ticket_ids in create mutation payload after preview', async () => {
    const mutateAsync = vi.fn().mockResolvedValue({})
    mockUseCreateChain.mockReturnValue({ mutateAsync, isPending: false, error: null })

    renderDialog()

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /select tickets/i }))
      await vi.advanceTimersByTimeAsync(300)
    })

    // orderedIds is now populated from preview response
    expect(screen.getByTestId('chain-order-list')).toBeInTheDocument()

    // Switch to real timers for userEvent click
    vi.useRealTimers()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({ ordered_ticket_ids: ['T-1', 'T-2'] })
    )
  })
})

describe('CreateChainDialog - Edit mode ordering', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCreateChain.mockReturnValue({ mutateAsync: vi.fn(), isPending: false, error: null })
    mockUseQuery.mockReturnValue({
      data: { feature: makeWorkflowDef('feature') },
      isLoading: false,
    })
    vi.mocked(previewChain).mockResolvedValue({
      ticket_ids: ['T-1', 'T-2'],
      deps: {},
      added_by_deps: [],
    })
  })

  it('initializes ChainOrderList from chain items sorted by position', async () => {
    // Items with swapped creation order but correct positions
    const chain = makeChain({
      items: [
        { id: 'item-2', chain_id: 'chain-123', ticket_id: 'T-2', position: 1, status: 'pending' },
        { id: 'item-1', chain_id: 'chain-123', ticket_id: 'T-1', position: 0, status: 'pending' },
      ],
    })
    mockUseUpdateChain.mockReturnValue({ mutateAsync: vi.fn(), isPending: false, error: null })

    renderDialog(true, chain)

    await waitFor(() => {
      const list = screen.getByTestId('chain-order-list')
      const spans = Array.from(list.querySelectorAll('span'))
      expect(spans[0]).toHaveTextContent('T-1')
      expect(spans[1]).toHaveTextContent('T-2')
    })
  })

  it('includes ordered_ticket_ids in update mutation payload in edit mode', async () => {
    const mutateAsync = vi.fn().mockResolvedValue({})
    mockUseUpdateChain.mockReturnValue({ mutateAsync, isPending: false, error: null })

    const chain = makeChain()

    renderDialog(true, chain)

    // orderedIds populated from editChain.items synchronously in useEffect
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /update/i })).not.toBeDisabled()
    })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /update/i }))

    expect(mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 'chain-123',
        data: expect.objectContaining({ ordered_ticket_ids: ['T-1', 'T-2'] }),
      })
    )
  })
})
