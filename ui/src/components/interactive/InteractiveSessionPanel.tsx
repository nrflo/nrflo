import { lazy, Suspense } from 'react'
import { Spinner } from '@/components/ui/Spinner'

const XTerminal = lazy(() =>
  import('@/components/workflow/XTerminal').then((m) => ({ default: m.XTerminal }))
)

interface InteractiveSessionPanelProps {
  sessionId: string
  isActive: boolean
  onExit: () => void
}

export function InteractiveSessionPanel({ sessionId, isActive, onExit }: InteractiveSessionPanelProps) {
  return (
    <div className={isActive ? 'flex-1 min-h-0 overflow-hidden' : 'hidden'}>
      <Suspense
        fallback={
          <div className="flex items-center justify-center h-full">
            <Spinner size="lg" />
          </div>
        }
      >
        <XTerminal sessionId={sessionId} onExit={onExit} />
      </Suspense>
    </div>
  )
}
