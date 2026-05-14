import { useState, useRef, useCallback } from 'react'
import { createPortal } from 'react-dom'
import { useClaudeLimits } from '@/hooks/useClaudeLimits'

const STALE_THRESHOLD_MS = 25 * 60 * 1000 // 25 minutes

function thresholdClass(pct: number): string {
  if (pct >= 85) return 'bg-red-500/20 text-red-600 border-red-400 dark:text-red-400'
  if (pct >= 60) return 'bg-yellow-500/20 text-yellow-600 border-yellow-400 dark:text-yellow-400'
  return 'bg-green-500/20 text-green-600 border-green-400 dark:text-green-400'
}

function grayClass(): string {
  return 'bg-muted text-muted-foreground border-border'
}

function formatRelative(isoDate: string): string {
  const ms = Date.parse(isoDate) - Date.now()
  if (ms <= 0) return 'now'
  const totalSec = Math.floor(ms / 1000)
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

function formatAgo(isoDate: string): string {
  const ms = Date.now() - Date.parse(isoDate)
  if (ms < 0) return 'just now'
  const totalSec = Math.floor(ms / 1000)
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  if (h > 0) return `${h}h ${m}m ago`
  if (m > 0) return `${m}m ago`
  return 'just now'
}

function isPast(isoDate: string | null): boolean {
  if (!isoDate) return false
  return Date.now() > Date.parse(isoDate)
}

export function ClaudeLimitsBadge() {
  const { data } = useClaudeLimits()
  const [visible, setVisible] = useState(false)
  const [coords, setCoords] = useState({ top: 0, left: 0 })
  const triggerRef = useRef<HTMLDivElement>(null)
  const hideTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clearHideTimeout = useCallback(() => {
    if (hideTimeout.current) {
      clearTimeout(hideTimeout.current)
      hideTimeout.current = null
    }
  }, [])

  const scheduleHide = useCallback(() => {
    clearHideTimeout()
    hideTimeout.current = setTimeout(() => setVisible(false), 150)
  }, [clearHideTimeout])

  const showPopover = useCallback(() => {
    clearHideTimeout()
    if (!triggerRef.current) return
    const rect = triggerRef.current.getBoundingClientRect()
    setCoords({
      top: rect.bottom + 6,
      left: rect.left + rect.width / 2,
    })
    setVisible(true)
  }, [clearHideTimeout])

  if (!data || (data.five_hour_used_pct === null && data.seven_day_used_pct === null)) return null

  const stale = data.updated_at ? Date.now() - Date.parse(data.updated_at) > STALE_THRESHOLD_MS : true
  const baseClass = 'inline-flex items-center rounded border px-1.5 py-0.5 text-xs font-medium'

  const fivePct = data.five_hour_used_pct
  const weekPct = data.seven_day_used_pct
  const fivePastReset = isPast(data.five_hour_resets_at)
  const weekPastReset = isPast(data.seven_day_resets_at)
  const fiveClass = (stale || fivePastReset) ? grayClass() : fivePct !== null ? thresholdClass(fivePct) : grayClass()
  const weekClass = (stale || weekPastReset) ? grayClass() : weekPct !== null ? thresholdClass(weekPct) : grayClass()

  return (
    <>
      <div
        ref={triggerRef}
        onMouseEnter={showPopover}
        onMouseLeave={scheduleHide}
        className="inline-flex items-center gap-1 cursor-default"
      >
        {fivePct !== null && (
          <span
            className={`${baseClass} ${fiveClass}`}
            title={fivePastReset ? 'Reset window elapsed — value is the last reading before reset' : undefined}
          >
            5h: {Math.round(fivePct)}%
          </span>
        )}
        {weekPct !== null && (
          <span
            className={`${baseClass} ${weekClass}`}
            title={weekPastReset ? 'Reset window elapsed — value is the last reading before reset' : undefined}
          >
            wk: {Math.round(weekPct)}%
          </span>
        )}
      </div>
      {visible &&
        createPortal(
          <div
            onMouseEnter={clearHideTimeout}
            onMouseLeave={scheduleHide}
            className="fixed z-[100] -translate-x-1/2 min-w-56 rounded-lg bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 shadow-lg p-3 text-xs"
            style={{ top: coords.top, left: coords.left }}
          >
            <div className="font-semibold mb-2">Claude Usage Limits</div>
            {fivePct !== null && (
              <div className="mb-1">
                <span className="text-gray-300 dark:text-gray-600">5-hour window: </span>
                <span className="font-medium">{Math.round(fivePct)}%</span>
                {data.five_hour_resets_at && (
                  <span className="ml-1 text-gray-400 dark:text-gray-500">
                    {isPast(data.five_hour_resets_at) ? '(reset overdue)' : `(resets in ${formatRelative(data.five_hour_resets_at)})`}
                  </span>
                )}
              </div>
            )}
            {weekPct !== null && (
              <div className="mb-1">
                <span className="text-gray-300 dark:text-gray-600">7-day window: </span>
                <span className="font-medium">{Math.round(weekPct)}%</span>
                {data.seven_day_resets_at && (
                  <span className="ml-1 text-gray-400 dark:text-gray-500">
                    {isPast(data.seven_day_resets_at) ? '(reset overdue)' : `(resets in ${formatRelative(data.seven_day_resets_at)})`}
                  </span>
                )}
              </div>
            )}
            {data.updated_at && (
              <div className="mt-2 text-gray-400 dark:text-gray-500">
                Updated {formatAgo(data.updated_at)}
              </div>
            )}
            {stale && (
              <div className="mt-2 text-yellow-400 dark:text-yellow-600">
                Stale — enable "Sync Claude limits" in Settings → Providers → Claude
              </div>
            )}
          </div>,
          document.body,
        )}
    </>
  )
}
