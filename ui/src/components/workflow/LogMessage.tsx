import { cn } from '@/lib/utils'

interface LogMessageProps {
  message: string
  variant?: 'compact' | 'full'
  className?: string
}

export function LogMessage({ message, variant = 'compact', className }: LogMessageProps) {
  if (variant === 'compact') {
    return (
      <div
        className={cn(
          'px-2 py-1 rounded-md border bg-muted/30',
          'font-mono text-xs text-foreground/90 truncate',
          className,
        )}
        title={message}
      >
        {message}
      </div>
    )
  }

  return (
    <div
      className={cn(
        'p-3 rounded-lg border bg-muted/30',
        'font-mono text-sm whitespace-pre-wrap break-words',
        'text-foreground/90',
        className,
      )}
    >
      {message}
    </div>
  )
}
