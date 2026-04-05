import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { DailyStats } from './DailyStats'
import type { DailyStats as DailyStatsType } from '@/types/ticket'

// localStorage is not available in threads-pool jsdom — mock it directly
const localStorageData: Record<string, string> = {}
const mockLocalStorage = {
  getItem: (key: string) => localStorageData[key] ?? null,
  setItem: (key: string, value: string) => { localStorageData[key] = value },
  removeItem: (key: string) => { delete localStorageData[key] },
}
Object.defineProperty(global, 'localStorage', {
  writable: true,
  configurable: true,
  value: mockLocalStorage,
})

// Capture the range argument passed to useDailyStats
let capturedRange: string | undefined
const mockUseDailyStats = vi.fn()

vi.mock('@/hooks/useTickets', () => ({
  useDailyStats: (range?: string) => {
    capturedRange = range
    return mockUseDailyStats()
  },
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string }) => unknown) =>
    selector({ currentProject: 'test-project' }),
}))

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

describe('DailyStats - Range Dropdown', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    capturedRange = undefined
    // Clear all localStorage keys
    for (const key of Object.keys(localStorageData)) {
      delete localStorageData[key]
    }
    mockUseDailyStats.mockReturnValue({
      data: createMockStats(),
      isLoading: false,
    })
  })

  it('dropdown is closed by default', () => {
    renderDailyStats()
    expect(screen.queryByText('This Week')).not.toBeInTheDocument()
    expect(screen.queryByText('This Month')).not.toBeInTheDocument()
    expect(screen.queryByText('All Time')).not.toBeInTheDocument()
  })

  it('opens dropdown with all 4 range options on click', async () => {
    const user = userEvent.setup()
    renderDailyStats()

    await user.click(screen.getByRole('button'))

    expect(screen.getByText('Today')).toBeInTheDocument()
    expect(screen.getByText('This Week')).toBeInTheDocument()
    expect(screen.getByText('This Month')).toBeInTheDocument()
    expect(screen.getByText('All Time')).toBeInTheDocument()
  })

  it('passes "today" range to useDailyStats by default', () => {
    renderDailyStats()
    expect(capturedRange).toBe('today')
  })

  it('closes dropdown on Escape key', async () => {
    const user = userEvent.setup()
    renderDailyStats()

    await user.click(screen.getByRole('button'))
    expect(screen.getByText('This Week')).toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(screen.queryByText('This Week')).not.toBeInTheDocument()
  })

  it('closes dropdown on click-outside', async () => {
    const user = userEvent.setup()
    renderDailyStats()

    await user.click(screen.getByRole('button'))
    expect(screen.getByText('This Week')).toBeInTheDocument()

    await user.click(document.body)
    expect(screen.queryByText('This Week')).not.toBeInTheDocument()
  })

  it('selecting "This Week" closes dropdown, updates range, and saves to localStorage', async () => {
    const user = userEvent.setup()
    renderDailyStats()

    await user.click(screen.getByRole('button'))
    await user.click(screen.getByText('This Week'))

    // Dropdown closes
    expect(screen.queryByText('This Month')).not.toBeInTheDocument()
    // Badge appears
    expect(screen.getByText('(7d)')).toBeInTheDocument()
    // localStorage saved with project-scoped key
    expect(localStorageData['nrf_daily_stats_range_test-project']).toBe('week')
    // Hook called with new range
    expect(capturedRange).toBe('week')
  })

  it('selecting "This Month" saves "month" and shows "(30d)" badge', async () => {
    const user = userEvent.setup()
    renderDailyStats()

    await user.click(screen.getByRole('button'))
    await user.click(screen.getByText('This Month'))

    expect(screen.getByText('(30d)')).toBeInTheDocument()
    expect(localStorageData['nrf_daily_stats_range_test-project']).toBe('month')
    expect(capturedRange).toBe('month')
  })

  it('selecting "All Time" saves "all" and shows "(all)" badge', async () => {
    const user = userEvent.setup()
    renderDailyStats()

    await user.click(screen.getByRole('button'))
    await user.click(screen.getByText('All Time'))

    expect(screen.getByText('(all)')).toBeInTheDocument()
    expect(localStorageData['nrf_daily_stats_range_test-project']).toBe('all')
    expect(capturedRange).toBe('all')
  })

  it('no badge shown when range is "today"', () => {
    renderDailyStats()
    expect(screen.queryByText('(7d)')).not.toBeInTheDocument()
    expect(screen.queryByText('(30d)')).not.toBeInTheDocument()
    expect(screen.queryByText('(all)')).not.toBeInTheDocument()
  })

  it('initializes from localStorage and shows badge immediately on mount', () => {
    localStorageData['nrf_daily_stats_range_test-project'] = 'week'
    renderDailyStats()

    expect(screen.getByText('(7d)')).toBeInTheDocument()
    expect(capturedRange).toBe('week')
  })

  it('ignores invalid localStorage values and defaults to "today"', () => {
    localStorageData['nrf_daily_stats_range_test-project'] = 'invalid'
    renderDailyStats()

    expect(screen.queryByText('(7d)')).not.toBeInTheDocument()
    expect(capturedRange).toBe('today')
  })

  it('uses project-scoped key and does not touch other projects', async () => {
    const user = userEvent.setup()
    renderDailyStats()

    await user.click(screen.getByRole('button'))
    await user.click(screen.getByText('This Week'))

    expect(localStorageData['nrf_daily_stats_range_test-project']).toBe('week')
    expect(localStorageData['nrf_daily_stats_range_other-project']).toBeUndefined()
  })

  it('selecting "Today" from a non-today range removes badge', async () => {
    localStorageData['nrf_daily_stats_range_test-project'] = 'month'
    const user = userEvent.setup()
    renderDailyStats()

    // Badge visible initially
    expect(screen.getByText('(30d)')).toBeInTheDocument()

    await user.click(screen.getByRole('button'))
    await user.click(screen.getByText('Today'))

    expect(screen.queryByText('(30d)')).not.toBeInTheDocument()
    expect(capturedRange).toBe('today')
  })
})
