export function formatRateLimitCountdown(target: Date, now: Date): string {
  const diffMs = target.getTime() - now.getTime()
  if (diffMs <= 0) return ''

  const totalSec = Math.ceil(diffMs / 1000)

  if (totalSec < 60) {
    return `${totalSec}s`
  }

  if (totalSec < 3600) {
    const m = Math.floor(totalSec / 60)
    const s = totalSec % 60
    return `${m}m ${s}s`
  }

  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  return `${h}h ${m}m`
}
