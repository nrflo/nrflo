import { describe, it, expect } from 'vitest'
import { formatRateLimitCountdown } from './rateLimit'

function fromDiffSec(diffSec: number): string {
  const now = new Date(0)
  const target = new Date(diffSec * 1000)
  return formatRateLimitCountdown(target, now)
}

describe('formatRateLimitCountdown', () => {
  it('returns "" when diff is zero', () => {
    const t = new Date('2026-01-01T00:00:00Z')
    expect(formatRateLimitCountdown(t, t)).toBe('')
  })

  it('returns "" when target is in the past', () => {
    const now = new Date('2026-01-01T00:02:00Z')
    const target = new Date('2026-01-01T00:01:00Z')
    expect(formatRateLimitCountdown(target, now)).toBe('')
  })

  it('1s → "1s"', () => {
    expect(fromDiffSec(1)).toBe('1s')
  })

  it('30s → "30s"', () => {
    expect(fromDiffSec(30)).toBe('30s')
  })

  it('59s → "59s"', () => {
    expect(fromDiffSec(59)).toBe('59s')
  })

  it('90s → "1m 30s"', () => {
    expect(fromDiffSec(90)).toBe('1m 30s')
  })

  it('600s → "10m 0s"', () => {
    expect(fromDiffSec(600)).toBe('10m 0s')
  })

  it('3725s → "1h 2m"', () => {
    expect(fromDiffSec(3725)).toBe('1h 2m')
  })

  it('3600s → "1h 0m"', () => {
    expect(fromDiffSec(3600)).toBe('1h 0m')
  })

  it('uses Math.ceil so sub-second remainder rounds up to 1s', () => {
    const now = new Date(0)
    const target = new Date(500) // 500ms
    expect(formatRateLimitCountdown(target, now)).toBe('1s')
  })
})
