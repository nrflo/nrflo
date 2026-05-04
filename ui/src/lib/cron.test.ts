import { describe, it, expect } from 'vitest'
import { formatCountdown, computeNextRuns } from './cron'

describe('formatCountdown', () => {
  const base = new Date('2026-01-01T00:00:00.000Z')

  it('returns "now" when target is within 500ms window (future)', () => {
    expect(formatCountdown(new Date(base.getTime() + 400), base)).toBe('now')
  })

  it('returns "now" when target is within 500ms window (past)', () => {
    expect(formatCountdown(new Date(base.getTime() - 400), base)).toBe('now')
  })

  it('returns "now" at exactly the 500ms boundary', () => {
    expect(formatCountdown(new Date(base.getTime() + 500), base)).toBe('now')
  })

  it('returns "overdue" when target is in the past beyond window', () => {
    expect(formatCountdown(new Date(base.getTime() - 1000), base)).toBe('overdue')
  })

  it('returns "in Xs" for seconds-only countdowns', () => {
    expect(formatCountdown(new Date(base.getTime() + 30_000), base)).toBe('in 30s')
  })

  it('returns "in Xm Ys" when both minutes and seconds are nonzero', () => {
    // 1m 30s
    expect(formatCountdown(new Date(base.getTime() + 90_000), base)).toBe('in 1m 30s')
  })

  it('returns "in Xm" when seconds component is zero', () => {
    // 2m exactly
    expect(formatCountdown(new Date(base.getTime() + 120_000), base)).toBe('in 2m')
  })

  it('returns "in Xh Ym" when both hours and minutes are nonzero', () => {
    // 1h 1m
    expect(formatCountdown(new Date(base.getTime() + 3_660_000), base)).toBe('in 1h 1m')
  })

  it('returns "in Xh" when minutes component is zero', () => {
    // 2h exactly
    expect(formatCountdown(new Date(base.getTime() + 7_200_000), base)).toBe('in 2h')
  })

  it('returns "in Xd Yh" for day-scale countdowns', () => {
    // 90000s = 1d 1h
    expect(formatCountdown(new Date(base.getTime() + 90_000_000), base)).toBe('in 1d 1h')
  })

  it('shows only top 2 units when all four are nonzero', () => {
    // 1d 2h 3m 4s = 93784s → parts ["1d","2h","3m","4s"] → slice(0,2) = ["1d","2h"]
    expect(formatCountdown(new Date(base.getTime() + 93_784_000), base)).toBe('in 1d 2h')
  })
})

describe('computeNextRuns', () => {
  const from = new Date('2026-01-01T00:00:00.000Z')

  it('returns count strictly-increasing Dates for a valid expression', () => {
    const results = computeNextRuns('*/5 * * * *', 5, from)
    expect(results).toHaveLength(5)
    for (let i = 0; i < results.length; i++) {
      expect(results[i]).toBeInstanceOf(Date)
      if (i > 0) {
        expect(results[i].getTime()).toBeGreaterThan(results[i - 1].getTime())
      }
    }
  })

  it('returns empty array for an invalid expression', () => {
    expect(computeNextRuns('xyz invalid', 5, from)).toEqual([])
  })

  it('respects the from parameter — later from yields later results', () => {
    const from2 = new Date('2026-06-01T00:00:00.000Z')
    const [r1] = computeNextRuns('*/5 * * * *', 1, from)
    const [r2] = computeNextRuns('*/5 * * * *', 1, from2)
    expect(r2.getTime()).toBeGreaterThan(r1.getTime())
  })

  it('returns exactly count dates', () => {
    expect(computeNextRuns('0 * * * *', 3, from)).toHaveLength(3)
  })

  it('returns empty array when count is 0', () => {
    expect(computeNextRuns('*/5 * * * *', 0, from)).toEqual([])
  })
})
