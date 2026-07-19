import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { DailyStats } from './DailyStats'
import type { DailyStats as DailyStatsType } from '@/types/ticket'

const mockUseDailyStats = vi.fn()
vi.mock('@/hooks/useTickets', () => ({
  useDailyStats: () => mockUseDailyStats(),
}))

function renderDailyStats() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <DailyStats />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

function createMockStats(overrides: Partial<DailyStatsType> = {}): DailyStatsType {
  return {
    date: '2026-02-14',
    tickets_created: 3,
    tickets_closed: 2,
    tokens_spent: 125000,
    agent_time_sec: 8100,
    ...overrides,
  }
}

describe('DailyStats - Cost Tile', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the cost tile from data.cost_estimate', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ cost_estimate: 12.345 }),
      isLoading: false,
    })

    const { container } = renderDailyStats()

    expect(screen.getByText('~$12.35')).toBeInTheDocument()
    expect(container.querySelectorAll('svg')).toHaveLength(5)
  })

  it('omits the cost tile when cost_estimate is undefined', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats(),
      isLoading: false,
    })

    const { container } = renderDailyStats()

    expect(screen.queryByText(/^~\$/)).not.toBeInTheDocument()
    expect(container.querySelectorAll('svg')).toHaveLength(4)
  })
})
