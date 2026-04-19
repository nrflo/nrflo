/**
 * Unit tests for the performFitView helper — the single shared invoker used by
 * the Fit View button, the 15s auto-center interval, and the FitViewOnChange
 * effects. Guards the ticket invariant: all three paths must produce identical
 * fitView args (zoom/center) by routing through the same deferred call.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { FIT_VIEW_OPTIONS, performFitView } from './fitViewOptions'

describe('performFitView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('defers the fitView call via requestAnimationFrame (not synchronous)', () => {
    const fitView = vi.fn()

    performFitView(fitView)

    // rAF is polyfilled to setTimeout(16ms) in jsdom; under fake timers the
    // call must not have fired yet.
    expect(fitView).not.toHaveBeenCalled()

    vi.advanceTimersByTime(20)
    expect(fitView).toHaveBeenCalledTimes(1)
  })

  it('passes FIT_VIEW_OPTIONS verbatim ({ padding: 0.3 }) and does not mutate it', () => {
    const fitView = vi.fn()
    const snapshot = { ...FIT_VIEW_OPTIONS }

    performFitView(fitView)
    vi.advanceTimersByTime(20)

    expect(fitView).toHaveBeenCalledWith({ padding: 0.3 })
    // Source-of-truth options object is unchanged after the call.
    expect(FIT_VIEW_OPTIONS).toEqual(snapshot)
  })

  it('produces identical args across repeated invocations (button vs. interval parity)', () => {
    const fitView = vi.fn()

    // Simulate the button click path.
    performFitView(fitView)
    vi.advanceTimersByTime(20)
    const buttonArgs = fitView.mock.calls[0]

    // Simulate the 15s auto-center tick path.
    performFitView(fitView)
    vi.advanceTimersByTime(20)
    const intervalArgs = fitView.mock.calls[1]

    expect(buttonArgs).toEqual(intervalArgs)
  })
})
