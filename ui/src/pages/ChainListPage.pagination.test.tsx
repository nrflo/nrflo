import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { ChainListPage } from './ChainListPage'
import type { ChainExecution } from '@/types/chain'

const mockNavigate = vi.fn()
const mockUseChainList = vi.fn()

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return { ...actual, useNavigate: () => mockNavigate }
})

vi.mock('@/hooks/useChains', () => ({
  useChainList: (params?: unknown) => mockUseChainList(params),
  useCreateChain: () => ({ mutateAsync: vi.fn(), isPending: false, error: null }),
  useUpdateChain: () => ({ mutateAsync: vi.fn(), isPending: false, error: null }),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector: (s: unknown) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

vi.mock('@/components/chains/CreateChainDialog', () => ({
  CreateChainDialog: () => null,
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

function makeChains(count: number): ChainExecution[] {
  return Array.from({ length: count }, (_, i) =>
    createMockChain({ id: `chain-${i + 1}`, name: `Chain ${i + 1}` })
  )
}

function renderPage(initialRoute = '/chains') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialRoute]}>
        <ChainListPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

const PAGE_SIZE = 20

describe('ChainListPage - Pagination', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('hides pagination controls when chains fit in one page', () => {
    mockUseChainList.mockReturnValue({ data: makeChains(PAGE_SIZE), isLoading: false, error: null })
    renderPage()
    expect(screen.queryByText(new RegExp(`of ${PAGE_SIZE}`))).not.toBeInTheDocument()
  })

  it('shows range text and Prev disabled on first page when chains exceed page size', () => {
    const total = PAGE_SIZE + 5
    mockUseChainList.mockReturnValue({ data: makeChains(total), isLoading: false, error: null })
    renderPage()

    expect(screen.getByText(`1–${PAGE_SIZE} of ${total}`)).toBeInTheDocument()
    expect(document.querySelectorAll('[data-testid="chain-row"]')).toHaveLength(PAGE_SIZE)
    const buttons = screen.getAllByRole('button')
    expect(buttons[buttons.length - 2]).toBeDisabled()    // Prev
    expect(buttons[buttons.length - 1]).not.toBeDisabled() // Next
  })

  it('navigates to next page and back with Prev/Next buttons', async () => {
    const user = userEvent.setup()
    const total = PAGE_SIZE + 5
    mockUseChainList.mockReturnValue({ data: makeChains(total), isLoading: false, error: null })
    renderPage()

    // Go to page 2
    const buttons = screen.getAllByRole('button')
    await user.click(buttons[buttons.length - 1]) // Next

    expect(screen.getByText(`${PAGE_SIZE + 1}–${total} of ${total}`)).toBeInTheDocument()
    expect(document.querySelectorAll('[data-testid="chain-row"]')).toHaveLength(5)

    const page2Buttons = screen.getAllByRole('button')
    expect(page2Buttons[page2Buttons.length - 1]).toBeDisabled()    // Next disabled on last page
    expect(page2Buttons[page2Buttons.length - 2]).not.toBeDisabled() // Prev enabled

    // Go back to page 1
    await user.click(page2Buttons[page2Buttons.length - 2]) // Prev
    expect(screen.getByText(`1–${PAGE_SIZE} of ${total}`)).toBeInTheDocument()
    expect(document.querySelectorAll('[data-testid="chain-row"]')).toHaveLength(PAGE_SIZE)
  })

  it('resets to page 0 when status filter changes', async () => {
    const user = userEvent.setup()
    mockUseChainList.mockReturnValue({ data: makeChains(40), isLoading: false, error: null })
    renderPage()

    // Advance to page 2
    const buttons = screen.getAllByRole('button')
    await user.click(buttons[buttons.length - 1])
    expect(screen.getByText(`${PAGE_SIZE + 1}–40 of 40`)).toBeInTheDocument()

    // Change status filter
    const dropdownBtn = screen.getByText('All Statuses').closest('button')!
    await user.click(dropdownBtn)
    await user.click(screen.getByText('Running'))

    // Page should reset to 0
    expect(screen.getByText(`1–${PAGE_SIZE} of 40`)).toBeInTheDocument()
  })
})

describe('ChainListPage - Row Navigation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('clicking a chain row navigates to the chain detail page', async () => {
    const user = userEvent.setup()
    const chain = createMockChain({ id: 'chain-xyz' })
    mockUseChainList.mockReturnValue({ data: [chain], isLoading: false, error: null })
    renderPage()

    await user.click(screen.getByText('Test Chain'))
    expect(mockNavigate).toHaveBeenCalledWith('/chains/chain-xyz')
  })

  it('shows dash when created_by is empty', () => {
    const chain = createMockChain({ created_by: '' })
    mockUseChainList.mockReturnValue({ data: [chain], isLoading: false, error: null })
    renderPage()

    expect(screen.getByText('-')).toBeInTheDocument()
  })
})
