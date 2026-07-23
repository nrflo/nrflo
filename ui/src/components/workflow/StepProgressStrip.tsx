import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import { Tooltip } from '@/components/ui/Tooltip'
import { useStepCursors } from '@/hooks/useStepCursors'
import { progressLabel, stepDisplayState, type StepDisplayState } from '@/lib/stepProgress'
import type { StepProgressStep } from '@/types/stepwise'

interface StepProgressStripProps {
  instanceId?: string
  nodeId?: string
}

const PIP_COLOR: Record<StepDisplayState, string> = {
  pending: 'bg-gray-300 dark:bg-gray-600',
  active: 'bg-amber-400 dark:bg-amber-500',
  done: 'bg-green-500 dark:bg-green-500',
  'rejected-retrying': 'bg-red-500 dark:bg-red-500',
  rotated: 'bg-purple-500 dark:bg-purple-400',
}

function tooltipLine(step: StepProgressStep, index: number): string {
  const state = stepDisplayState(step)
  const when = step.completed_at ? ` @ ${new Date(step.completed_at).toLocaleString()}` : ''
  return `${index + 1}. ${step.title} — ${state}${when}`
}

export function StepProgressStrip({ instanceId, nodeId }: StepProgressStripProps) {
  const { data } = useStepCursors(instanceId)
  const cursor = nodeId ? data?.cursors[nodeId] : undefined
  if (!cursor) return null

  const tooltipText = (
    <div className="flex flex-col gap-0.5">
      {cursor.steps.map((step, i) => (
        <span key={step.step_id || i}>{tooltipLine(step, i)}</span>
      ))}
    </div>
  )

  return (
    <Tooltip text={tooltipText} placement="bottom">
      <div className="flex items-center gap-1.5 mt-3">
        <Badge variant="secondary" className="text-xs px-2 py-0.5">
          {progressLabel(cursor)}
        </Badge>
        <div className="flex items-center gap-1">
          {cursor.steps.map((step, i) => (
            <span
              key={step.step_id || i}
              className={cn('h-1.5 w-1.5 rounded-full', PIP_COLOR[stepDisplayState(step)])}
            />
          ))}
        </div>
      </div>
    </Tooltip>
  )
}
