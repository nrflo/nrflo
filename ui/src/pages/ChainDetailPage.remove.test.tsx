import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { ChainDetailPage } from './ChainDetailPage'
import type { ChainExecution, ChainExecutionItem, ChainStatus, ChainItemStatus } from '@/types/chain'

const mockUseChain = vi.fn()
const mockUseStartChain = vi.fn()
const mockUseCancelChain = vi.fn()
const mockUseRemoveFromChain = vi.fn()

vi.mock('@/hooks/useChains', () => ({
  useChain: (id: string, options?: unknown) => mockUseChain(id, options),
  useStartChain: () => mockUseStartChain(),
  useCancelChain: () => mockUseCancelChain(),
  useRemoveFromChain: () => mockUseRemoveFromChain(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) => {
    const store = { currentProject: 'test-project', projectsLoaded: true }
    return selector(store)
  }),
}))

vi.mock('@/components/chains/CreateChainDialog', () => ({ CreateChainDialog: () => null }))
vi.mock('@/components/chains/AppendToChainDialog', () => ({ AppendToChainDialog: () => null }))

function makeChain(overrides: Partial<ChainExecution> = {}): ChainExecution {
  return {
    id: 'chain-1',
    project_id: 'test-project',
    name: 'Test Chain',
    status: 'running',
    workflow_name: 'feature',
    created_by: 'user',
    total_items: 1,
    completed_items: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    items: [],
    ...overrides,
  }
}

function makeItem(overrides: Partial<ChainExecutionItem> = {}): ChainExecutionItem {
  return {
    id: 'item-1',
    chain_id: 'chain-1',
    ticket_id: 'TICKET-1',
    position: 0,
    status: 'pending',
    ...overrides,
  }
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/chains/chain-1']}>
        <Routes>
          <Route path="/chains/:id" element={<ChainDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

function stubMutations(removeOverrides: Partial<{ mutateAsync: ReturnType<typeof vi.fn>; isPending: boolean; isError: boolean; error: Error | null }> = {}) {
  const noop = { mutateAsync: vi.fn(), isPending: false, isError: false, error: null }
  mockUseStartChain.mockReturnValue(noop)
  mockUseCancelChain.mockReturnValue(noop)
  mockUseRemoveFromChain.mockReturnValue({ ...noop, ...removeOverrides })
}

describe('ChainDetailPage - Remove button visibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    stubMutations()
  })

  it('shows remove button on pending item when chain is running', () => {
    mockUseChain.mockReturnValue({
      data: makeChain({ status: 'running', items: [makeItem({ status: 'pending' })] }),
      isLoading: false,
      error: null,
    })
    renderPage()

    const link = screen.getByRole('link', { name: 'TICKET-1' })
    const row = link.closest('tr')!
    expect(within(row).getByRole('button')).toBeInTheDocument()
  })

  it.each<[ChainStatus, ChainItemStatus]>([
    ['running', 'running'],
    ['running', 'completed'],
    ['running', 'failed'],
    ['running', 'canceled'],
    ['pending', 'pending'],
    ['completed', 'pending'],
    ['failed', 'pending'],
  ])('does not show remove button for chain=%s item=%s', (chainStatus, itemStatus) => {
    mockUseChain.mockReturnValue({
      data: makeChain({ status: chainStatus, items: [makeItem({ status: itemStatus })] }),
      isLoading: false,
      error: null,
    })
    renderPage()

    const link = screen.getByRole('link', { name: 'TICKET-1' })
    const row = link.closest('tr')!
    expect(within(row).queryByRole('button')).not.toBeInTheDocument()
  })
})

describe('ChainDetailPage - Remove confirmation flow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    stubMutations()
  })

  it('clicking the remove button shows inline confirmation', async () => {
    const user = userEvent.setup()
    mockUseChain.mockReturnValue({
      data: makeChain({ items: [makeItem()] }),
      isLoading: false,
      error: null,
    })
    renderPage()

    const link = screen.getByRole('link', { name: 'TICKET-1' })
    const row = link.closest('tr')!
    await user.click(within(row).getByRole('button'))

    // Confirmation row replaces the item row; scope to the colSpan cell
    const confirmCell = screen.getByText(/from chain\?/).closest('td')!
    expect(confirmCell).toHaveTextContent('TICKET-1')
    expect(within(confirmCell).getByRole('button', { name: /^cancel$/i })).toBeInTheDocument()
    expect(within(confirmCell).getByRole('button', { name: /^remove$/i })).toBeInTheDocument()
  })

  it('clicking Cancel hides the confirmation and restores the row', async () => {
    const user = userEvent.setup()
    mockUseChain.mockReturnValue({
      data: makeChain({ items: [makeItem()] }),
      isLoading: false,
      error: null,
    })
    renderPage()

    // Open confirmation
    const link = screen.getByRole('link', { name: 'TICKET-1' })
    await user.click(within(link.closest('tr')!).getByRole('button'))

    // Click Cancel scoped to the confirmation cell (avoids chain Cancel button)
    const confirmCell = screen.getByText(/from chain\?/).closest('td')!
    await user.click(within(confirmCell).getByRole('button', { name: /^cancel$/i }))

    // Row restored with ticket link visible
    expect(screen.getByRole('link', { name: 'TICKET-1' })).toBeInTheDocument()
    expect(screen.queryByText(/from chain\?/)).not.toBeInTheDocument()
  })

  it('clicking Remove calls removeFromChain with correct args', async () => {
    const user = userEvent.setup()
    const mutateAsync = vi.fn().mockResolvedValue({})
    stubMutations({ mutateAsync })
    mockUseChain.mockReturnValue({
      data: makeChain({ items: [makeItem({ ticket_id: 'TICKET-42', chain_id: 'chain-1' })] }),
      isLoading: false,
      error: null,
    })
    renderPage()

    const link = screen.getByRole('link', { name: 'TICKET-42' })
    await user.click(within(link.closest('tr')!).getByRole('button'))
    await user.click(screen.getByRole('button', { name: /^remove$/i }))

    expect(mutateAsync).toHaveBeenCalledWith({
      id: 'chain-1',
      data: { ticket_ids: ['TICKET-42'] },
    })
  })

  it('shows Removing... text and disables button while mutation is pending', async () => {
    const user = userEvent.setup()
    // Pre-set isPending: true so the confirmation row renders in the pending state immediately
    stubMutations({ isPending: true })
    mockUseChain.mockReturnValue({
      data: makeChain({ items: [makeItem()] }),
      isLoading: false,
      error: null,
    })
    renderPage()

    // Trash button still visible (isPending only affects confirmation row, not canRemove)
    const link = screen.getByRole('link', { name: 'TICKET-1' })
    await user.click(within(link.closest('tr')!).getByRole('button'))

    // Confirmation row shows Removing... and the button is disabled
    const confirmCell = screen.getByText(/from chain\?/).closest('td')!
    const removeBtn = within(confirmCell).getByRole('button', { name: /removing/i })
    expect(removeBtn).toBeDisabled()
  })
})

describe('ChainDetailPage - Remove error display', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows error message from mutation when removal fails', () => {
    stubMutations({ isError: true, error: new Error('Item is not pending') })
    mockUseChain.mockReturnValue({
      data: makeChain({ items: [makeItem()] }),
      isLoading: false,
      error: null,
    })
    renderPage()

    expect(screen.getByText('Item is not pending')).toBeInTheDocument()
  })

  it('shows fallback error text when error is null', () => {
    // error: null — (null as Error)?.message is undefined, so ?? 'Remove failed' kicks in
    stubMutations({ isError: true, error: null })
    mockUseChain.mockReturnValue({
      data: makeChain({ items: [makeItem()] }),
      isLoading: false,
      error: null,
    })
    renderPage()

    expect(screen.getByText('Remove failed')).toBeInTheDocument()
  })
})
