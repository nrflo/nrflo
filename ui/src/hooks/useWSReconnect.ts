import { useEffect, useRef } from 'react'

export const MAX_RECONNECT_DELAY = 30_000

export function computeReconnectDelay(attempt: number, base = 3000, max = MAX_RECONNECT_DELAY): number {
  return Math.min(base * 2 ** attempt, max) + Math.round(Math.random() * 1000)
}

// Revive a dead socket without a page refresh: fire onRecover when the browser
// regains connectivity (window 'online') or the tab becomes visible again.
export function useConnectionRecovery(onRecover: () => void, enabled: boolean): void {
  const onRecoverRef = useRef(onRecover)
  onRecoverRef.current = onRecover

  useEffect(() => {
    if (!enabled) return

    const handleOnline = () => onRecoverRef.current()
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') onRecoverRef.current()
    }

    window.addEventListener('online', handleOnline)
    document.addEventListener('visibilitychange', handleVisibilityChange)

    return () => {
      window.removeEventListener('online', handleOnline)
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [enabled])
}
