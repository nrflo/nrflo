import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { NrvappDashboard } from './Dashboard'
import type { NrvappSummary, NrvappEditRateRow, NrvappThroughputPoint } from '@/types/nrvapp'

vi.mock('recharts', () => ({
  ResponsiveContainer: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  AreaChart: ({ children }: { children: React.ReactNode }) => <div data-testid="area-chart">{children}</div>,
  BarChart: ({ children }: { children: React.ReactNode }) => <div data-testid="bar-chart">{children}</div>,
  Area: () => null,
  Bar: () => null,
  XAxis: () => null,
  YAxis: () => null,
  Tooltip: () => null,
  CartesianGrid: () => null,
  Legend: () => null,
}))

vi.mock('@/components/nrvapp/SummaryCards', () => ({
  SummaryCards: ({ cards }: { cards: Array<{ label: string; value: string | number }> }) => (
    <div data-testid="summary-cards">
      {cards.map((c) => <div key={c.label}>{c.label}</div>)}
    </div>
  ),
}))

vi.mock('@/components/nrvapp/ThroughputChart', () => ({
  ThroughputChart: () => <div data-testid="throughput-chart" />,
}))

vi.mock('@/components/nrvapp/EditRateChart', () => ({
  EditRateChart: () => <div data-testid="edit-rate-chart" />,
}))

vi.mock('@/hooks/useNrvapp', () => ({
  useNrvappSummary: vi.fn(),
  useNrvappEditRate: vi.fn(),
  useNrvappThroughput: vi.fn(),
}))

import { useNrvappSummary, useNrvappEditRate, useNrvappThroughput } from '@/hooks/useNrvapp'

const mockSummary: NrvappSummary = {
  total_dispatches: 100,
  total_reviews: 40,
  pending_reviews: 5,
  approved_rate: 0.75,
  reject_rate: 0.25,
}

const mockEditRate: NrvappEditRateRow[] = [
  { tool_name: 'tool-a', approve_no_edits: 10, approve_with_edits: 5, reject: 2 },
]

const mockThroughput: NrvappThroughputPoint[] = [
  { time: '2026-01-01T00:00:00Z', success: 8, error: 2 },
]

function setupMocks(summary: NrvappSummary | undefined = mockSummary) {
  vi.mocked(useNrvappSummary).mockReturnValue({
    data: summary,
  } as unknown as ReturnType<typeof useNrvappSummary>)
  vi.mocked(useNrvappEditRate).mockReturnValue({
    data: mockEditRate,
  } as unknown as ReturnType<typeof useNrvappEditRate>)
  vi.mocked(useNrvappThroughput).mockReturnValue({
    data: mockThroughput,
  } as unknown as ReturnType<typeof useNrvappThroughput>)
}

function renderPage() {
  return render(
    <MemoryRouter>
      <NrvappDashboard />
    </MemoryRouter>
  )
}

beforeEach(() => vi.clearAllMocks())

describe('NrvappDashboard', () => {
  describe('summary cards', () => {
    it('renders SummaryCards component when summary data available', () => {
      setupMocks()
      renderPage()
      expect(screen.getByTestId('summary-cards')).toBeInTheDocument()
      expect(screen.getByText('Total Dispatches')).toBeInTheDocument()
      expect(screen.getByText('Approve Rate')).toBeInTheDocument()
    })

    it('does not render SummaryCards when summary is undefined', () => {
      vi.mocked(useNrvappSummary).mockReturnValue({
        data: undefined,
      } as unknown as ReturnType<typeof useNrvappSummary>)
      vi.mocked(useNrvappEditRate).mockReturnValue({
        data: mockEditRate,
      } as unknown as ReturnType<typeof useNrvappEditRate>)
      vi.mocked(useNrvappThroughput).mockReturnValue({
        data: mockThroughput,
      } as unknown as ReturnType<typeof useNrvappThroughput>)
      renderPage()
      expect(screen.queryByTestId('summary-cards')).not.toBeInTheDocument()
    })
  })

  describe('charts', () => {
    it('renders throughput chart container', () => {
      setupMocks()
      renderPage()
      expect(screen.getByText('Throughput')).toBeInTheDocument()
    })

    it('renders edit rate chart container', () => {
      setupMocks()
      renderPage()
      expect(screen.getByText('Edit Rate by Tool')).toBeInTheDocument()
    })
  })

  describe('range selector', () => {
    it('renders 7d and 30d range options', () => {
      setupMocks()
      renderPage()
      expect(screen.getByRole('button', { name: '7d' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '30d' })).toBeInTheDocument()
    })

    it('defaults to 7d range and calls hooks with 7d + 1h bucket', () => {
      setupMocks()
      renderPage()
      expect(vi.mocked(useNrvappSummary)).toHaveBeenCalledWith('7d')
      expect(vi.mocked(useNrvappThroughput)).toHaveBeenCalledWith('7d', '1h')
    })

    it('clicking 30d calls hooks with 30d + 6h bucket', async () => {
      const user = userEvent.setup()
      setupMocks()
      renderPage()
      await user.click(screen.getByRole('button', { name: '30d' }))
      expect(vi.mocked(useNrvappSummary)).toHaveBeenCalledWith('30d')
      expect(vi.mocked(useNrvappThroughput)).toHaveBeenCalledWith('30d', '6h')
    })

    it('clicking 7d after 30d reverts hooks to 7d', async () => {
      const user = userEvent.setup()
      setupMocks()
      renderPage()
      await user.click(screen.getByRole('button', { name: '30d' }))
      await user.click(screen.getByRole('button', { name: '7d' }))
      const lastSummaryCalls = vi.mocked(useNrvappSummary).mock.calls
      expect(lastSummaryCalls[lastSummaryCalls.length - 1][0]).toBe('7d')
    })
  })
})
