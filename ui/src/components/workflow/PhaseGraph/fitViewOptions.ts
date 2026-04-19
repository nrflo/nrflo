export const FIT_VIEW_OPTIONS = { padding: 0.3 }

type FitViewFn = (options?: typeof FIT_VIEW_OPTIONS) => unknown

/**
 * Defers fitView via requestAnimationFrame so React Flow reads the latest node
 * measurements (live elapsed-time labels, async ELK layout, internal width/height
 * store) before computing zoom. Without this, the manual Fit View button and the
 * 15s auto-center interval produced different viewports because each fired at a
 * different point in the measurement/commit cycle.
 */
export function performFitView(fitViewFn: FitViewFn): void {
  requestAnimationFrame(() => fitViewFn(FIT_VIEW_OPTIONS))
}
