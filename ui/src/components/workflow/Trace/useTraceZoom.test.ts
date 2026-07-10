import { describe, it, expect } from 'vitest'
import { clampZoom, TRACE_ZOOM_MIN, TRACE_ZOOM_MAX } from './useTraceZoom'

describe('clampZoom', () => {
  it('clamps into [min, max] and rejects non-finite values', () => {
    expect(clampZoom(0.5)).toBe(TRACE_ZOOM_MIN)
    expect(clampZoom(1)).toBe(1)
    expect(clampZoom(4)).toBe(4)
    expect(clampZoom(1000)).toBe(TRACE_ZOOM_MAX)
    expect(clampZoom(NaN)).toBe(TRACE_ZOOM_MIN)
    expect(clampZoom(Infinity)).toBe(TRACE_ZOOM_MIN)
  })
})
