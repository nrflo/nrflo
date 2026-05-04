import { CronExpressionParser } from 'cron-parser'

const NOW_WINDOW_MS = 500

export function formatCountdown(target: Date, now: Date): string {
  const diffMs = target.getTime() - now.getTime()

  if (Math.abs(diffMs) <= NOW_WINDOW_MS) return 'now'
  if (diffMs < 0) return 'overdue'

  const totalSec = Math.floor(diffMs / 1000)
  const days = Math.floor(totalSec / 86400)
  const hours = Math.floor((totalSec % 86400) / 3600)
  const mins = Math.floor((totalSec % 3600) / 60)
  const secs = totalSec % 60

  const parts: string[] = []
  if (days > 0) parts.push(`${days}d`)
  if (hours > 0) parts.push(`${hours}h`)
  if (mins > 0) parts.push(`${mins}m`)
  if (secs > 0) parts.push(`${secs}s`)

  return 'in ' + parts.slice(0, 2).join(' ')
}

export function computeNextRuns(expr: string, count: number, from?: Date): Date[] {
  try {
    const interval = CronExpressionParser.parse(expr, { currentDate: from ?? new Date() })
    const dates: Date[] = []
    for (let i = 0; i < count; i++) {
      dates.push(interval.next().toDate())
    }
    return dates
  } catch {
    return []
  }
}
