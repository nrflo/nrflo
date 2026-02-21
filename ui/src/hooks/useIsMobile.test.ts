import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useIsMobile } from './useIsMobile'

type MqlListener = (e: MediaQueryListEvent) => void

function makeMql(matches: boolean) {
  const listeners: MqlListener[] = []
  const mql = {
    matches,
    addEventListener: vi.fn((_type: string, fn: MqlListener) => {
      listeners.push(fn)
    }),
    removeEventListener: vi.fn((_type: string, fn: MqlListener) => {
      const idx = listeners.indexOf(fn)
      if (idx >= 0) listeners.splice(idx, 1)
    }),
    /** Trigger a change event on all registered listeners */
    trigger: (newMatches: boolean) => {
      listeners.forEach(fn => fn({ matches: newMatches } as MediaQueryListEvent))
    },
  }
  return mql
}

describe('useIsMobile', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('returns false when matchMedia does not match (desktop >= 640px)', () => {
    const mql = makeMql(false)
    Object.defineProperty(window, 'matchMedia', { writable: true, value: vi.fn(() => mql) })

    const { result } = renderHook(() => useIsMobile())

    expect(result.current).toBe(false)
  })

  it('returns true when matchMedia matches (mobile < 640px)', () => {
    const mql = makeMql(true)
    Object.defineProperty(window, 'matchMedia', { writable: true, value: vi.fn(() => mql) })

    const { result } = renderHook(() => useIsMobile())

    expect(result.current).toBe(true)
  })

  it('updates to true when viewport shrinks below 640px', () => {
    const mql = makeMql(false)
    Object.defineProperty(window, 'matchMedia', { writable: true, value: vi.fn(() => mql) })

    const { result } = renderHook(() => useIsMobile())
    expect(result.current).toBe(false)

    act(() => { mql.trigger(true) })

    expect(result.current).toBe(true)
  })

  it('updates to false when viewport expands above 640px', () => {
    const mql = makeMql(true)
    Object.defineProperty(window, 'matchMedia', { writable: true, value: vi.fn(() => mql) })

    const { result } = renderHook(() => useIsMobile())
    expect(result.current).toBe(true)

    act(() => { mql.trigger(false) })

    expect(result.current).toBe(false)
  })

  it('registers event listener on mount', () => {
    const mql = makeMql(false)
    Object.defineProperty(window, 'matchMedia', { writable: true, value: vi.fn(() => mql) })

    renderHook(() => useIsMobile())

    expect(mql.addEventListener).toHaveBeenCalledWith('change', expect.any(Function))
  })

  it('removes event listener on unmount', () => {
    const mql = makeMql(false)
    Object.defineProperty(window, 'matchMedia', { writable: true, value: vi.fn(() => mql) })

    const { unmount } = renderHook(() => useIsMobile())
    unmount()

    expect(mql.removeEventListener).toHaveBeenCalledWith('change', expect.any(Function))
  })

  it('queries (max-width: 639px) media query', () => {
    const mql = makeMql(false)
    const matchMediaMock = vi.fn(() => mql)
    Object.defineProperty(window, 'matchMedia', { writable: true, value: matchMediaMock })

    renderHook(() => useIsMobile())

    expect(matchMediaMock).toHaveBeenCalledWith('(max-width: 639px)')
  })
})
