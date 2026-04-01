import { CheckCircle2, XCircle, MinusCircle } from 'lucide-react'
import { cn } from '@/lib/utils'

interface ResultIconProps {
  result: string
  className?: string
}

export function ResultIcon({ result, className }: ResultIconProps) {
  const iconClass = cn('h-4 w-4', className)

  switch (result) {
    case 'pass':
      return (
        <span className="inline-flex items-center gap-1">
          <CheckCircle2 className={cn(iconClass, 'text-green-500')} />
          <span>pass</span>
        </span>
      )
    case 'fail':
      return (
        <span className="inline-flex items-center gap-1">
          <XCircle className={cn(iconClass, 'text-red-500')} />
          <span>fail</span>
        </span>
      )
    case 'skip':
    case 'skipped':
      return (
        <span className="inline-flex items-center gap-1">
          <MinusCircle className={cn(iconClass, 'text-gray-400')} />
          <span>skipped</span>
        </span>
      )
    default:
      return (
        <span className="inline-flex items-center gap-1">
          <MinusCircle className={cn(iconClass, 'text-gray-400')} />
          <span>{result}</span>
        </span>
      )
  }
}
