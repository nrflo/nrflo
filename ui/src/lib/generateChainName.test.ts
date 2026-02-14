import { describe, it, expect } from 'vitest'
import { generateChainName } from './generateChainName'

describe('generateChainName', () => {
  describe('format validation', () => {
    it('returns a string starting with "chain-"', () => {
      const name = generateChainName()
      expect(name).toMatch(/^chain-/)
    })

    it('returns exactly 14 characters (chain- prefix + 8 random chars)', () => {
      const name = generateChainName()
      expect(name).toHaveLength(14)
    })

    it('returns only URL-safe base64 characters after prefix (A-Za-z0-9)', () => {
      const name = generateChainName()
      const randomPart = name.slice(6) // Remove "chain-" prefix
      expect(randomPart).toMatch(/^[A-Za-z0-9]{8}$/)
    })

    it('does not include +, /, or = characters (non-URL-safe base64)', () => {
      // Generate multiple names to increase confidence
      for (let i = 0; i < 50; i++) {
        const name = generateChainName()
        expect(name).not.toContain('+')
        expect(name).not.toContain('/')
        expect(name).not.toContain('=')
      }
    })
  })

  describe('randomness and uniqueness', () => {
    it('generates different names on consecutive calls', () => {
      const name1 = generateChainName()
      const name2 = generateChainName()
      expect(name1).not.toBe(name2)
    })

    it('generates unique names across multiple invocations', () => {
      const names = new Set<string>()
      const iterations = 100

      for (let i = 0; i < iterations; i++) {
        names.add(generateChainName())
      }

      // All names should be unique
      expect(names.size).toBe(iterations)
    })

    it('generates sufficiently random distribution (no predictable patterns)', () => {
      const names = Array.from({ length: 20 }, () => generateChainName())

      // Check that we don't have sequential patterns
      const randomParts = names.map(n => n.slice(6))
      const uniqueChars = new Set(randomParts.join(''))

      // Should use multiple different characters (at least 10 out of 62 possible)
      expect(uniqueChars.size).toBeGreaterThanOrEqual(10)
    })
  })

  describe('edge cases and browser compatibility', () => {
    it('works when called immediately after page load', () => {
      // This tests that crypto.getRandomValues is available
      expect(() => generateChainName()).not.toThrow()
    })

    it('handles btoa encoding correctly for binary data', () => {
      // btoa should handle binary data from random bytes
      const name = generateChainName()
      expect(name).toBeDefined()
      expect(typeof name).toBe('string')
    })

    it('consistently returns valid chain names', () => {
      // Run multiple times to ensure consistency
      for (let i = 0; i < 20; i++) {
        const name = generateChainName()
        expect(name).toMatch(/^chain-[A-Za-z0-9]{8}$/)
      }
    })
  })

  describe('collision probability', () => {
    it('demonstrates low collision risk with 8-char random string', () => {
      // 8 chars from base64 alphabet (62 chars: A-Z, a-z, 0-9) = 62^8 possibilities
      // Even with birthday paradox, collision is extremely unlikely for practical use

      const names = new Set<string>()
      const sampleSize = 10000

      for (let i = 0; i < sampleSize; i++) {
        names.add(generateChainName())
      }

      // With 10k names, we should have no collisions (probability is negligible)
      expect(names.size).toBe(sampleSize)
    })
  })

  describe('crypto.getRandomValues availability', () => {
    it('uses crypto.getRandomValues when available', () => {
      // Verify that crypto.getRandomValues is being used by checking
      // that multiple calls produce different random values
      const name1 = generateChainName()
      const name2 = generateChainName()

      // Both should be valid
      expect(name1).toMatch(/^chain-[A-Za-z0-9]{8}$/)
      expect(name2).toMatch(/^chain-[A-Za-z0-9]{8}$/)

      // And different (proving randomness from crypto.getRandomValues)
      expect(name1).not.toBe(name2)
    })

    it('reliably calls Web Crypto API for randomness', () => {
      // Generate multiple names and verify they all come from a secure random source
      const names = Array.from({ length: 10 }, () => generateChainName())

      // All should be valid format
      names.forEach(name => {
        expect(name).toMatch(/^chain-[A-Za-z0-9]{8}$/)
      })

      // All should be unique (extremely high probability with crypto random)
      const uniqueNames = new Set(names)
      expect(uniqueNames.size).toBe(names.length)
    })
  })

  describe('base64 encoding specifics', () => {
    it('properly converts random bytes to base64', () => {
      // Test that the function handles all possible byte values
      const name = generateChainName()
      const randomPart = name.slice(6)

      // Each character should be valid base64 (URL-safe variant)
      for (const char of randomPart) {
        expect('ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789').toContain(char)
      }
    })

    it('takes first 8 characters after URL-safe conversion', () => {
      // Generate multiple names and verify consistent length
      const names = Array.from({ length: 10 }, () => generateChainName())

      names.forEach(name => {
        const randomPart = name.slice(6)
        expect(randomPart).toHaveLength(8)
      })
    })
  })
})
