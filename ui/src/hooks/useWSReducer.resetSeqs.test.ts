import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import {
  getLastSeq,
  setLastSeq,
  persistSeqs,
  clearSeqs,
  resetSeqs,
} from './useWSReducer'

describe('resetSeqs', () => {
  beforeEach(() => {
    clearSeqs()
    sessionStorage.clear()
  })

  afterEach(() => {
    clearSeqs()
    sessionStorage.clear()
  })

  it('clears the in-memory seqMap', () => {
    setLastSeq('proj:tick', 5)
    expect(getLastSeq('proj:tick')).toBe(5)
    resetSeqs()
    expect(getLastSeq('proj:tick')).toBeUndefined()
  })

  it('removes ws_last_seqs from sessionStorage', () => {
    setLastSeq('proj:tick', 5)
    persistSeqs()
    expect(sessionStorage.getItem('ws_last_seqs')).not.toBeNull()
    resetSeqs()
    expect(sessionStorage.getItem('ws_last_seqs')).toBeNull()
  })

  it('clears multiple subscription keys', () => {
    setLastSeq('proj:a', 1)
    setLastSeq('proj:b', 2)
    persistSeqs()
    resetSeqs()
    expect(getLastSeq('proj:a')).toBeUndefined()
    expect(getLastSeq('proj:b')).toBeUndefined()
    expect(sessionStorage.getItem('ws_last_seqs')).toBeNull()
  })

  it('is safe to call when state is already empty', () => {
    expect(() => resetSeqs()).not.toThrow()
    expect(sessionStorage.getItem('ws_last_seqs')).toBeNull()
  })

  it('does not leave stale data in sessionStorage after persist+reset cycle', () => {
    setLastSeq('p:t', 99)
    persistSeqs()
    resetSeqs()

    // Storage key gone
    expect(sessionStorage.getItem('ws_last_seqs')).toBeNull()

    // A subsequent persist of empty state writes empty object, not old data
    persistSeqs()
    const stored = sessionStorage.getItem('ws_last_seqs')
    if (stored) {
      const parsed = JSON.parse(stored) as Record<string, number>
      expect(Object.keys(parsed)).toHaveLength(0)
    }
  })
})
