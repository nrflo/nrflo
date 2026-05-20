import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { DocumentationPage } from './DocumentationPage'

// Mock react-markdown to avoid parsing complexity in tests
vi.mock('react-markdown', () => ({
  default: ({ children }: { children: string }) => <div data-testid="markdown-content">{children}</div>,
}))

const mockRefetch = vi.fn()

vi.mock('@/hooks/useDocs', () => ({
  useAgentManual: vi.fn(),
}))

import { useAgentManual } from '@/hooks/useDocs'
const mockUseAgentManual = vi.mocked(useAgentManual)

describe('DocumentationPage', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    mockRefetch.mockReset()
    mockUseAgentManual.mockClear()
  })

  function renderPage(initialUrl = '/docs') {
    return render(
      <MemoryRouter initialEntries={[initialUrl]}>
        <QueryClientProvider client={queryClient}>
          <DocumentationPage />
        </QueryClientProvider>
      </MemoryRouter>
    )
  }

  it('renders all four sub-tab buttons and calls useAgentManual with common by default', () => {
    mockUseAgentManual.mockReturnValue({
      data: undefined, isLoading: false, error: null, refetch: mockRefetch, isFetching: false,
    } as ReturnType<typeof useAgentManual>)

    renderPage()

    expect(screen.getByRole('button', { name: 'Common' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'CLI' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Python' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'API' })).toBeInTheDocument()
    expect(mockUseAgentManual).toHaveBeenCalledWith('common')
  })

  it('switches kind when a sub-tab is clicked', async () => {
    const user = userEvent.setup()
    mockUseAgentManual.mockReturnValue({
      data: undefined, isLoading: false, error: null, refetch: mockRefetch, isFetching: false,
    } as ReturnType<typeof useAgentManual>)

    renderPage()

    await user.click(screen.getByRole('button', { name: 'CLI' }))

    expect(mockUseAgentManual).toHaveBeenCalledWith('cli')
  })

  it('falls back to common for invalid ?sub= value', () => {
    mockUseAgentManual.mockReturnValue({
      data: undefined, isLoading: false, error: null, refetch: mockRefetch, isFetching: false,
    } as ReturnType<typeof useAgentManual>)

    renderPage('/docs?sub=bogus')

    expect(mockUseAgentManual).toHaveBeenCalledWith('common')
  })

  it('shows page title always', () => {
    mockUseAgentManual.mockReturnValue({
      data: undefined, isLoading: true, error: null, refetch: mockRefetch, isFetching: true,
    } as ReturnType<typeof useAgentManual>)

    renderPage()

    expect(screen.getByRole('heading', { name: 'Agent Documentation' })).toBeInTheDocument()
  })

  it('shows loading spinner while fetching', () => {
    mockUseAgentManual.mockReturnValue({
      data: undefined, isLoading: true, error: null, refetch: mockRefetch, isFetching: true,
    } as ReturnType<typeof useAgentManual>)

    renderPage()

    // Spinner is rendered — no markdown content shown
    expect(screen.queryByTestId('markdown-content')).not.toBeInTheDocument()
    expect(screen.queryByText(/Failed to load/)).not.toBeInTheDocument()
  })

  it('renders markdown content on success', () => {
    mockUseAgentManual.mockReturnValue({
      data: { content: '# Hello\n\nSome docs here.', title: 'Agent Documentation' },
      isLoading: false, error: null, refetch: mockRefetch, isFetching: false,
    } as ReturnType<typeof useAgentManual>)

    renderPage()

    expect(screen.getByTestId('markdown-content')).toBeInTheDocument()
    expect(screen.getByTestId('markdown-content')).toHaveTextContent('# Hello')
    expect(screen.queryByText(/Failed to load/)).not.toBeInTheDocument()
  })

  it('shows error message when API call fails', () => {
    mockUseAgentManual.mockReturnValue({
      data: undefined, isLoading: false,
      error: new Error('documentation not found'),
      refetch: mockRefetch, isFetching: false,
    } as ReturnType<typeof useAgentManual>)

    renderPage()

    expect(screen.getByText(/Failed to load documentation:/)).toBeInTheDocument()
    expect(screen.queryByTestId('markdown-content')).not.toBeInTheDocument()
  })

  it('refresh button calls refetch', async () => {
    const user = userEvent.setup()
    mockUseAgentManual.mockReturnValue({
      data: { content: 'Some content', title: 'Agent Documentation' },
      isLoading: false, error: null, refetch: mockRefetch, isFetching: false,
    } as ReturnType<typeof useAgentManual>)

    renderPage()

    await user.click(screen.getByRole('button', { name: /refresh/i }))

    expect(mockRefetch).toHaveBeenCalledOnce()
  })

  it('retry button in error state calls refetch', async () => {
    const user = userEvent.setup()
    mockUseAgentManual.mockReturnValue({
      data: undefined, isLoading: false,
      error: new Error('not found'),
      refetch: mockRefetch, isFetching: false,
    } as ReturnType<typeof useAgentManual>)

    renderPage()

    await user.click(screen.getByRole('button', { name: /retry/i }))

    expect(mockRefetch).toHaveBeenCalledOnce()
  })

  it('refresh button is disabled while fetching', () => {
    mockUseAgentManual.mockReturnValue({
      data: { content: 'content', title: 'Agent Documentation' },
      isLoading: false, error: null, refetch: mockRefetch, isFetching: true,
    } as ReturnType<typeof useAgentManual>)

    renderPage()

    expect(screen.getByRole('button', { name: /refresh/i })).toBeDisabled()
  })
})
