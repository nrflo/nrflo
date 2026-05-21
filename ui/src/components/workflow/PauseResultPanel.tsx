import { PauseCircle } from 'lucide-react'
import type { PauseResult } from '@/types/workflow'

interface PauseResultPanelProps {
  result: PauseResult | undefined
}

export function PauseResultPanel({ result }: PauseResultPanelProps) {
  if (!result) return null

  const evt = result.event
  const eventOk = !evt || evt.status === 'ok'

  return (
    <div className={
      eventOk
        ? 'flex items-start gap-3 rounded-lg border border-yellow-200 bg-yellow-50 px-4 py-3 text-sm dark:border-yellow-800 dark:bg-yellow-950/30'
        : 'flex items-start gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm dark:border-red-800 dark:bg-red-950/30'
    }>
      <PauseCircle className="h-4 w-4 mt-0.5 shrink-0 text-yellow-700 dark:text-yellow-400" />
      <div className="space-y-1 min-w-0">
        <div className="font-medium text-foreground">
          Paused after layer {result.paused_after_layer} → resume at layer {result.resume_layer}
        </div>
        {evt && (
          <>
            <div className="text-muted-foreground">
              Hook: {evt.kind === 'command' ? evt.target : `script:${evt.target}`} · Exit {evt.exit_code} · {evt.status}
            </div>
            {evt.output_tail && (
              <pre className="mt-1 text-xs text-foreground whitespace-pre-wrap font-mono">{evt.output_tail}</pre>
            )}
          </>
        )}
      </div>
    </div>
  )
}
