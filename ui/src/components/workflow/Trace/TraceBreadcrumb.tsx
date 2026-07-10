import { ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'

export interface TraceCrumb {
  instanceId: string
  workflow: string
}

/** Root → child trace navigation stack. */
export function TraceBreadcrumb({
  stack,
  onNavigate,
}: {
  stack: TraceCrumb[]
  onNavigate: (index: number) => void
}) {
  if (stack.length <= 1) return null
  return (
    <div className="flex items-center gap-1 text-xs" data-testid="trace-breadcrumb">
      {stack.map((crumb, i) => (
        <span key={crumb.instanceId} className="flex items-center gap-1">
          {i > 0 && <ChevronRight className="h-3 w-3 text-muted-foreground" />}
          <button
            onClick={() => onNavigate(i)}
            disabled={i === stack.length - 1}
            className={cn(
              i === stack.length - 1
                ? 'text-foreground font-medium'
                : 'text-muted-foreground hover:text-primary'
            )}
          >
            {crumb.workflow} <span className="opacity-60">#{crumb.instanceId.slice(0, 8)}</span>
          </button>
        </span>
      ))}
    </div>
  )
}
