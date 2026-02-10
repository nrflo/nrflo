import { useState, useEffect } from 'react'
import { formatElapsedTime } from '@/lib/utils'

/**
 * Hook that returns a formatted elapsed time string, updating every second
 * when isRunning is true.
 */
export function useElapsedTime(
  startDate: string | Date | undefined,
  endDate?: string | Date,
  isRunning: boolean = false
): string {
  const [, setTick] = useState(0)

  useEffect(() => {
    if (!isRunning) return
    const interval = setInterval(() => setTick(t => t + 1), 1000)
    return () => clearInterval(interval)
  }, [isRunning])

  if (!startDate) return '0s'
  return formatElapsedTime(startDate, endDate)
}

/**
 * Hook that forces a re-render every second when active.
 * Useful for components that need to update multiple elapsed times.
 */
export function useTickingClock(active: boolean = true): void {
  const [, setTick] = useState(0)

  useEffect(() => {
    if (!active) return
    const interval = setInterval(() => setTick(t => t + 1), 1000)
    return () => clearInterval(interval)
  }, [active])
}
