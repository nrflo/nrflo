import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { UsageLimits } from './UsageLimits'
import type { UsageLimits as UsageLimitsType, ToolUsage } from '@/types/usageLimits'

const mockUseUsageLimits = vi.fn()
vi.mock('@/hooks/useUsageLimits', () => ({
  useUsageLimits: () => mockUseUsageLimits(),
}))

function renderUsageLimits() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <UsageLimits />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

function makeToolUsage(overrides: Partial<ToolUsage> = {}): ToolUsage {
  return {
    available: true,
    session: { used_pct: 48, resets_at: '9pm (Asia/Bangkok)' },
    weekly: { used_pct: 30, resets_at: 'Monday 9pm (Asia/Bangkok)' },
    ...overrides,
  }
}

function makeData(overrides: Partial<UsageLimitsType> = {}): UsageLimitsType {
  return {
    claude: makeToolUsage(),
    codex: makeToolUsage({
      session: { used_pct: 20, resets_at: '22:26' },
      weekly: { used_pct: 15, resets_at: '23:26 on 26 Feb' },
    }),
    fetched_at: '2026-02-20T12:00:00Z',
    ...overrides,
  }
}

describe('UsageLimits - Rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders Claude and Codex metrics with correct labels', () => {
    mockUseUsageLimits.mockReturnValue({ data: makeData(), isLoading: false })
    renderUsageLimits()

    expect(screen.getByText('Claude:')).toBeInTheDocument()
    expect(screen.getByText('Codex:')).toBeInTheDocument()
  })

  it('shows session percentage with 5h label', () => {
    mockUseUsageLimits.mockReturnValue({ data: makeData(), isLoading: false })
    renderUsageLimits()

    expect(screen.getByText('48% 5h')).toBeInTheDocument()
    expect(screen.getByText('20% 5h')).toBeInTheDocument()
  })

  it('shows weekly percentage with wk label', () => {
    mockUseUsageLimits.mockReturnValue({ data: makeData(), isLoading: false })
    renderUsageLimits()

    expect(screen.getByText('30% wk')).toBeInTheDocument()
    expect(screen.getByText('15% wk')).toBeInTheDocument()
  })

  it('shows middle-dot separator between session and weekly', () => {
    mockUseUsageLimits.mockReturnValue({ data: makeData(), isLoading: false })
    renderUsageLimits()

    const dots = screen.getAllByText('·')
    expect(dots.length).toBeGreaterThanOrEqual(1)
  })

  it('has responsive hidden class for small screens', () => {
    mockUseUsageLimits.mockReturnValue({ data: makeData(), isLoading: false })
    const { container } = renderUsageLimits()

    const wrapper = container.querySelector('.hidden')
    expect(wrapper).toBeInTheDocument()
    expect(wrapper).toHaveClass('md:flex')
  })

  it('rounds fractional percentages', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({ session: { used_pct: 48.7, resets_at: '9pm' }, weekly: null }),
      }),
      isLoading: false,
    })
    renderUsageLimits()

    expect(screen.getByText('49% 5h')).toBeInTheDocument()
  })
})

describe('UsageLimits - Color Coding', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('applies green color for < 50% used', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({ session: { used_pct: 49, resets_at: 't' }, weekly: null }),
        codex: makeToolUsage({ available: false }),
      }),
      isLoading: false,
    })
    const { container } = renderUsageLimits()

    const green = container.querySelectorAll('.text-green-500')
    expect(green.length).toBeGreaterThan(0)
  })

  it('applies yellow color for 50-80% used', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({ session: { used_pct: 50, resets_at: 't' }, weekly: null }),
        codex: makeToolUsage({ available: false }),
      }),
      isLoading: false,
    })
    const { container } = renderUsageLimits()

    const yellow = container.querySelectorAll('.text-yellow-500')
    expect(yellow.length).toBeGreaterThan(0)
  })

  it('applies yellow color at 80% boundary', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({ session: { used_pct: 80, resets_at: 't' }, weekly: null }),
        codex: makeToolUsage({ available: false }),
      }),
      isLoading: false,
    })
    const { container } = renderUsageLimits()

    const yellow = container.querySelectorAll('.text-yellow-500')
    expect(yellow.length).toBeGreaterThan(0)
  })

  it('applies red color for > 80% used', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({ session: { used_pct: 81, resets_at: 't' }, weekly: null }),
        codex: makeToolUsage({ available: false }),
      }),
      isLoading: false,
    })
    const { container } = renderUsageLimits()

    const red = container.querySelectorAll('.text-red-500')
    expect(red.length).toBeGreaterThan(0)
  })

  it('applies green to session and weekly independently', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({
          session: { used_pct: 49, resets_at: 't' },
          weekly: { used_pct: 49, resets_at: 't' },
        }),
        codex: makeToolUsage({ available: false }),
      }),
      isLoading: false,
    })
    const { container } = renderUsageLimits()

    const green = container.querySelectorAll('.text-green-500')
    expect(green.length).toBe(2)
  })

  it('applies correct colors independently when session and weekly differ', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({
          session: { used_pct: 90, resets_at: 't' },
          weekly: { used_pct: 30, resets_at: 't' },
        }),
        codex: makeToolUsage({ available: false }),
      }),
      isLoading: false,
    })
    const { container } = renderUsageLimits()

    expect(container.querySelectorAll('.text-red-500').length).toBe(1)
    expect(container.querySelectorAll('.text-green-500').length).toBe(1)
  })
})

describe('UsageLimits - Data States', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns null when loading', () => {
    mockUseUsageLimits.mockReturnValue({ data: undefined, isLoading: true })
    const { container } = renderUsageLimits()
    expect(container.firstChild).toBeNull()
  })

  it('returns null when data is undefined and not loading', () => {
    mockUseUsageLimits.mockReturnValue({ data: undefined, isLoading: false })
    const { container } = renderUsageLimits()
    expect(container.firstChild).toBeNull()
  })

  it('returns null when both tools are unavailable', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({ available: false }),
        codex: makeToolUsage({ available: false }),
      }),
      isLoading: false,
    })
    const { container } = renderUsageLimits()
    expect(container.firstChild).toBeNull()
  })

  it('renders only Claude section when Codex is unavailable', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({ codex: makeToolUsage({ available: false }) }),
      isLoading: false,
    })
    renderUsageLimits()

    expect(screen.getByText('Claude:')).toBeInTheDocument()
    expect(screen.queryByText('Codex:')).toBeNull()
  })

  it('renders only Codex section when Claude is unavailable', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({ claude: makeToolUsage({ available: false }) }),
      isLoading: false,
    })
    renderUsageLimits()

    expect(screen.queryByText('Claude:')).toBeNull()
    expect(screen.getByText('Codex:')).toBeInTheDocument()
  })

  it('skips session metric when session is null', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({ session: null }),
        codex: makeToolUsage({ available: false }),
      }),
      isLoading: false,
    })
    renderUsageLimits()

    expect(screen.queryByText(/5h/)).toBeNull()
    expect(screen.getByText('30% wk')).toBeInTheDocument()
  })

  it('skips weekly metric when weekly is null', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({ weekly: null }),
        codex: makeToolUsage({ available: false }),
      }),
      isLoading: false,
    })
    renderUsageLimits()

    expect(screen.getByText('48% 5h')).toBeInTheDocument()
    expect(screen.queryByText(/wk/)).toBeNull()
  })

  it('omits separator when only session exists', () => {
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({ weekly: null }),
        codex: makeToolUsage({ available: false }),
      }),
      isLoading: false,
    })
    renderUsageLimits()

    expect(screen.queryByText('·')).toBeNull()
  })
})

describe('UsageLimits - Tooltips', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('session tooltip shows Resets at text on hover', async () => {
    const user = userEvent.setup()
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({
          session: { used_pct: 48, resets_at: '9pm (Asia/Bangkok)' },
          weekly: null,
        }),
        codex: makeToolUsage({ available: false }),
      }),
      isLoading: false,
    })
    renderUsageLimits()

    const sessionTrigger = screen.getByText('48% 5h')
    await user.hover(sessionTrigger)

    expect(screen.getByText('Resets at 9pm (Asia/Bangkok)')).toBeInTheDocument()
  })

  it('weekly tooltip shows Resets at text on hover', async () => {
    const user = userEvent.setup()
    mockUseUsageLimits.mockReturnValue({
      data: makeData({
        claude: makeToolUsage({
          session: null,
          weekly: { used_pct: 30, resets_at: 'Monday 9pm (Asia/Bangkok)' },
        }),
        codex: makeToolUsage({ available: false }),
      }),
      isLoading: false,
    })
    renderUsageLimits()

    const weeklyTrigger = screen.getByText('30% wk')
    await user.hover(weeklyTrigger)

    expect(screen.getByText('Resets at Monday 9pm (Asia/Bangkok)')).toBeInTheDocument()
  })
})
