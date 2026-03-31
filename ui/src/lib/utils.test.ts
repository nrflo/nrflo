import { describe, it, expect } from 'vitest'
import { contextLeftColor, formatElapsedTime, formatTokenCount, formatDurationSec, restartReasonLabel, formatRestartReasons } from './utils'

describe('contextLeftColor', () => {
  it('returns red text classes for context_left <= 25', () => {
    const result = contextLeftColor(25)
    expect(result).toContain('text-red-700')
    expect(result).not.toContain('bg-')
  })

  it('returns red text classes for context_left = 0', () => {
    const result = contextLeftColor(0)
    expect(result).toContain('text-red-700')
    expect(result).not.toContain('bg-')
  })

  it('returns red text classes for context_left = 1', () => {
    const result = contextLeftColor(1)
    expect(result).toContain('text-red-700')
    expect(result).not.toContain('bg-')
  })

  it('returns yellow text classes for context_left = 26', () => {
    const result = contextLeftColor(26)
    expect(result).toContain('text-yellow-700')
    expect(result).not.toContain('bg-')
  })

  it('returns yellow text classes for context_left = 50', () => {
    const result = contextLeftColor(50)
    expect(result).toContain('text-yellow-700')
    expect(result).not.toContain('bg-')
  })

  it('returns green text classes for context_left = 51', () => {
    const result = contextLeftColor(51)
    expect(result).toContain('text-green-700')
    expect(result).not.toContain('bg-')
  })

  it('returns green text classes for context_left = 100', () => {
    const result = contextLeftColor(100)
    expect(result).toContain('text-green-700')
    expect(result).not.toContain('bg-')
  })

  it('returns green text classes for context_left = 75', () => {
    const result = contextLeftColor(75)
    expect(result).toContain('text-green-700')
    expect(result).not.toContain('bg-')
  })

  // Dark mode classes
  it('includes dark mode red text classes at threshold', () => {
    const result = contextLeftColor(25)
    expect(result).toContain('dark:text-red-400')
    expect(result).not.toContain('dark:bg-')
  })

  it('includes dark mode yellow text classes at threshold', () => {
    const result = contextLeftColor(50)
    expect(result).toContain('dark:text-yellow-400')
    expect(result).not.toContain('dark:bg-')
  })

  it('includes dark mode green text classes above threshold', () => {
    const result = contextLeftColor(51)
    expect(result).toContain('dark:text-green-400')
    expect(result).not.toContain('dark:bg-')
  })
})

describe('formatElapsedTime', () => {
  it('returns seconds for short durations', () => {
    const start = '2026-01-01T00:00:00Z'
    const end = '2026-01-01T00:00:30Z'
    expect(formatElapsedTime(start, end)).toBe('30s')
  })

  it('returns minutes and seconds for medium durations', () => {
    const start = '2026-01-01T00:00:00Z'
    const end = '2026-01-01T00:02:30Z'
    expect(formatElapsedTime(start, end)).toBe('2m 30s')
  })

  it('returns hours and minutes for long durations', () => {
    const start = '2026-01-01T00:00:00Z'
    const end = '2026-01-01T01:30:00Z'
    expect(formatElapsedTime(start, end)).toBe('1h 30m')
  })

  it('returns 0s for negative durations', () => {
    const start = '2026-01-01T01:00:00Z'
    const end = '2026-01-01T00:00:00Z'
    expect(formatElapsedTime(start, end)).toBe('0s')
  })

  it('returns 0s for same start and end', () => {
    const ts = '2026-01-01T00:00:00Z'
    expect(formatElapsedTime(ts, ts)).toBe('0s')
  })

  it('returns just minutes when seconds is 0', () => {
    const start = '2026-01-01T00:00:00Z'
    const end = '2026-01-01T00:03:00Z'
    expect(formatElapsedTime(start, end)).toBe('3m')
  })

  it('returns just hours when minutes is 0', () => {
    const start = '2026-01-01T00:00:00Z'
    const end = '2026-01-01T02:00:00Z'
    expect(formatElapsedTime(start, end)).toBe('2h')
  })

  it('handles invalid endDate by using current time', () => {
    const start = new Date(Date.now() - 5000) // 5 seconds ago
    const result = formatElapsedTime(start, 'invalid-date')
    // Should be approximately 5s
    expect(result).toMatch(/^\d+s$/)
  })

  it('handles missing endDate by using current time', () => {
    const start = new Date(Date.now() - 60000) // 60 seconds ago
    const result = formatElapsedTime(start)
    // Should be approximately 1m
    expect(result).toMatch(/^1m/)
  })
})

describe('formatTokenCount', () => {
  it('returns plain number for values under 1000', () => {
    expect(formatTokenCount(500)).toBe('500')
  })

  it('returns K suffix for values in thousands', () => {
    expect(formatTokenCount(1000)).toBe('1K')
  })

  it('returns K with decimal for non-round thousands', () => {
    expect(formatTokenCount(1500)).toBe('1.5K')
  })

  it('returns M suffix for values in millions', () => {
    expect(formatTokenCount(1000000)).toBe('1M')
  })

  it('returns M with decimal for non-round millions', () => {
    expect(formatTokenCount(1200000)).toBe('1.2M')
  })

  it('handles typical token counts from context calculation', () => {
    // 200000 * (100-60)/100 = 80000
    expect(formatTokenCount(80000)).toBe('80K')
    // 200000 * (100-25)/100 = 150000
    expect(formatTokenCount(150000)).toBe('150K')
    // Total: 230000
    expect(formatTokenCount(230000)).toBe('230K')
  })

  it('returns 0 for zero', () => {
    expect(formatTokenCount(0)).toBe('0')
  })

  it('handles exact 200K (fully consumed context)', () => {
    expect(formatTokenCount(200000)).toBe('200K')
  })
})

describe('restartReasonLabel', () => {
  it('maps all 7 known reason codes to human-readable labels', () => {
    expect(restartReasonLabel('low_context')).toBe('Low context')
    expect(restartReasonLabel('stall_restart_start_stall')).toBe('Start stall')
    expect(restartReasonLabel('stall_restart_running_stall')).toBe('Running stall')
    expect(restartReasonLabel('instant_stall')).toBe('Instant stall')
    expect(restartReasonLabel('fail_restart')).toBe('Fail restart')
    expect(restartReasonLabel('timeout_restart')).toBe('Timeout restart')
    expect(restartReasonLabel('explicit')).toBe('Manual continue')
  })

  it('returns raw code for unknown reasons', () => {
    expect(restartReasonLabel('some_unknown')).toBe('some_unknown')
    expect(restartReasonLabel('')).toBe('')
  })
})

describe('formatRestartReasons', () => {
  it('returns numbered list from reasons array', () => {
    expect(formatRestartReasons(['low_context', 'explicit'])).toBe('1. Low context\n2. Manual continue')
  })

  it('returns single-item list for one reason', () => {
    expect(formatRestartReasons(['instant_stall'])).toBe('1. Instant stall')
  })

  it('returns count fallback when reasons array is empty', () => {
    expect(formatRestartReasons([], 2)).toBe('2 restarts')
  })

  it('returns singular form for count=1 fallback', () => {
    expect(formatRestartReasons(undefined, 1)).toBe('1 restart')
  })

  it('returns empty string when no reasons and no count', () => {
    expect(formatRestartReasons()).toBe('')
    expect(formatRestartReasons([])).toBe('')
    expect(formatRestartReasons(undefined, 0)).toBe('')
  })
})

describe('formatDurationSec', () => {
  it('returns seconds only for values under 60', () => {
    expect(formatDurationSec(45)).toBe('45s')
  })

  it('returns 0s for zero', () => {
    expect(formatDurationSec(0)).toBe('0s')
  })

  it('returns minutes and seconds for values between 60 and 3600', () => {
    expect(formatDurationSec(125)).toBe('2m 5s')
  })

  it('returns just minutes when seconds is 0', () => {
    expect(formatDurationSec(180)).toBe('3m')
  })

  it('returns hours and minutes for values over 3600', () => {
    expect(formatDurationSec(3700)).toBe('1h 1m')
  })

  it('returns just hours when minutes is 0', () => {
    expect(formatDurationSec(7200)).toBe('2h')
  })

  it('handles large durations', () => {
    expect(formatDurationSec(86400)).toBe('24h')
  })
})
