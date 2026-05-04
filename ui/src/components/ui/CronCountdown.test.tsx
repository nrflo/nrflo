import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { CronCountdown } from './CronCountdown'

afterEach(() => {
  vi.useRealTimers()
})

describe('CronCountdown', () => {
  it('renders em-dash when nextRunAt is undefined', () => {
    render(<CronCountdown />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('renders countdown line and absolute datetime line for a future date', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'))

    const { container } = render(
      <CronCountdown nextRunAt="2026-01-01T01:00:00.000Z" />
    )

    // Countdown: exactly 1h → "in 1h"
    expect(screen.getByText('in 1h')).toBeInTheDocument()

    // Absolute datetime — formatDateTime produces "Jan 1, 2026, ..."
    const paragraphs = container.querySelectorAll('p')
    expect(paragraphs).toHaveLength(2)
    expect(paragraphs[0].textContent).toBe('in 1h')
    expect(paragraphs[1].textContent).toMatch(/2026/)
  })

  it('applies text-destructive class for overdue countdowns', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T01:00:00.000Z'))

    const { container } = render(
      <CronCountdown nextRunAt="2026-01-01T00:00:00.000Z" />
    )

    expect(screen.getByText('overdue')).toBeInTheDocument()
    expect(container.querySelector('.text-destructive')).toBeInTheDocument()
  })

  it('applies text-muted-foreground (not text-destructive) for future countdown', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'))

    const { container } = render(
      <CronCountdown nextRunAt="2026-01-01T01:00:00.000Z" />
    )

    expect(container.querySelector('.text-destructive')).toBeNull()
    expect(container.querySelector('.text-muted-foreground')).toBeInTheDocument()
  })

  it('re-renders on 1-second tick — countdown text updates', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'))

    // 60s from now: totalSec=60, mins=1, secs=0 → "in 1m"
    render(<CronCountdown nextRunAt="2026-01-01T00:01:00.000Z" />)
    expect(screen.getByText('in 1m')).toBeInTheDocument()

    act(() => {
      vi.advanceTimersByTime(1000)
    })

    // After 1s: 59s remaining → totalSec=59, mins=0, secs=59 → "in 59s"
    expect(screen.getByText('in 59s')).toBeInTheDocument()
  })

  it('absolute datetime line has text-xs class', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'))

    const { container } = render(
      <CronCountdown nextRunAt="2026-06-15T14:30:00.000Z" />
    )

    const paragraphs = container.querySelectorAll('p')
    expect(paragraphs[1].className).toContain('text-xs')
  })
})
