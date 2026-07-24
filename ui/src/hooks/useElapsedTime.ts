import { useSyncExternalStore } from 'react'
import { formatElapsedTime } from '@/lib/utils'

// One shared 1s ticker for every elapsed-time display: N subscribers share a
// single interval instead of each arming their own.
let now = Date.now()
const listeners = new Set<() => void>()
let interval: ReturnType<typeof setInterval> | null = null

function subscribe(cb: () => void): () => void {
  listeners.add(cb)
  if (!interval) {
    interval = setInterval(() => {
      now = Date.now()
      listeners.forEach((l) => l())
    }, 1000)
  }
  return () => {
    listeners.delete(cb)
    if (listeners.size === 0 && interval) {
      clearInterval(interval)
      interval = null
    }
  }
}

const subscribeNoop = () => () => {}
const getNow = () => now
const getZero = () => 0

/**
 * Hook that forces a re-render every second (off the shared ticker) when active.
 * Prefer the `ElapsedTime` leaf component so only the timestamp span re-renders.
 */
export function useTickingClock(active: boolean = true): void {
  useSyncExternalStore(active ? subscribe : subscribeNoop, active ? getNow : getZero)
}

/**
 * Hook that returns a formatted elapsed time string, updating every second
 * when isRunning is true.
 */
export function useElapsedTime(
  startDate: string | Date | undefined,
  endDate?: string | Date,
  isRunning: boolean = false
): string {
  useTickingClock(isRunning)
  if (!startDate) return '0s'
  return formatElapsedTime(startDate, endDate)
}
