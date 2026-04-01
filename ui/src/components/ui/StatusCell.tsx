import { Circle, Timer, CheckCircle2, XCircle, Ban, MinusCircle } from 'lucide-react'
import { cn } from '@/lib/utils'

function StatusIcon({ status }: { status: string }) {
  const base = 'h-4 w-4'
  switch (status) {
    case 'open':
      return <Circle className={cn(base, 'text-blue-500')} />
    case 'in_progress':
    case 'running':
      return <Timer className={cn(base, 'text-yellow-500 animate-pulse')} />
    case 'closed':
    case 'completed':
      return <CheckCircle2 className={cn(base, 'text-green-500')} />
    case 'failed':
    case 'error':
      return <XCircle className={cn(base, 'text-red-500')} />
    case 'pending':
      return <Circle className={cn(base, 'text-gray-400')} />
    case 'canceled':
      return <Ban className={cn(base, 'text-orange-500')} />
    case 'skipped':
      return <MinusCircle className={cn(base, 'text-gray-400')} />
    default:
      return <Circle className={cn(base, 'text-gray-400')} />
  }
}

export function StatusCell({ status, className }: { status: string; className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-1.5', className)}>
      <StatusIcon status={status} />
      {status}
    </span>
  )
}
