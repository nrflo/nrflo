import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'

export const TRACE_ZOOM_MIN = 1
export const TRACE_ZOOM_MAX = 32
const WHEEL_STEP = 1.2
export const BUTTON_STEP = 1.5

export function clampZoom(z: number): number {
  if (!Number.isFinite(z)) return TRACE_ZOOM_MIN
  return Math.min(TRACE_ZOOM_MAX, Math.max(TRACE_ZOOM_MIN, z))
}

/**
 * Zoom state for the trace timeline. Zooming only widens the inner plot (all
 * positions are percentages), so panning is the scroller's native horizontal
 * scroll. Ctrl/Cmd+wheel zooms anchored at the cursor; the anchored content
 * point is kept stable by adjusting scrollLeft right after the width change.
 */
export function useTraceZoom() {
  const scrollerRef = useRef<HTMLDivElement | null>(null)
  const [zoom, setZoom] = useState(TRACE_ZOOM_MIN)
  const pendingScrollLeft = useRef<number | null>(null)

  const zoomTo = useCallback(
    (next: number, anchorClientX?: number) => {
      const clamped = clampZoom(next)
      const scroller = scrollerRef.current
      if (scroller && clamped !== zoom) {
        const rect = scroller.getBoundingClientRect()
        const anchorX = anchorClientX != null ? anchorClientX - rect.left : rect.width / 2
        const contentX = scroller.scrollLeft + anchorX
        pendingScrollLeft.current = (contentX * clamped) / zoom - anchorX
      }
      setZoom(clamped)
    },
    [zoom]
  )

  useLayoutEffect(() => {
    const scroller = scrollerRef.current
    if (scroller && pendingScrollLeft.current != null) {
      scroller.scrollLeft = pendingScrollLeft.current
      pendingScrollLeft.current = null
    }
  }, [zoom])

  const zoomBy = useCallback(
    (factor: number, anchorClientX?: number) => zoomTo(zoom * factor, anchorClientX),
    [zoom, zoomTo]
  )

  // Non-passive wheel listener (React's synthetic wheel cannot preventDefault);
  // the ref is refreshed in an effect so the listener stays registered once.
  const zoomByRef = useRef(zoomBy)
  useEffect(() => {
    zoomByRef.current = zoomBy
  }, [zoomBy])
  useEffect(() => {
    const scroller = scrollerRef.current
    if (!scroller) return
    const onWheel = (e: WheelEvent) => {
      if (!e.ctrlKey && !e.metaKey) return
      e.preventDefault()
      zoomByRef.current(e.deltaY < 0 ? WHEEL_STEP : 1 / WHEEL_STEP, e.clientX)
    }
    scroller.addEventListener('wheel', onWheel, { passive: false })
    return () => scroller.removeEventListener('wheel', onWheel)
  }, [])

  return {
    zoom,
    scrollerRef,
    zoomIn: () => zoomBy(BUTTON_STEP),
    zoomOut: () => zoomBy(1 / BUTTON_STEP),
    resetZoom: () => zoomTo(TRACE_ZOOM_MIN),
  }
}
