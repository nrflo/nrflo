import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { DailyStats } from './DailyStats'
import type { DailyStats as DailyStatsType } from '@/types/ticket'

// Mock useDailyStats hook
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

describe('DailyStats - Rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders all 4 metrics with correct values', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats(),
      isLoading: false,
    })

    renderDailyStats()

    // Check for created metric
    expect(screen.getByText('3 created')).toBeInTheDocument()

    // Check for closed metric
    expect(screen.getByText('2 closed')).toBeInTheDocument()

    // Check for tokens metric (125000 -> 125K)
    expect(screen.getByText('125K tokens')).toBeInTheDocument()

    // Check for agent time metric (8100 sec -> 2h 15m)
    expect(screen.getByText('2h 15m')).toBeInTheDocument()
  })

  it('returns null when loading', () => {
    mockUseDailyStats.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    const { container } = renderDailyStats()
    expect(container.firstChild).toBeNull()
  })

  it('returns null when data is undefined', () => {
    mockUseDailyStats.mockReturnValue({
      data: undefined,
      isLoading: false,
    })

    const { container } = renderDailyStats()
    expect(container.firstChild).toBeNull()
  })

  it('has responsive hidden class for small screens', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats(),
      isLoading: false,
    })

    const { container } = renderDailyStats()
    const statsDiv = container.querySelector('.hidden')
    expect(statsDiv).toBeInTheDocument()
    expect(statsDiv).toHaveClass('sm:flex')
  })
})

describe('DailyStats - Token Formatting', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('formats zero tokens correctly', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ tokens_spent: 0 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('0 tokens')).toBeInTheDocument()
  })

  it('formats small token counts (under 1000) as plain numbers', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ tokens_spent: 500 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('500 tokens')).toBeInTheDocument()
  })

  it('formats thousands with K suffix', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ tokens_spent: 1000 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('1K tokens')).toBeInTheDocument()
  })

  it('formats non-round thousands with one decimal place', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ tokens_spent: 1500 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('1.5K tokens')).toBeInTheDocument()
  })

  it('formats millions with M suffix', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ tokens_spent: 1000000 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('1M tokens')).toBeInTheDocument()
  })

  it('formats non-round millions with one decimal place', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ tokens_spent: 1200000 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('1.2M tokens')).toBeInTheDocument()
  })

  it('formats typical 125K token count correctly', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ tokens_spent: 125000 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('125K tokens')).toBeInTheDocument()
  })
})

describe('DailyStats - Agent Time Formatting', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('formats zero time as 0s', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ agent_time_sec: 0 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('0s')).toBeInTheDocument()
  })

  it('formats seconds only for values under 60', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ agent_time_sec: 45 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('45s')).toBeInTheDocument()
  })

  it('formats minutes and seconds for values between 60 and 3600', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ agent_time_sec: 2700 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('45m')).toBeInTheDocument()
  })

  it('formats hours and minutes for values over 3600', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ agent_time_sec: 8100 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('2h 15m')).toBeInTheDocument()
  })

  it('formats just hours when minutes is 0', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ agent_time_sec: 7200 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('2h')).toBeInTheDocument()
  })

  it('formats just minutes when seconds is 0', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ agent_time_sec: 180 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('3m')).toBeInTheDocument()
  })

  it('formats minutes with seconds for typical work time', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({ agent_time_sec: 3700 }),
      isLoading: false,
    })

    renderDailyStats()
    expect(screen.getByText('1h 1m')).toBeInTheDocument()
  })
})

describe('DailyStats - Zero Values', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('handles all zero values gracefully', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({
        tickets_created: 0,
        tickets_closed: 0,
        tokens_spent: 0,
        agent_time_sec: 0,
      }),
      isLoading: false,
    })

    renderDailyStats()

    expect(screen.getByText('0 created')).toBeInTheDocument()
    expect(screen.getByText('0 closed')).toBeInTheDocument()
    expect(screen.getByText('0 tokens')).toBeInTheDocument()
    expect(screen.getByText('0s')).toBeInTheDocument()
  })

  it('handles mixed zero and non-zero values', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({
        tickets_created: 5,
        tickets_closed: 0,
        tokens_spent: 0,
        agent_time_sec: 3600,
      }),
      isLoading: false,
    })

    renderDailyStats()

    expect(screen.getByText('5 created')).toBeInTheDocument()
    expect(screen.getByText('0 closed')).toBeInTheDocument()
    expect(screen.getByText('0 tokens')).toBeInTheDocument()
    expect(screen.getByText('1h')).toBeInTheDocument()
  })
})

describe('DailyStats - Icons', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders all required icons', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats(),
      isLoading: false,
    })

    const { container } = renderDailyStats()

    // Check that icons are rendered (lucide-react icons have SVG elements)
    const svgs = container.querySelectorAll('svg')
    expect(svgs.length).toBe(4) // PlusCircle, CheckCircle2, Cpu, Clock
  })

  it('icons have correct small size classes', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats(),
      isLoading: false,
    })

    const { container } = renderDailyStats()

    const svgs = container.querySelectorAll('svg')
    svgs.forEach((svg) => {
      expect(svg).toHaveClass('h-3.5', 'w-3.5')
    })
  })
})

describe('DailyStats - Styling', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('uses muted text styling', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats(),
      isLoading: false,
    })

    const { container } = renderDailyStats()
    const statsDiv = container.querySelector('.text-muted-foreground')
    expect(statsDiv).toBeInTheDocument()
  })

  it('uses small text size (text-xs)', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats(),
      isLoading: false,
    })

    const { container } = renderDailyStats()
    const statsDiv = container.querySelector('.text-xs')
    expect(statsDiv).toBeInTheDocument()
  })

  it('uses horizontal flex layout with gaps', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats(),
      isLoading: false,
    })

    const { container } = renderDailyStats()
    const statsDiv = container.querySelector('.items-center')
    expect(statsDiv).toBeInTheDocument()
    expect(statsDiv).toHaveClass('gap-3')
  })
})

describe('DailyStats - Edge Cases', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('handles very large ticket counts', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({
        tickets_created: 999,
        tickets_closed: 1000,
      }),
      isLoading: false,
    })

    renderDailyStats()

    expect(screen.getByText('999 created')).toBeInTheDocument()
    expect(screen.getByText('1000 closed')).toBeInTheDocument()
  })

  it('handles very large token counts', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({
        tokens_spent: 5500000, // 5.5M
      }),
      isLoading: false,
    })

    renderDailyStats()

    expect(screen.getByText('5.5M tokens')).toBeInTheDocument()
  })

  it('handles very large agent time (multi-day)', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({
        agent_time_sec: 86400, // 24 hours
      }),
      isLoading: false,
    })

    renderDailyStats()

    expect(screen.getByText('24h')).toBeInTheDocument()
  })

  it('handles transition from loading to data', () => {
    mockUseDailyStats.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    const { container, rerender } = renderDailyStats()

    // Initially null
    expect(container.firstChild).toBeNull()

    // Update to have data
    mockUseDailyStats.mockReturnValue({
      data: createMockStats(),
      isLoading: false,
    })

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <DailyStats />
        </MemoryRouter>
      </QueryClientProvider>
    )

    // Should now render stats
    expect(screen.getByText('3 created')).toBeInTheDocument()
  })

  it('handles single digit values', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats({
        tickets_created: 1,
        tickets_closed: 1,
        tokens_spent: 1,
        agent_time_sec: 1,
      }),
      isLoading: false,
    })

    renderDailyStats()

    expect(screen.getByText('1 created')).toBeInTheDocument()
    expect(screen.getByText('1 closed')).toBeInTheDocument()
    expect(screen.getByText('1 tokens')).toBeInTheDocument()
    expect(screen.getByText('1s')).toBeInTheDocument()
  })
})

describe('DailyStats - Data States', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('handles isLoading true with undefined data', () => {
    mockUseDailyStats.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    const { container } = renderDailyStats()
    expect(container.firstChild).toBeNull()
  })

  it('handles isLoading false with undefined data', () => {
    mockUseDailyStats.mockReturnValue({
      data: undefined,
      isLoading: false,
    })

    const { container } = renderDailyStats()
    expect(container.firstChild).toBeNull()
  })

  it('renders when isLoading false with valid data', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats(),
      isLoading: false,
    })

    const { container } = renderDailyStats()
    expect(container.firstChild).not.toBeNull()
  })

  it('does not render when isLoading true even with stale data', () => {
    mockUseDailyStats.mockReturnValue({
      data: createMockStats(),
      isLoading: true,
    })

    const { container } = renderDailyStats()
    // Component returns null if isLoading is true
    expect(container.firstChild).toBeNull()
  })
})
