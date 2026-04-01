import { cn } from '@/lib/utils'

function dotColor(status: string): string {
  switch (status) {
    case 'open':
      return 'bg-blue-500'
    case 'in_progress':
    case 'running':
      return 'bg-yellow-500 animate-pulse'
    case 'closed':
    case 'completed':
      return 'bg-green-500'
    case 'failed':
    case 'error':
      return 'bg-red-500'
    case 'pending':
      return 'bg-gray-400'
    case 'canceled':
      return 'bg-orange-500'
    case 'skipped':
      return 'bg-gray-400'
    default:
      return 'bg-gray-400'
  }
}

export function StatusCell({ status, className }: { status: string; className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-1.5', className)}>
      <span className={cn('h-1.5 w-1.5 rounded-full', dotColor(status))} />
      {status}
    </span>
  )
}
