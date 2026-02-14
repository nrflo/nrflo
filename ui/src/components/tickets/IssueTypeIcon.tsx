import { Bug, Lightbulb, CheckSquare, Layers } from 'lucide-react'
import { cn } from '@/lib/utils'

const sizeMap = {
  sm: 'h-4 w-4',
  md: 'h-5 w-5',
} as const

interface IssueTypeIconProps {
  type: string
  size?: keyof typeof sizeMap
}

export function IssueTypeIcon({ type, size = 'sm' }: IssueTypeIconProps) {
  const s = sizeMap[size]
  switch (type) {
    case 'bug':
      return <Bug className={cn(s, 'text-red-500')} />
    case 'feature':
      return <Lightbulb className={cn(s, 'text-purple-500')} />
    case 'task':
      return <CheckSquare className={cn(s, 'text-blue-500')} />
    case 'epic':
      return <Layers className={cn(s, 'text-green-500')} />
    default:
      return <CheckSquare className={cn(s, 'text-gray-500')} />
  }
}
