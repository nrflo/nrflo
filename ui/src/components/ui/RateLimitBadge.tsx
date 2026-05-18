import { useTickingClock } from '@/hooks/useElapsedTime'
import { Badge } from '@/components/ui/Badge'
import { formatRateLimitCountdown } from '@/lib/rateLimit'

interface RateLimitBadgeProps {
  untilTs: string
}

export function RateLimitBadge({ untilTs }: RateLimitBadgeProps) {
  useTickingClock(true)

  const countdown = formatRateLimitCountdown(new Date(untilTs), new Date())
  if (!countdown) return null

  return (
    <Badge className="bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-200">
      Rate-limited, retrying in {countdown}
    </Badge>
  )
}
