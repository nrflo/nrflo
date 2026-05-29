import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { computeReconnectDelay, useConnectionRecovery, MAX_RECONNECT_DELAY } from './useWSReconnect'

describe('computeReconnectDelay', () => {
  afterEach(() => { vi.restoreAllMocks() })

  it('returns value in [base, base+1000] at attempt 0', () => {
    const d = computeReconnectDelay(0, 3000)
    expect(d).toBeGreaterThanOrEqual(3000)
    expect(d).toBeLessThanOrEqual(4000)
  })

  it('doubles each attempt (no jitter)', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0)
    expect(computeReconnectDelay(0, 100)).toBe(100)
    expect(computeReconnectDelay(1, 100)).toBe(200)
    expect(computeReconnectDelay(2, 100)).toBe(400)
    expect(computeReconnectDelay(3, 100)).toBe(800)
  })

  it('clamps to MAX_RECONNECT_DELAY at high attempt numbers (no jitter)', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0)
    expect(computeReconnectDelay(10)).toBe(MAX_RECONNECT_DELAY)
  })

  it('never exceeds MAX_RECONNECT_DELAY + 1000 regardless of attempt', () => {
    for (let i = 0; i < 50; i++) {
      expect(computeReconnectDelay(20)).toBeLessThanOrEqual(MAX_RECONNECT_DELAY + 1000)
    }
  })

  it('is monotonically non-decreasing up to cap (no jitter)', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0)
    let prev = computeReconnectDelay(0, 100)
    for (let i = 1; i <= 10; i++) {
      const d = computeReconnectDelay(i, 100)
      expect(d).toBeGreaterThanOrEqual(prev)
      prev = d
    }
  })

  it('plateaus at MAX_RECONNECT_DELAY once cap is reached (no jitter)', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0)
    expect(computeReconnectDelay(10)).toBe(MAX_RECONNECT_DELAY)
    expect(computeReconnectDelay(15)).toBe(MAX_RECONNECT_DELAY)
    expect(computeReconnectDelay(20)).toBe(MAX_RECONNECT_DELAY)
  })
})

describe('useConnectionRecovery', () => {
  afterEach(() => { vi.restoreAllMocks() })

  it('registers online and visibilitychange listeners when enabled=true', () => {
    const addWin = vi.spyOn(window, 'addEventListener')
    const addDoc = vi.spyOn(document, 'addEventListener')
    const { unmount } = renderHook(() => useConnectionRecovery(vi.fn(), true))
    expect(addWin).toHaveBeenCalledWith('online', expect.any(Function))
    expect(addDoc).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
    unmount()
  })

  it('does not register online listener when enabled=false', () => {
    const addWin = vi.spyOn(window, 'addEventListener')
    const { unmount } = renderHook(() => useConnectionRecovery(vi.fn(), false))
    expect(addWin).not.toHaveBeenCalledWith('online', expect.any(Function))
    unmount()
  })

  it('removes listeners on unmount', () => {
    const removeWin = vi.spyOn(window, 'removeEventListener')
    const removeDoc = vi.spyOn(document, 'removeEventListener')
    const { unmount } = renderHook(() => useConnectionRecovery(vi.fn(), true))
    unmount()
    expect(removeWin).toHaveBeenCalledWith('online', expect.any(Function))
    expect(removeDoc).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
  })

  it('fires onRecover on window online event', () => {
    const onRecover = vi.fn()
    const { unmount } = renderHook(() => useConnectionRecovery(onRecover, true))
    act(() => { window.dispatchEvent(new Event('online')) })
    expect(onRecover).toHaveBeenCalledTimes(1)
    unmount()
  })

  it('fires onRecover on visibilitychange when document is visible', () => {
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })
    const onRecover = vi.fn()
    const { unmount } = renderHook(() => useConnectionRecovery(onRecover, true))
    act(() => { document.dispatchEvent(new Event('visibilitychange')) })
    expect(onRecover).toHaveBeenCalledTimes(1)
    unmount()
  })

  it('does not fire onRecover on visibilitychange when document is hidden', () => {
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true })
    const onRecover = vi.fn()
    const { unmount } = renderHook(() => useConnectionRecovery(onRecover, true))
    act(() => { document.dispatchEvent(new Event('visibilitychange')) })
    expect(onRecover).not.toHaveBeenCalled()
    unmount()
  })

  it('uses latest onRecover via stable ref — no re-registration required', () => {
    const fn1 = vi.fn()
    const fn2 = vi.fn()
    const { rerender, unmount } = renderHook(
      ({ fn }: { fn: () => void }) => useConnectionRecovery(fn, true),
      { initialProps: { fn: fn1 } }
    )
    rerender({ fn: fn2 })
    act(() => { window.dispatchEvent(new Event('online')) })
    expect(fn1).not.toHaveBeenCalled()
    expect(fn2).toHaveBeenCalledTimes(1)
    unmount()
  })
})
