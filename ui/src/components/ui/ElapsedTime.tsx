import { useTickingClock } from '@/hooks/useElapsedTime'
import { formatElapsedTime } from '@/lib/utils'

interface ElapsedTimeProps {
  start?: string | Date
  end?: string | Date
  running?: boolean
  fallback?: string
}

/**
 * Leaf elapsed-time display: subscribes to the shared 1s ticker so a second
 * tick re-renders only this text node, not the hosting card/panel.
 */
export function ElapsedTime({ start, end, running = false, fallback = '0s' }: ElapsedTimeProps) {
  useTickingClock(running && !!start && !end)
  if (!start) return <>{fallback}</>
  return <>{formatElapsedTime(start, end)}</>
}
