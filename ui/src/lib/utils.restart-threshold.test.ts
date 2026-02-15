import { describe, it, expect } from 'vitest'
import { isNearRestartThreshold } from './utils'

describe('isNearRestartThreshold', () => {
  it('returns true at and below threshold+15 boundary', () => {
    expect(isNearRestartThreshold(40, 25)).toBe(true)  // exactly threshold+15
    expect(isNearRestartThreshold(39, 25)).toBe(true)  // below boundary
    expect(isNearRestartThreshold(25, 25)).toBe(true)  // at threshold
    expect(isNearRestartThreshold(10, 25)).toBe(true)  // well below
  })

  it('returns false above threshold+15 boundary', () => {
    expect(isNearRestartThreshold(41, 25)).toBe(false) // one above boundary
    expect(isNearRestartThreshold(60, 25)).toBe(false) // well above
  })

  it('works with different threshold values', () => {
    expect(isNearRestartThreshold(16, 1)).toBe(true)
    expect(isNearRestartThreshold(17, 1)).toBe(false)

    expect(isNearRestartThreshold(65, 50)).toBe(true)
    expect(isNearRestartThreshold(66, 50)).toBe(false)

    expect(isNearRestartThreshold(15, 0)).toBe(true)
    expect(isNearRestartThreshold(16, 0)).toBe(false)
  })

  it('handles edge values', () => {
    expect(isNearRestartThreshold(0, 25)).toBe(true)    // context_left=0
    expect(isNearRestartThreshold(-10, 25)).toBe(true)  // negative (defensive)
    expect(isNearRestartThreshold(100, 25)).toBe(false) // full context
    expect(isNearRestartThreshold(100, 85)).toBe(true)  // high threshold
  })
})
