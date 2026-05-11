import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ClaudeLimitsBadge } from './ClaudeLimitsBadge'
import type { ClaudeLimits } from '@/types/claudeLimits'

vi.mock('@/hooks/useClaudeLimits', () => ({
  useClaudeLimits: vi.fn(),
}))

import { useClaudeLimits } from '@/hooks/useClaudeLimits'
const mockUseClaudeLimits = vi.mocked(useClaudeLimits)

function makeLimits(overrides: Partial<ClaudeLimits> = {}): ClaudeLimits {
  return {
    five_hour_used_pct: 45,
    five_hour_resets_at: new Date(Date.now() + 3_600_000).toISOString(),
    seven_day_used_pct: 30,
    seven_day_resets_at: new Date(Date.now() + 7 * 24 * 3_600_000).toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }
}

function renderBadge() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <ClaudeLimitsBadge />
    </QueryClientProvider>,
  )
}

describe('ClaudeLimitsBadge', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-05-11T10:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  describe('null / no-data cases', () => {
    it('renders nothing when data is undefined', () => {
      mockUseClaudeLimits.mockReturnValue({ data: undefined } as any)
      const { container } = renderBadge()
      expect(container.firstChild).toBeNull()
    })

    it('renders nothing when both pcts are null', () => {
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_used_pct: null, seven_day_used_pct: null }),
      } as any)
      const { container } = renderBadge()
      expect(container.firstChild).toBeNull()
    })

    it('renders only 5h pill when seven_day_used_pct is null', () => {
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ seven_day_used_pct: null }),
      } as any)
      renderBadge()
      expect(screen.getByText(/5h:/)).toBeInTheDocument()
      expect(screen.queryByText(/wk:/)).not.toBeInTheDocument()
    })

    it('renders only wk pill when five_hour_used_pct is null', () => {
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_used_pct: null }),
      } as any)
      renderBadge()
      expect(screen.queryByText(/5h:/)).not.toBeInTheDocument()
      expect(screen.getByText(/wk:/)).toBeInTheDocument()
    })
  })

  describe('color threshold classes', () => {
    const recentUpdatedAt = new Date('2026-05-11T10:00:00Z').toISOString()

    it('applies green class at 45%', () => {
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_used_pct: 45, updated_at: recentUpdatedAt }),
      } as any)
      renderBadge()
      const pill = screen.getByText(/5h:/)
      expect(pill.className).toMatch(/green/)
    })

    it('applies yellow class at 70%', () => {
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_used_pct: 70, updated_at: recentUpdatedAt }),
      } as any)
      renderBadge()
      const pill = screen.getByText(/5h:/)
      expect(pill.className).toMatch(/yellow/)
    })

    it('applies red class at 90%', () => {
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_used_pct: 90, updated_at: recentUpdatedAt }),
      } as any)
      renderBadge()
      const pill = screen.getByText(/5h:/)
      expect(pill.className).toMatch(/red/)
    })

    it('applies gray stale class when updated_at is older than 25 minutes', () => {
      const staleTime = new Date('2026-05-11T09:30:00Z').toISOString() // 30 min ago
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_used_pct: 45, updated_at: staleTime }),
      } as any)
      renderBadge()
      const pill = screen.getByText(/5h:/)
      // Stale = gray (muted), not green/yellow/red
      expect(pill.className).not.toMatch(/green/)
      expect(pill.className).not.toMatch(/yellow/)
      expect(pill.className).not.toMatch(/red/)
    })

    it('green applies at boundary <60 (59%)', () => {
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_used_pct: 59, updated_at: recentUpdatedAt }),
      } as any)
      renderBadge()
      expect(screen.getByText(/5h:/).className).toMatch(/green/)
    })

    it('yellow applies at boundary 60%', () => {
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_used_pct: 60, updated_at: recentUpdatedAt }),
      } as any)
      renderBadge()
      expect(screen.getByText(/5h:/).className).toMatch(/yellow/)
    })

    it('red applies at boundary 85%', () => {
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_used_pct: 85, updated_at: recentUpdatedAt }),
      } as any)
      renderBadge()
      expect(screen.getByText(/5h:/).className).toMatch(/red/)
    })
  })

  describe('isPast — shows "?" when reset time has passed', () => {
    it('shows "?" for 5h when five_hour_resets_at is in the past', () => {
      const pastDate = new Date('2026-05-11T09:00:00Z').toISOString() // 1h ago
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_resets_at: pastDate }),
      } as any)
      renderBadge()
      expect(screen.getByText(/5h: \?/)).toBeInTheDocument()
    })

    it('shows percentage for 5h when five_hour_resets_at is in the future', () => {
      const futureDate = new Date('2026-05-11T11:00:00Z').toISOString() // 1h from now
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_used_pct: 45, five_hour_resets_at: futureDate }),
      } as any)
      renderBadge()
      expect(screen.getByText(/5h: 45%/)).toBeInTheDocument()
    })

    it('shows "?" for wk when seven_day_resets_at is in the past', () => {
      const pastDate = new Date('2026-05-10T10:00:00Z').toISOString() // 1 day ago
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ seven_day_resets_at: pastDate }),
      } as any)
      renderBadge()
      expect(screen.getByText(/wk: \?/)).toBeInTheDocument()
    })

    it('shows percentage for wk when seven_day_resets_at is in the future', () => {
      const futureDate = new Date('2026-05-18T10:00:00Z').toISOString()
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ seven_day_used_pct: 30, seven_day_resets_at: futureDate }),
      } as any)
      renderBadge()
      expect(screen.getByText(/wk: 30%/)).toBeInTheDocument()
    })

    it('shows "?" for 5h when five_hour_resets_at is null', () => {
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_resets_at: null }),
      } as any)
      renderBadge()
      // null is treated as not-past by isPast, so shows pct
      expect(screen.getByText(/5h: 45%/)).toBeInTheDocument()
    })
  })

  describe('hover popover', () => {
    function getTrigger() {
      return screen.getByText(/5h:/).closest('div')!
    }

    it('does not show popover before hover', () => {
      mockUseClaudeLimits.mockReturnValue({ data: makeLimits() } as any)
      renderBadge()
      expect(screen.queryByText('Claude Usage Limits')).not.toBeInTheDocument()
    })

    it('shows popover title on mouseenter', () => {
      mockUseClaudeLimits.mockReturnValue({ data: makeLimits() } as any)
      renderBadge()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText('Claude Usage Limits')).toBeInTheDocument()
    })

    it('shows 5-hour window line in popover', () => {
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_used_pct: 45 }),
      } as any)
      renderBadge()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText('5-hour window:')).toBeInTheDocument()
    })

    it('shows 7-day window line in popover', () => {
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ seven_day_used_pct: 30 }),
      } as any)
      renderBadge()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText('7-day window:')).toBeInTheDocument()
    })

    it('shows last-updated text in popover', () => {
      const updatedAt = new Date('2026-05-11T09:55:00Z').toISOString() // 5 min ago
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ updated_at: updatedAt }),
      } as any)
      renderBadge()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText(/Updated 5m ago/)).toBeInTheDocument()
    })

    it('shows "reset overdue" when resets_at is in the past', () => {
      const pastDate = new Date('2026-05-11T09:00:00Z').toISOString()
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_resets_at: pastDate }),
      } as any)
      renderBadge()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText(/reset overdue/)).toBeInTheDocument()
    })

    it('shows stale warning when updated_at older than 25 min', () => {
      const staleTime = new Date('2026-05-11T09:30:00Z').toISOString() // 30 min ago
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ updated_at: staleTime }),
      } as any)
      renderBadge()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText(/Stale/)).toBeInTheDocument()
    })

    it('does not show stale warning when updated_at is recent (< 25 min)', () => {
      const recentTime = new Date('2026-05-11T09:40:00Z').toISOString() // 20 min ago
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ updated_at: recentTime }),
      } as any)
      renderBadge()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.queryByText(/Stale/)).not.toBeInTheDocument()
    })

    it('shows "resets in Xh Ym" for future reset in popover', () => {
      const futureDate = new Date('2026-05-11T11:30:00Z').toISOString() // 1h 30m from now
      mockUseClaudeLimits.mockReturnValue({
        data: makeLimits({ five_hour_resets_at: futureDate }),
      } as any)
      renderBadge()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText(/resets in 1h 30m/)).toBeInTheDocument()
    })
  })
})
