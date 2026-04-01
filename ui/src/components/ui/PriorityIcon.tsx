import { ChevronsUp, ArrowUp, ArrowRight, ArrowDown } from 'lucide-react'
import { cn, priorityLabel } from '@/lib/utils'

interface PriorityIconProps {
  priority: number
  className?: string
  showLabel?: boolean
}

export function PriorityIcon({ priority, className, showLabel = true }: PriorityIconProps) {
  const iconClass = cn('h-4 w-4', className)

  const icon = (() => {
    switch (priority) {
      case 1:
        return <ChevronsUp className={cn(iconClass, 'text-red-500')} />
      case 2:
        return <ArrowUp className={cn(iconClass, 'text-orange-500')} />
      case 3:
        return <ArrowRight className={cn(iconClass, 'text-yellow-500')} />
      case 4:
        return <ArrowDown className={cn(iconClass, 'text-blue-500')} />
      default:
        return <ArrowRight className={cn(iconClass, 'text-gray-400')} />
    }
  })()

  return (
    <span className="inline-flex items-center gap-1">
      {icon}
      {showLabel && <span>{priorityLabel(priority)}</span>}
    </span>
  )
}
