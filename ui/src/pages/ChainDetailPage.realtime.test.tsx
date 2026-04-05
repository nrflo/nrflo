import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ChainDetailPage } from './ChainDetailPage'
import { useTickingClock } from '@/hooks/useElapsedTime'
import type { ChainExecution } from '@/types/chain'

const mockUseChain = vi.fn()
const mockUseStartChain = vi.fn()
const mockUseCancelChain = vi.fn()

vi.mock('@/hooks/useChains', () => ({
  useChain: (id: string, options?: unknown) => mockUseChain(id, options),
  useStartChain: () => mockUseStartChain(),
  useCancelChain: () => mockUseCancelChain(),
  useRemoveFromChain: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
}))

vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector: (s: object) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

vi.mock('@/components/chains/CreateChainDialog', () => ({ CreateChainDialog: () => null }))
vi.mock('@/components/chains/AppendToChainDialog', () => ({ AppendToChainDialog: () => null }))

function createMockChain(overrides: Partial<ChainExecution> = {}): ChainExecution {
  return {
    id: 'chain-1',
    project_id: 'test-project',
    name: 'Test Chain',
    status: 'pending',
    workflow_name: 'feature',
    created_by: 'user',
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
      <MemoryRouter initialEntries={['/chains/chain-1']}>
        <Routes>
          <Route path="/chains/:id" element={<ChainDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

function defaultMutations() {
  const stub = { mutateAsync: vi.fn(), isPending: false, isError: false, error: null }
  mockUseStartChain.mockReturnValue(stub)
  mockUseCancelChain.mockReturnValue(stub)
}

describe('ChainDetailPage - useTickingClock', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    defaultMutations()
    vi.mocked(useTickingClock).mockClear()
  })

  it('calls useTickingClock(true) when chain is running', () => {
    mockUseChain.mockReturnValue({ data: createMockChain({ status: 'running' }), isLoading: false, error: null })
    renderPage()
    expect(vi.mocked(useTickingClock)).toHaveBeenCalledWith(true)
  })

  it('calls useTickingClock(false) when chain is completed', () => {
    mockUseChain.mockReturnValue({ data: createMockChain({ status: 'completed' }), isLoading: false, error: null })
    renderPage()
    expect(vi.mocked(useTickingClock)).toHaveBeenCalledWith(false)
  })

  it('calls useTickingClock(false) when chain is pending', () => {
    mockUseChain.mockReturnValue({ data: createMockChain({ status: 'pending' }), isLoading: false, error: null })
    renderPage()
    expect(vi.mocked(useTickingClock)).toHaveBeenCalledWith(false)
  })

  it('calls useTickingClock(false) when chain data is undefined (loading)', () => {
    mockUseChain.mockReturnValue({ data: undefined, isLoading: true, error: null })
    renderPage()
    expect(vi.mocked(useTickingClock)).toHaveBeenCalledWith(false)
  })
})

describe('ChainDetailPage - refetchInterval', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    defaultMutations()
  })

  function capturedRefetchInterval() {
    const [, options] = mockUseChain.mock.calls[0] as [string, Record<string, unknown>]
    return options.refetchInterval as (query: { state: { data: unknown } }) => number | false
  }

  it('refetchInterval function returns 10_000 when chain status is running', () => {
    mockUseChain.mockReturnValue({ data: createMockChain({ status: 'running' }), isLoading: false, error: null })
    renderPage()
    const fn = capturedRefetchInterval()
    expect(fn({ state: { data: { status: 'running' } } })).toBe(10_000)
  })

  it('refetchInterval function returns false when chain status is completed', () => {
    mockUseChain.mockReturnValue({ data: createMockChain({ status: 'completed' }), isLoading: false, error: null })
    renderPage()
    const fn = capturedRefetchInterval()
    expect(fn({ state: { data: { status: 'completed' } } })).toBe(false)
  })

  it('refetchInterval function returns false when query has no data', () => {
    mockUseChain.mockReturnValue({ data: undefined, isLoading: true, error: null })
    renderPage()
    const fn = capturedRefetchInterval()
    expect(fn({ state: { data: undefined } })).toBe(false)
  })
})
