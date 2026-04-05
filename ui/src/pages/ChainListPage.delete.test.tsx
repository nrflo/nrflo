import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { ChainListPage } from './ChainListPage'
import type { ChainExecution } from '@/types/chain'

const mockNavigate = vi.fn()
const mockUseChainList = vi.fn()
const mockDeleteMutate = vi.fn()

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return { ...actual, useNavigate: () => mockNavigate }
})

vi.mock('@/hooks/useChains', () => ({
  useChainList: (params?: unknown) => mockUseChainList(params),
  useCreateChain: () => ({ mutateAsync: vi.fn(), isPending: false, error: null }),
  useUpdateChain: () => ({ mutateAsync: vi.fn(), isPending: false, error: null }),
  useDeleteChain: () => ({ mutate: mockDeleteMutate, isPending: false }),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector: (s: unknown) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

vi.mock('@/components/chains/CreateChainDialog', () => ({
  CreateChainDialog: () => null,
}))

function makeChain(overrides: Partial<ChainExecution> = {}): ChainExecution {
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

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/chains']}>
        <ChainListPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('ChainListPage - Delete Button', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders a delete button for each chain row', () => {
    const chains = [
      makeChain({ id: 'chain-1', name: 'Chain One' }),
      makeChain({ id: 'chain-2', name: 'Chain Two' }),
    ]
    mockUseChainList.mockReturnValue({ data: chains, isLoading: false, error: null })
    renderPage()

    // One delete button per row (each contains a Trash2 svg)
    const rows = document.querySelectorAll('[data-testid="chain-row"]')
    expect(rows).toHaveLength(2)
    rows.forEach((row) => {
      expect(row.querySelector('svg')).toBeInTheDocument()
    })
  })

  it('disables delete button when chain status is running', () => {
    mockUseChainList.mockReturnValue({
      data: [makeChain({ status: 'running' })],
      isLoading: false,
      error: null,
    })
    renderPage()

    const row = document.querySelector('[data-testid="chain-row"]')!
    const deleteBtn = row.querySelector('button')!
    expect(deleteBtn).toBeDisabled()
  })

  it.each(['pending', 'completed', 'failed', 'canceled'] as const)(
    'enables delete button when status is %s',
    (status) => {
      mockUseChainList.mockReturnValue({
        data: [makeChain({ status })],
        isLoading: false,
        error: null,
      })
      renderPage()

      const row = document.querySelector('[data-testid="chain-row"]')!
      const deleteBtn = row.querySelector('button')!
      expect(deleteBtn).not.toBeDisabled()
    }
  )

  it('clicking delete button opens ConfirmDialog and does not navigate', async () => {
    const user = userEvent.setup()
    mockUseChainList.mockReturnValue({
      data: [makeChain({ id: 'chain-abc', name: 'My Chain' })],
      isLoading: false,
      error: null,
    })
    renderPage()

    const row = document.querySelector('[data-testid="chain-row"]')!
    const deleteBtn = row.querySelector('button')!
    await user.click(deleteBtn)

    // ConfirmDialog should be open
    expect(screen.getByText('Delete Chain')).toBeInTheDocument()
    // Row click navigation must NOT have fired
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('confirming deletion calls mutate with chain id and closes dialog', async () => {
    const user = userEvent.setup()
    // Make mutate call the onSettled callback so dialog closes
    mockDeleteMutate.mockImplementation((_id: string, opts?: { onSettled?: () => void }) => {
      opts?.onSettled?.()
    })
    mockUseChainList.mockReturnValue({
      data: [makeChain({ id: 'chain-abc' })],
      isLoading: false,
      error: null,
    })
    renderPage()

    // Open dialog
    const deleteBtn = document.querySelector('[data-testid="chain-row"] button')!
    await user.click(deleteBtn)

    // Confirm
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    expect(mockDeleteMutate).toHaveBeenCalledWith('chain-abc', expect.objectContaining({ onSettled: expect.any(Function) }))
    // Dialog should be closed
    expect(screen.queryByText('Delete Chain')).not.toBeInTheDocument()
  })

  it('canceling dialog does not call mutate', async () => {
    const user = userEvent.setup()
    mockUseChainList.mockReturnValue({
      data: [makeChain({ id: 'chain-abc' })],
      isLoading: false,
      error: null,
    })
    renderPage()

    const deleteBtn = document.querySelector('[data-testid="chain-row"] button')!
    await user.click(deleteBtn)

    await user.click(screen.getByRole('button', { name: /cancel/i }))

    expect(mockDeleteMutate).not.toHaveBeenCalled()
    expect(screen.queryByText('Delete Chain')).not.toBeInTheDocument()
  })
})
