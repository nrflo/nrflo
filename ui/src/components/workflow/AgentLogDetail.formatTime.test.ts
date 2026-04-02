import { describe, it, expect } from 'vitest'
import { formatTime } from './AgentLogDetail'

describe('formatTime', () => {
  it('returns empty string for empty input', () => {
    expect(formatTime('')).toBe('')
  })

  it('does not include AM/PM for afternoon times', () => {
    // 2:20:10 PM UTC — should display as 14:20:10 not 02:20:10
    const result = formatTime('2026-01-15T14:20:10Z')
    expect(result).not.toMatch(/AM|PM/i)
  })

  it('does not include AM/PM for morning times', () => {
    const result = formatTime('2026-01-15T09:05:30Z')
    expect(result).not.toMatch(/AM|PM/i)
  })

  it('outputs HH:MM:SS format with colons', () => {
    const result = formatTime('2026-01-15T10:30:45Z')
    expect(result).toMatch(/\d{2}:\d{2}:\d{2}/)
  })
})
