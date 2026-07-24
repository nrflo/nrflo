import { cn } from '@/lib/utils'
import { Tooltip } from '@/components/ui/Tooltip'
import type { TimeBuckets } from './types'

type BucketKey = keyof TimeBuckets

const BUCKET_ORDER: BucketKey[] = ['thinking_sec', 'tool_arg_sec', 'text_sec', 'tool_wait_sec']

const BUCKET_LABEL: Record<BucketKey, string> = {
  thinking_sec: 'Thinking',
  tool_arg_sec: 'Tool args',
  text_sec: 'Text',
  tool_wait_sec: 'Tool wait',
}

const BUCKET_COLOR: Record<BucketKey, string> = {
  thinking_sec: 'bg-gray-400 dark:bg-gray-500',
  tool_arg_sec: 'bg-sky-500 dark:bg-sky-400',
  text_sec: 'bg-green-500 dark:bg-green-400',
  tool_wait_sec: 'bg-orange-400 dark:bg-orange-500',
}

function fmtSec(sec: number): string {
  return `${sec.toFixed(1)}s`
}

/** Small stacked bar showing the share of wall-clock time spent in each phase bucket. Renders nothing without data. */
export function TimeBreakdownBar({ buckets }: { buckets?: TimeBuckets }) {
  if (!buckets) return null

  const total = BUCKET_ORDER.reduce((sum, key) => sum + (buckets[key] ?? 0), 0)
  if (total <= 0) return null

  const tooltip = (
    <div className="space-y-0.5">
      {BUCKET_ORDER.map((key) => {
        const value = buckets[key] ?? 0
        const pct = (value / total) * 100
        return (
          <div key={key}>
            {BUCKET_LABEL[key]}: {fmtSec(value)} ({pct.toFixed(0)}%)
          </div>
        )
      })}
    </div>
  )

  return (
    <Tooltip text={tooltip}>
      <div data-testid="trace-lane-timebar" className="flex w-full h-1.5 rounded-sm overflow-hidden mt-1">
        {BUCKET_ORDER.map((key) => {
          const value = buckets[key] ?? 0
          if (value <= 0) return null
          const pct = (value / total) * 100
          return (
            <div
              key={key}
              data-testid={`trace-lane-timebar-${key}`}
              className={cn('h-full', BUCKET_COLOR[key])}
              style={{ width: `${pct}%` }}
            />
          )
        })}
      </div>
    </Tooltip>
  )
}
