import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Dashboard } from './Dashboard'
import type { StatusResponse } from '@/types/ticket'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string }) => unknown) =>
    selector({ currentProject: 'test-project' }),
}))

const mockUseStatus = vi.fn()
vi.mock('@/hooks/useTickets', () => ({
  useStatus: () => mockUseStatus(),
}))

function renderDashboard() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <Dashboard />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

function makeStatus(overrides: Partial<StatusResponse> = {}): StatusResponse {
  return {
    counts: { open: 4, in_progress: 2, closed: 10, blocked: 1, total: 16 },
    ready_count: 3,
    pending_tickets: [],
    recent_closed: [],
    ...overrides,
  }
}

describe('Dashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading spinner while fetching', () => {
    mockUseStatus.mockReturnValue({ data: undefined, isLoading: true, error: null })
    renderDashboard()
    // Only the page-level spinner, stat cards not rendered
    expect(screen.queryByText('Total Tickets')).not.toBeInTheDocument()
  })

  it('shows error message on failure', () => {
    mockUseStatus.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('server offline'),
    })
    renderDashboard()
    expect(screen.getByText('Failed to load dashboard')).toBeInTheDocument()
    expect(screen.getByText(/server offline/)).toBeInTheDocument()
  })

  describe('stat cards', () => {
    beforeEach(() => {
      mockUseStatus.mockReturnValue({
        data: makeStatus(),
        isLoading: false,
        error: null,
      })
    })

    it('renders all four stat card titles', () => {
      renderDashboard()
      expect(screen.getByText('Total Tickets')).toBeInTheDocument()
      expect(screen.getByText('Open')).toBeInTheDocument()
      expect(screen.getByText('In Progress')).toBeInTheDocument()
      expect(screen.getByText('Closed')).toBeInTheDocument()
    })

    it('displays correct counts from status data', () => {
      renderDashboard()
      // Values rendered as text nodes inside the stat cards
      expect(screen.getByText('16')).toBeInTheDocument() // total
      expect(screen.getByText('4')).toBeInTheDocument()  // open
      expect(screen.getByText('2')).toBeInTheDocument()  // in_progress
      expect(screen.getByText('10')).toBeInTheDocument() // closed
    })

    it('Open card shows ready_count description text', () => {
      renderDashboard()
      expect(screen.getByText('3 ready to work on')).toBeInTheDocument()
    })

    it('non-Open cards do not show description text', () => {
      renderDashboard()
      // Only one description line should exist
      const descriptions = screen.queryAllByText(/ready to work on/)
      expect(descriptions).toHaveLength(1)
    })
  })

  it('defaults to zero counts when status data is absent', () => {
    mockUseStatus.mockReturnValue({ data: undefined, isLoading: false, error: null })
    renderDashboard()
    // Renders with zeros via fallback
    const zeros = screen.getAllByText('0')
    expect(zeros.length).toBeGreaterThanOrEqual(4)
  })

  it('shows updated ready_count in Open card description', () => {
    mockUseStatus.mockReturnValue({
      data: makeStatus({ ready_count: 7 }),
      isLoading: false,
      error: null,
    })
    renderDashboard()
    expect(screen.getByText('7 ready to work on')).toBeInTheDocument()
  })
})
