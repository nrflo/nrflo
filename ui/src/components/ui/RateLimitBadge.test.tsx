import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { RateLimitBadge } from './RateLimitBadge'

afterEach(() => {
  vi.useRealTimers()
})

describe('RateLimitBadge', () => {
  it('renders badge with countdown when untilTs is in the future', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00Z'))

    render(<RateLimitBadge untilTs="2026-01-01T00:01:00Z" />)

    expect(screen.getByText('Rate-limited, retrying in 1m 0s')).toBeInTheDocument()
  })

  it('returns null when untilTs is exactly now', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:01:00Z'))

    const { container } = render(<RateLimitBadge untilTs="2026-01-01T00:01:00Z" />)
    expect(container.firstChild).toBeNull()
  })

  it('returns null when untilTs is in the past', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:02:00Z'))

    const { container } = render(<RateLimitBadge untilTs="2026-01-01T00:01:00Z" />)
    expect(container.firstChild).toBeNull()
  })

  it('updates countdown text after advancing the clock by 10s', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00Z'))

    render(<RateLimitBadge untilTs="2026-01-01T00:00:30Z" />)
    expect(screen.getByText('Rate-limited, retrying in 30s')).toBeInTheDocument()

    act(() => {
      vi.advanceTimersByTime(10_000)
    })

    expect(screen.getByText('Rate-limited, retrying in 20s')).toBeInTheDocument()
  })

  it('disappears when countdown reaches expiry', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00Z'))

    const { container } = render(<RateLimitBadge untilTs="2026-01-01T00:00:05Z" />)
    expect(screen.getByText('Rate-limited, retrying in 5s')).toBeInTheDocument()

    act(() => {
      vi.advanceTimersByTime(6_000)
    })

    expect(container.firstChild).toBeNull()
  })

  it('badge has amber styling class', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00Z'))

    const { container } = render(<RateLimitBadge untilTs="2026-01-01T00:01:00Z" />)
    expect(container.querySelector('.bg-amber-100')).toBeInTheDocument()
  })
})
