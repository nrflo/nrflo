import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TicketListPage } from './TicketListPage'
import type { TicketListResponse } from '@/types/ticket'

vi.mock('@/components/tickets/TicketTable', () => ({
  TicketTable: () => <div data-testid="ticket-table" />,
}))

const mockUseTicketList = vi.fn()
const mockUseTicketSearch = vi.fn()

vi.mock('@/hooks/useTickets', () => ({
  useTicketList: (...args: unknown[]) => mockUseTicketList(...args),
  useTicketSearch: (...args: unknown[]) => mockUseTicketSearch(...args),
}))

function makeListResponse(overrides: Partial<TicketListResponse> = {}): TicketListResponse {
  return {
    tickets: [],
    total_count: 0,
    page: 1,
    per_page: 30,
    total_pages: 1,
    ...overrides,
  }
}

function renderPage(initialUrl = '/tickets') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialUrl]}>
        <TicketListPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

const idle = { isLoading: false, error: null }

describe('TicketListPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTicketList.mockReturnValue({ ...idle, data: makeListResponse() })
    mockUseTicketSearch.mockReturnValue({ ...idle, data: undefined })
  })

  describe('total count display', () => {
    it('shows total_count from server response', () => {
      mockUseTicketList.mockReturnValue({
        ...idle,
        data: makeListResponse({ total_count: 42, total_pages: 2 }),
      })
      renderPage()
      expect(screen.getByText(/42 tickets/)).toBeInTheDocument()
    })

    it('uses singular form for 1 ticket', () => {
      mockUseTicketList.mockReturnValue({
        ...idle,
        data: makeListResponse({ total_count: 1, total_pages: 1 }),
      })
      renderPage()
      expect(screen.getByText(/^1 ticket$/)).toBeInTheDocument()
    })
  })

  describe('pagination controls', () => {
    it('hides pagination when total_pages is 1', () => {
      mockUseTicketList.mockReturnValue({
        ...idle,
        data: makeListResponse({ total_count: 5, total_pages: 1 }),
      })
      renderPage()
      expect(screen.queryByRole('button', { name: /previous/i })).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /next/i })).not.toBeInTheDocument()
    })

    it('shows pagination controls and page indicator when multiple pages', () => {
      mockUseTicketList.mockReturnValue({
        ...idle,
        data: makeListResponse({ total_count: 100, total_pages: 4 }),
      })
      renderPage()
      expect(screen.getByRole('button', { name: /previous/i })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /next/i })).toBeInTheDocument()
      expect(screen.getByText('Page 1 of 4')).toBeInTheDocument()
    })

    it('shows current page from URL in indicator', () => {
      mockUseTicketList.mockReturnValue({
        ...idle,
        data: makeListResponse({ total_count: 150, total_pages: 5 }),
      })
      renderPage('/tickets?page=3')
      expect(screen.getByText('Page 3 of 5')).toBeInTheDocument()
    })

    it('disables Prev on first page', () => {
      mockUseTicketList.mockReturnValue({
        ...idle,
        data: makeListResponse({ total_count: 90, total_pages: 3 }),
      })
      renderPage()
      expect(screen.getByRole('button', { name: /previous/i })).toBeDisabled()
      expect(screen.getByRole('button', { name: /next/i })).not.toBeDisabled()
    })

    it('disables Next on last page', () => {
      mockUseTicketList.mockReturnValue({
        ...idle,
        data: makeListResponse({ total_count: 90, total_pages: 3 }),
      })
      renderPage('/tickets?page=3')
      expect(screen.getByRole('button', { name: /next/i })).toBeDisabled()
      expect(screen.getByRole('button', { name: /previous/i })).not.toBeDisabled()
    })
  })

  describe('pagination navigation', () => {
    it('clicking Next advances page and calls hook with incremented page', async () => {
      const user = userEvent.setup()
      mockUseTicketList.mockReturnValue({
        ...idle,
        data: makeListResponse({ total_count: 90, total_pages: 3 }),
      })
      renderPage('/tickets')

      await user.click(screen.getByRole('button', { name: /next/i }))

      const calls = mockUseTicketList.mock.calls
      expect(calls[calls.length - 1][0]).toMatchObject({ page: 2 })
    })

    it('clicking Prev decrements page and calls hook with decremented page', async () => {
      const user = userEvent.setup()
      mockUseTicketList.mockReturnValue({
        ...idle,
        data: makeListResponse({ total_count: 90, total_pages: 3 }),
      })
      renderPage('/tickets?page=2')

      await user.click(screen.getByRole('button', { name: /previous/i }))

      const calls = mockUseTicketList.mock.calls
      expect(calls[calls.length - 1][0]).toMatchObject({ page: 1 })
    })
  })

  describe('sort params', () => {
    it('passes sort_by and sort_order from URL to the hook', () => {
      renderPage('/tickets?sort_by=priority&sort_order=asc')
      expect(mockUseTicketList.mock.calls[0][0]).toMatchObject({
        sort_by: 'priority',
        sort_order: 'asc',
      })
    })

    it('defaults sort_by to updated_at and sort_order to desc when not in URL', () => {
      renderPage('/tickets')
      expect(mockUseTicketList.mock.calls[0][0]).toMatchObject({
        sort_by: 'updated_at',
        sort_order: 'desc',
      })
    })
  })

  describe('filter changes', () => {
    it('resets page to 1 when status filter changes', async () => {
      const user = userEvent.setup()
      mockUseTicketList.mockReturnValue({
        ...idle,
        data: makeListResponse({ total_count: 90, total_pages: 3 }),
      })
      renderPage('/tickets?status=open&page=2')

      // Open the status dropdown (button showing "Open") and select "All Statuses"
      await user.click(screen.getByRole('button', { name: 'Open' }))
      await user.click(screen.getByText('All Statuses'))

      const calls = mockUseTicketList.mock.calls
      expect(calls[calls.length - 1][0]).toMatchObject({ page: 1 })
    })
  })

  describe('search mode', () => {
    beforeEach(() => {
      mockUseTicketSearch.mockReturnValue({
        ...idle,
        data: makeListResponse({ tickets: [], total_count: 7, total_pages: 1 }),
      })
    })

    it('hides pagination controls during search', () => {
      renderPage('/tickets?search=foo')
      expect(screen.queryByRole('button', { name: /previous/i })).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /next/i })).not.toBeInTheDocument()
    })

    it('shows search result count from returned tickets array length', () => {
      const fakeTickets = Array.from({ length: 7 }, (_, i) => ({
        id: `T-${i}`, title: `Ticket ${i}`, status: 'open' as const,
        priority: 1, issue_type: 'task' as const, is_blocked: false,
        description: null, created_at: '', updated_at: '', closed_at: null,
        created_by: 'user', close_reason: null,
      }))
      mockUseTicketSearch.mockReturnValue({
        ...idle,
        data: makeListResponse({ tickets: fakeTickets, total_count: 7, total_pages: 1 }),
      })
      renderPage('/tickets?search=bug')
      expect(screen.getByText(/7 tickets/)).toBeInTheDocument()
    })

    it('hides filter dropdowns during search', () => {
      renderPage('/tickets?search=foo')
      // Filter/sort dropdowns only rendered when !isSearching
      expect(screen.queryByText('All Statuses')).not.toBeInTheDocument()
      expect(screen.queryByText('All Types')).not.toBeInTheDocument()
    })
  })
})
