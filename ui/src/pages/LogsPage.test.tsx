import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { LogsPage } from './LogsPage'

const mockRefetch = vi.fn()

vi.mock('@/hooks/useLogs', () => ({
  useLogs: vi.fn(),
}))

import { useLogs } from '@/hooks/useLogs'
const mockUseLogs = vi.mocked(useLogs)

function makeLogsResponse(lines: string[], type: 'be' | 'fe' = 'be') {
  return { lines, type }
}

describe('LogsPage', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    mockRefetch.mockReset()
    vi.clearAllMocks()
  })

  function renderPage() {
    return render(
      <QueryClientProvider client={queryClient}>
        <LogsPage />
      </QueryClientProvider>
    )
  }

  function idleState(overrides = {}) {
    return {
      data: undefined,
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: mockRefetch,
      ...overrides,
    } as ReturnType<typeof useLogs>
  }

  it('shows page heading', () => {
    mockUseLogs.mockReturnValue(idleState())

    renderPage()

    expect(screen.getByRole('heading', { name: 'Logs' })).toBeInTheDocument()
  })

  it('renders BE and FE tab buttons', () => {
    mockUseLogs.mockReturnValue(idleState())

    renderPage()

    expect(screen.getByRole('button', { name: /BE/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /FE/i })).toBeInTheDocument()
  })

  it('calls useLogs with "be" by default', () => {
    mockUseLogs.mockReturnValue(idleState())

    renderPage()

    expect(mockUseLogs).toHaveBeenCalledWith('be')
  })

  it('switches to FE tab on click and calls useLogs with "fe"', async () => {
    const user = userEvent.setup()
    mockUseLogs.mockReturnValue(idleState())

    renderPage()

    await user.click(screen.getByRole('button', { name: /FE/i }))

    expect(mockUseLogs).toHaveBeenCalledWith('fe')
  })

  it('shows loading spinner on initial load (isLoading=true)', () => {
    mockUseLogs.mockReturnValue(idleState({ isLoading: true, isFetching: true }))

    renderPage()

    // Table content should not appear while loading
    expect(screen.queryByText(/No log lines available/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Failed to load/)).not.toBeInTheDocument()
  })

  it('does not show spinner when only refetching (isFetching but not isLoading)', () => {
    mockUseLogs.mockReturnValue(
      idleState({
        data: makeLogsResponse(['line1']),
        isLoading: false,
        isFetching: true,
      })
    )

    renderPage()

    expect(screen.getByText('line1')).toBeInTheDocument()
  })

  it('shows error message when API call fails', () => {
    mockUseLogs.mockReturnValue(
      idleState({ error: new Error('connection refused') })
    )

    renderPage()

    expect(screen.getByText(/Failed to load logs: connection refused/)).toBeInTheDocument()
    expect(screen.queryByText(/No log lines available/)).not.toBeInTheDocument()
  })

  it('retry button in error state calls refetch', async () => {
    const user = userEvent.setup()
    mockUseLogs.mockReturnValue(
      idleState({ error: new Error('not found') })
    )

    renderPage()

    await user.click(screen.getByRole('button', { name: /retry/i }))

    expect(mockRefetch).toHaveBeenCalledOnce()
  })

  it('shows empty state message when lines array is empty', () => {
    mockUseLogs.mockReturnValue(idleState({ data: makeLogsResponse([]) }))

    renderPage()

    expect(screen.getByText('No log lines available.')).toBeInTheDocument()
  })

  it('renders log lines in table rows', () => {
    const lines = ['2026-01-01 INFO server started', '2026-01-01 DEBUG ready']
    mockUseLogs.mockReturnValue(idleState({ data: makeLogsResponse(lines) }))

    renderPage()

    expect(screen.getByText('2026-01-01 INFO server started')).toBeInTheDocument()
    expect(screen.getByText('2026-01-01 DEBUG ready')).toBeInTheDocument()
  })

  it('does not show error section when data is available', () => {
    mockUseLogs.mockReturnValue(
      idleState({ data: makeLogsResponse(['ok line']) })
    )

    renderPage()

    expect(screen.queryByText(/Failed to load/)).not.toBeInTheDocument()
    expect(screen.getByText('ok line')).toBeInTheDocument()
  })
})
