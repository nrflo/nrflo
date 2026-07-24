import type { QueryClient } from '@tanstack/react-query'

// Leading+trailing throttle for WS-driven query invalidations. A burst of
// events (12 agents writing findings, context updates, lifecycle churn)
// otherwise refetches the multi-MB workflow endpoints once per event; this
// caps each query key at ~1 refetch per WINDOW_MS while still firing
// immediately on the first event and once more after the burst settles.
// State is keyed per QueryClient so tests with fresh clients never share
// throttle windows.
const WINDOW_MS = 1000

interface ThrottleEntry {
  pending: boolean
}

const throttleState = new WeakMap<QueryClient, Map<string, ThrottleEntry>>()

export function throttledInvalidate(qc: QueryClient, queryKey: readonly unknown[]): void {
  let keys = throttleState.get(qc)
  if (!keys) {
    keys = new Map()
    throttleState.set(qc, keys)
  }

  const key = JSON.stringify(queryKey)
  const entry = keys.get(key)
  if (entry) {
    entry.pending = true
    return
  }

  qc.invalidateQueries({ queryKey })
  const newEntry: ThrottleEntry = { pending: false }
  keys.set(key, newEntry)

  const arm = () => {
    setTimeout(() => {
      if (newEntry.pending) {
        newEntry.pending = false
        qc.invalidateQueries({ queryKey })
        arm()
      } else {
        keys.delete(key)
      }
    }, WINDOW_MS)
  }
  arm()
}
