import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { ChainDetailPage } from './ChainDetailPage'
import type { ChainExecution, ChainExecutionItem } from '@/types/chain'

// Mock hooks
const mockUseChain = vi.fn()
const mockUseStartChain = vi.fn()
const mockUseCancelChain = vi.fn()

vi.mock('@/hooks/useChains', () => ({
  useChain: (id: string, options?: any) => mockUseChain(id, options),
  useStartChain: () => mockUseStartChain(),
  useCancelChain: () => mockUseCancelChain(),
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

// Mock CreateChainDialog and AppendToChainDialog
vi.mock('@/components/chains/CreateChainDialog', () => ({
  CreateChainDialog: () => null,
}))

vi.mock('@/components/chains/AppendToChainDialog', () => ({
  AppendToChainDialog: () => null,
}))

function createMockChain(overrides: Partial<ChainExecution> = {}): ChainExecution {
  return {
    id: 'chain-123',
    project_id: 'test-project',
    name: 'Test Chain',
    status: 'pending',
    workflow_name: 'feature',
    created_by: 'test-user',
    total_items: 0,
    completed_items: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    items: [],
    ...overrides,
  }
}

function createMockItem(overrides: Partial<ChainExecutionItem> = {}): ChainExecutionItem {
  return {
    id: 'item-1',
    chain_id: 'chain-123',
    ticket_id: 'TICKET-1',
    position: 0,
    status: 'pending',
    ...overrides,
  }
}

function renderChainDetailPage(chainId = 'chain-123') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/chains/${chainId}`]}>
        <Routes>
          <Route path="/chains/:id" element={<ChainDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('ChainDetailPage - Render States', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('renders loading state', () => {
    mockUseChain.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })

    const { container } = renderChainDetailPage()

    expect(container.querySelector('[class*="spin-sync"]')).toBeInTheDocument()
  })

  it('renders error state when chain not found', () => {
    mockUseChain.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Chain not found'),
    })

    renderChainDetailPage()

    expect(screen.getByText(/chain not found/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /back to chains/i })).toBeInTheDocument()
  })

  it('renders chain details successfully', () => {
    const chain = createMockChain()
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    expect(screen.getByText('Test Chain')).toBeInTheDocument()
    expect(screen.getByText('Pending')).toBeInTheDocument()
  })
})

describe('ChainDetailPage - Chain Header', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('displays chain name in header', () => {
    const chain = createMockChain({ name: 'My Test Chain' })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    expect(screen.getByRole('heading', { name: 'My Test Chain' })).toBeInTheDocument()
  })

  it('displays status badge with correct status', () => {
    const chain = createMockChain({ status: 'running' })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    expect(screen.getByText('Running')).toBeInTheDocument()
  })

  it('displays workflow name', () => {
    const chain = createMockChain({ workflow_name: 'bugfix' })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    expect(screen.getByText(/workflow: bugfix/i)).toBeInTheDocument()
  })


  it('displays items completed count', () => {
    const chain = createMockChain({
      items: [
        createMockItem({ id: 'item-1', position: 0, status: 'completed' }),
        createMockItem({ id: 'item-2', position: 1, status: 'pending' }),
        createMockItem({ id: 'item-3', position: 2, status: 'pending' }),
      ],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    expect(screen.getByText(/1\/3 items completed/i)).toBeInTheDocument()
  })
})

describe('ChainDetailPage - Action Buttons', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows Start and Edit buttons for pending chain', () => {
    const chain = createMockChain({ status: 'pending' })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })

    renderChainDetailPage()

    expect(screen.getByRole('button', { name: /start/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /edit/i })).toBeInTheDocument()
  })

  it('shows Append Tickets and Cancel buttons for running chain', () => {
    const chain = createMockChain({ status: 'running' })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })

    renderChainDetailPage()

    expect(screen.getByRole('button', { name: /append tickets/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /start/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /edit/i })).not.toBeInTheDocument()
  })

  it('shows no action buttons for completed chain', () => {
    const chain = createMockChain({ status: 'completed' })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })

    renderChainDetailPage()

    expect(screen.queryByRole('button', { name: /start/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /edit/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /append tickets/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /cancel/i })).not.toBeInTheDocument()
  })

})

describe('ChainDetailPage - Start Chain Action', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls startChain mutation when Start button is clicked', async () => {
    const user = userEvent.setup()
    const mutateAsync = vi.fn().mockResolvedValue({})
    const chain = createMockChain({ status: 'pending' })

    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })
    mockUseStartChain.mockReturnValue({
      mutateAsync,
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })

    renderChainDetailPage()

    const startButton = screen.getByRole('button', { name: /start/i })
    await user.click(startButton)

    expect(mutateAsync).toHaveBeenCalledWith('chain-123')
  })

  it('disables Start button while mutation is pending', () => {
    const chain = createMockChain({ status: 'pending' })

    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: true,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })

    renderChainDetailPage()

    const startButton = screen.getByRole('button', { name: /start/i })
    expect(startButton).toBeDisabled()
  })

  it('shows error message when start fails', () => {
    const chain = createMockChain({ status: 'pending' })

    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: true,
      error: new Error('Failed to start chain'),
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })

    renderChainDetailPage()

    expect(screen.getByText(/failed to start chain/i)).toBeInTheDocument()
  })
})

describe('ChainDetailPage - Cancel Chain Action', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls cancelChain mutation when Cancel button is clicked', async () => {
    const user = userEvent.setup()
    const mutateAsync = vi.fn().mockResolvedValue({})
    const chain = createMockChain({ status: 'running' })

    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync,
      isPending: false,
      isError: false,
      error: null,
    })

    renderChainDetailPage()

    const cancelButton = screen.getByRole('button', { name: /cancel/i })
    await user.click(cancelButton)

    expect(mutateAsync).toHaveBeenCalledWith('chain-123')
  })

  it('disables Cancel button while mutation is pending', () => {
    const chain = createMockChain({ status: 'running' })

    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: true,
      isError: false,
      error: null,
    })

    renderChainDetailPage()

    const cancelButton = screen.getByRole('button', { name: /cancel/i })
    expect(cancelButton).toBeDisabled()
  })

  it('shows error message when cancel fails', () => {
    const chain = createMockChain({ status: 'running' })

    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: true,
      error: new Error('Failed to cancel chain'),
    })

    renderChainDetailPage()

    expect(screen.getByText(/failed to cancel chain/i)).toBeInTheDocument()
  })
})

describe('ChainDetailPage - Items Table', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('shows empty state when chain has no items', () => {
    const chain = createMockChain({ items: [] })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    expect(screen.getByText(/no items in this chain/i)).toBeInTheDocument()
  })

  it('renders items sorted by position', () => {
    const chain = createMockChain({
      items: [
        createMockItem({ id: 'item-1', ticket_id: 'TICKET-3', position: 2, status: 'pending' }),
        createMockItem({ id: 'item-2', ticket_id: 'TICKET-1', position: 0, status: 'pending' }),
        createMockItem({ id: 'item-3', ticket_id: 'TICKET-2', position: 1, status: 'pending' }),
      ],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    const ticketLinks = screen.getAllByRole('link', { name: /TICKET-/i })
    expect(ticketLinks[0]).toHaveTextContent('TICKET-1')
    expect(ticketLinks[1]).toHaveTextContent('TICKET-2')
    expect(ticketLinks[2]).toHaveTextContent('TICKET-3')
  })

  it('displays ticket IDs as links to ticket detail pages', () => {
    const chain = createMockChain({
      items: [createMockItem({ ticket_id: 'TICKET-ABC' })],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    const link = screen.getByRole('link', { name: 'TICKET-ABC' })
    expect(link).toHaveAttribute('href', '/tickets/TICKET-ABC')
  })

  it('displays item position numbers starting from 1', () => {
    const chain = createMockChain({
      items: [
        createMockItem({ position: 0 }),
        createMockItem({ id: 'item-2', ticket_id: 'TICKET-2', position: 1 }),
      ],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    // Position should be displayed as position + 1
    expect(screen.getByText('1')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
  })

  it('displays item status badges', () => {
    const chain = createMockChain({
      status: 'running',
      items: [
        createMockItem({ id: 'item-1', position: 0, status: 'pending' }),
        createMockItem({ id: 'item-2', ticket_id: 'TICKET-2', position: 1, status: 'completed' }),
        createMockItem({ id: 'item-3', ticket_id: 'TICKET-3', position: 2, status: 'running' }),
      ],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    const { container } = renderChainDetailPage()

    const badges = container.querySelectorAll('span.rounded-full')
    const badgeTexts = Array.from(badges).map((b) => b.textContent)
    expect(badgeTexts).toContain('Pending')
    expect(badgeTexts).toContain('Completed')
    expect(badgeTexts).toContain('Running')
  })
})

describe('ChainDetailPage - Ticket Title Display', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('displays ticket title when ticket_title is provided', () => {
    const chain = createMockChain({
      items: [
        createMockItem({
          ticket_id: 'TICKET-123',
          ticket_title: 'Add user authentication',
          position: 0,
        }),
      ],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    expect(screen.getByText('Add user authentication')).toBeInTheDocument()
    expect(screen.getByText('TICKET-123')).toBeInTheDocument()
  })

  it('does not display ticket title span when ticket_title is undefined', () => {
    const chain = createMockChain({
      items: [
        createMockItem({
          ticket_id: 'TICKET-456',
          ticket_title: undefined,
          position: 0,
        }),
      ],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    const { container } = renderChainDetailPage()

    // Should only show the ticket ID link
    expect(screen.getByText('TICKET-456')).toBeInTheDocument()
    // The title span should not exist in the DOM
    const titleSpans = container.querySelectorAll('span.truncate')
    expect(titleSpans.length).toBe(0)
  })

  it('handles multiple items with mixed ticket_title presence', () => {
    const chain = createMockChain({
      items: [
        createMockItem({
          id: 'item-1',
          ticket_id: 'TICKET-1',
          ticket_title: 'First ticket with title',
          position: 0,
        }),
        createMockItem({
          id: 'item-2',
          ticket_id: 'TICKET-2',
          ticket_title: undefined,
          position: 1,
        }),
        createMockItem({
          id: 'item-3',
          ticket_id: 'TICKET-3',
          ticket_title: 'Third ticket with title',
          position: 2,
        }),
      ],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    // Items with titles should display them
    expect(screen.getByText('First ticket with title')).toBeInTheDocument()
    expect(screen.getByText('Third ticket with title')).toBeInTheDocument()

    // All ticket IDs should be present
    expect(screen.getByText('TICKET-1')).toBeInTheDocument()
    expect(screen.getByText('TICKET-2')).toBeInTheDocument()
    expect(screen.getByText('TICKET-3')).toBeInTheDocument()
  })
})

describe('ChainDetailPage - Spinner on Running Items', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('displays spinner instead of ordinal number when item status is running', () => {
    const chain = createMockChain({
      status: 'running',
      items: [
        createMockItem({
          id: 'item-1',
          ticket_id: 'TICKET-1',
          position: 0,
          status: 'running',
        }),
      ],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    const { container } = renderChainDetailPage()

    // Should find a spinner with role="status"
    const spinner = container.querySelector('[role="status"]')
    expect(spinner).toBeInTheDocument()
    expect(spinner).toHaveClass('spin-sync')

    // Should NOT display the ordinal number "1"
    const itemRow = container.querySelector('.flex.items-center.gap-4.px-4.py-3')
    const ordinalColumn = itemRow?.querySelector('.w-6.shrink-0')
    expect(ordinalColumn?.textContent).not.toContain('1')
  })

  it('handles mixed item statuses - spinner only on running items', () => {
    const chain = createMockChain({
      status: 'running',
      items: [
        createMockItem({
          id: 'item-1',
          ticket_id: 'TICKET-1',
          position: 0,
          status: 'completed',
        }),
        createMockItem({
          id: 'item-2',
          ticket_id: 'TICKET-2',
          position: 1,
          status: 'running',
        }),
        createMockItem({
          id: 'item-3',
          ticket_id: 'TICKET-3',
          position: 2,
          status: 'pending',
        }),
      ],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    const { container } = renderChainDetailPage()

    // Should have exactly one spinner (for the running item)
    const spinners = container.querySelectorAll('[role="status"]')
    expect(spinners.length).toBe(1)

    // Should display ordinal numbers for completed and pending items
    expect(screen.getByText('1')).toBeInTheDocument() // position 0
    expect(screen.getByText('3')).toBeInTheDocument() // position 2
  })

})

describe('ChainDetailPage - Tokens Used Column', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseStartChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseCancelChain.mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('displays formatted token count when total_tokens_used is provided', () => {
    const chain = createMockChain({
      items: [
        createMockItem({
          ticket_id: 'TICKET-1',
          position: 0,
          status: 'completed',
          total_tokens_used: 150000,
        }),
      ],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    expect(screen.getByText('150K tokens')).toBeInTheDocument()
  })

  it('displays em-dash when total_tokens_used is 0', () => {
    const chain = createMockChain({
      items: [
        createMockItem({
          ticket_id: 'TICKET-1',
          position: 0,
          status: 'pending',
          total_tokens_used: 0,
        }),
      ],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    // Em-dash should be displayed for zero tokens
    const emDashes = screen.getAllByText('—')
    expect(emDashes.length).toBeGreaterThan(0)
  })

  it('handles multiple items with varying token counts', () => {
    const chain = createMockChain({
      items: [
        createMockItem({
          id: 'item-1',
          ticket_id: 'TICKET-1',
          position: 0,
          status: 'completed',
          total_tokens_used: 150000,
        }),
        createMockItem({
          id: 'item-2',
          ticket_id: 'TICKET-2',
          position: 1,
          status: 'running',
          total_tokens_used: 0,
        }),
        createMockItem({
          id: 'item-3',
          ticket_id: 'TICKET-3',
          position: 2,
          status: 'pending',
          total_tokens_used: undefined,
        }),
        createMockItem({
          id: 'item-4',
          ticket_id: 'TICKET-4',
          position: 3,
          status: 'completed',
          total_tokens_used: 80000,
        }),
      ],
    })
    mockUseChain.mockReturnValue({
      data: chain,
      isLoading: false,
      error: null,
    })

    renderChainDetailPage()

    // Completed items with token data
    expect(screen.getByText('150K tokens')).toBeInTheDocument()
    expect(screen.getByText('80K tokens')).toBeInTheDocument()

    // Items without tokens (running, pending, or 0)
    const emDashes = screen.getAllByText('—')
    expect(emDashes.length).toBeGreaterThanOrEqual(2)
  })

})
