import { useTickingClock } from '@/hooks/useElapsedTime'
import { formatDateTime } from '@/lib/utils'
import { formatCountdown } from '@/lib/cron'
import { cn } from '@/lib/utils'

interface CronCountdownProps {
  nextRunAt?: string
}

export function CronCountdown({ nextRunAt }: CronCountdownProps) {
  useTickingClock(true)

  if (!nextRunAt) return <span>—</span>

  const target = new Date(nextRunAt)
  const now = new Date()
  const countdown = formatCountdown(target, now)
  const overdue = countdown === 'overdue'

  return (
    <div>
      <p className={cn('text-sm', overdue ? 'text-destructive' : 'text-muted-foreground')}>
        {countdown}
      </p>
      <p className="text-xs text-muted-foreground">{formatDateTime(nextRunAt)}</p>
    </div>
  )
}
