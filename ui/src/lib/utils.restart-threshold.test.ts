import { describe, it, expect } from 'vitest'
import { isNearRestartThreshold } from './utils'

describe('isNearRestartThreshold', () => {
  describe('basic functionality', () => {
    it('returns true when context_left equals threshold+10', () => {
      expect(isNearRestartThreshold(35, 25)).toBe(true)
    })

    it('returns true when context_left is less than threshold+10', () => {
      expect(isNearRestartThreshold(30, 25)).toBe(true)
    })

    it('returns true when context_left equals threshold', () => {
      expect(isNearRestartThreshold(25, 25)).toBe(true)
    })

    it('returns true when context_left is below threshold', () => {
      expect(isNearRestartThreshold(20, 25)).toBe(true)
    })

    it('returns false when context_left is greater than threshold+10', () => {
      expect(isNearRestartThreshold(36, 25)).toBe(false)
    })

    it('returns false when context_left is far above threshold', () => {
      expect(isNearRestartThreshold(60, 25)).toBe(false)
    })
  })

  describe('edge cases around threshold boundary', () => {
    it('returns true at exactly threshold+10 (boundary)', () => {
      expect(isNearRestartThreshold(35, 25)).toBe(true)
    })

    it('returns false at threshold+11 (just above boundary)', () => {
      expect(isNearRestartThreshold(36, 25)).toBe(false)
    })

    it('returns true at threshold+9 (just below boundary)', () => {
      expect(isNearRestartThreshold(34, 25)).toBe(true)
    })

    it('returns true at threshold+1', () => {
      expect(isNearRestartThreshold(26, 25)).toBe(true)
    })

    it('returns true at threshold-1', () => {
      expect(isNearRestartThreshold(24, 25)).toBe(true)
    })
  })

  describe('different threshold values', () => {
    it('handles threshold=1 (minimum)', () => {
      expect(isNearRestartThreshold(11, 1)).toBe(true)
      expect(isNearRestartThreshold(12, 1)).toBe(false)
      expect(isNearRestartThreshold(10, 1)).toBe(true)
      expect(isNearRestartThreshold(1, 1)).toBe(true)
      expect(isNearRestartThreshold(0, 1)).toBe(true)
    })

    it('handles threshold=50 (middle range)', () => {
      expect(isNearRestartThreshold(60, 50)).toBe(true)
      expect(isNearRestartThreshold(61, 50)).toBe(false)
      expect(isNearRestartThreshold(50, 50)).toBe(true)
      expect(isNearRestartThreshold(45, 50)).toBe(true)
    })

    it('handles threshold=99 (maximum)', () => {
      expect(isNearRestartThreshold(109, 99)).toBe(true) // theoretical (context can't exceed 100)
      expect(isNearRestartThreshold(110, 99)).toBe(false)
      expect(isNearRestartThreshold(100, 99)).toBe(true)
      expect(isNearRestartThreshold(99, 99)).toBe(true)
    })

    it('handles threshold=0 (edge case)', () => {
      expect(isNearRestartThreshold(10, 0)).toBe(true)
      expect(isNearRestartThreshold(11, 0)).toBe(false)
      expect(isNearRestartThreshold(0, 0)).toBe(true)
    })
  })

  describe('extreme context_left values', () => {
    it('handles context_left=0', () => {
      expect(isNearRestartThreshold(0, 25)).toBe(true)
    })

    it('handles context_left=100', () => {
      expect(isNearRestartThreshold(100, 25)).toBe(false)
      expect(isNearRestartThreshold(100, 90)).toBe(true)
      expect(isNearRestartThreshold(100, 89)).toBe(false)
    })

    it('handles negative context_left (invalid but defensive)', () => {
      expect(isNearRestartThreshold(-10, 25)).toBe(true)
      expect(isNearRestartThreshold(-5, 25)).toBe(true)
    })

    it('handles context_left > 100 (invalid but defensive)', () => {
      expect(isNearRestartThreshold(150, 25)).toBe(false)
    })
  })

  describe('real-world scenarios', () => {
    it('default threshold 25: warns at 35% and below', () => {
      expect(isNearRestartThreshold(35, 25)).toBe(true)
      expect(isNearRestartThreshold(34, 25)).toBe(true)
      expect(isNearRestartThreshold(30, 25)).toBe(true)
      expect(isNearRestartThreshold(25, 25)).toBe(true)
      expect(isNearRestartThreshold(20, 25)).toBe(true)
      expect(isNearRestartThreshold(36, 25)).toBe(false)
      expect(isNearRestartThreshold(40, 25)).toBe(false)
    })

    it('conservative threshold 15: warns at 25% and below', () => {
      expect(isNearRestartThreshold(25, 15)).toBe(true)
      expect(isNearRestartThreshold(26, 15)).toBe(false)
      expect(isNearRestartThreshold(15, 15)).toBe(true)
      expect(isNearRestartThreshold(10, 15)).toBe(true)
    })

    it('aggressive threshold 40: warns at 50% and below', () => {
      expect(isNearRestartThreshold(50, 40)).toBe(true)
      expect(isNearRestartThreshold(51, 40)).toBe(false)
      expect(isNearRestartThreshold(40, 40)).toBe(true)
      expect(isNearRestartThreshold(30, 40)).toBe(true)
    })

    it('very low threshold 5: warns at 15% and below', () => {
      expect(isNearRestartThreshold(15, 5)).toBe(true)
      expect(isNearRestartThreshold(16, 5)).toBe(false)
      expect(isNearRestartThreshold(10, 5)).toBe(true)
      expect(isNearRestartThreshold(5, 5)).toBe(true)
    })
  })

  describe('mathematical properties', () => {
    it('is monotonic: if A <= B and near(A), then near(B)', () => {
      const threshold = 25
      const contextA = 30
      const contextB = 25

      expect(isNearRestartThreshold(contextA, threshold)).toBe(true)
      expect(isNearRestartThreshold(contextB, threshold)).toBe(true)
      expect(contextA > contextB).toBe(true)
    })

    it('boundary is threshold + 10', () => {
      const threshold = 25
      const boundary = threshold + 10

      expect(isNearRestartThreshold(boundary, threshold)).toBe(true)
      expect(isNearRestartThreshold(boundary + 1, threshold)).toBe(false)
      expect(isNearRestartThreshold(boundary - 1, threshold)).toBe(true)
    })

    it('always true when context_left <= threshold', () => {
      const threshold = 30
      for (let ctx = 0; ctx <= threshold; ctx += 5) {
        expect(isNearRestartThreshold(ctx, threshold)).toBe(true)
      }
    })

    it('always false when context_left > threshold + 10', () => {
      const threshold = 30
      for (let ctx = threshold + 11; ctx <= 100; ctx += 10) {
        expect(isNearRestartThreshold(ctx, threshold)).toBe(false)
      }
    })
  })

  describe('type coercion and edge inputs', () => {
    it('handles float values (rounds correctly)', () => {
      expect(isNearRestartThreshold(35.5, 25.5)).toBe(true)
      expect(isNearRestartThreshold(34.9, 25)).toBe(true)
      expect(isNearRestartThreshold(36.1, 25)).toBe(false)
    })

    it('handles very large thresholds', () => {
      expect(isNearRestartThreshold(1000, 990)).toBe(true)
      expect(isNearRestartThreshold(1001, 990)).toBe(false)
    })

    it('handles very small positive thresholds', () => {
      expect(isNearRestartThreshold(0.5, 0.1)).toBe(true)
      expect(isNearRestartThreshold(10.2, 0.1)).toBe(false)
    })
  })
})
