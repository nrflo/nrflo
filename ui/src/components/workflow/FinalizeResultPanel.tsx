import { CheckCircle, XCircle } from 'lucide-react'
import type { FinalizeResult } from '@/types/workflow'

interface FinalizeResultPanelProps {
  result: FinalizeResult | undefined
}

export function FinalizeResultPanel({ result }: FinalizeResultPanelProps) {
  if (!result) return null

  const isOk = result.status === 'ok'

  return (
    <div
      className={
        isOk
          ? 'flex items-start gap-3 rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm dark:border-green-800 dark:bg-green-950/30'
          : 'flex items-start gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm dark:border-red-800 dark:bg-red-950/30'
      }
    >
      {isOk ? (
        <CheckCircle className="h-4 w-4 mt-0.5 shrink-0 text-green-700 dark:text-green-400" />
      ) : (
        <XCircle className="h-4 w-4 mt-0.5 shrink-0 text-red-700 dark:text-red-400" />
      )}
      <div className="space-y-1 min-w-0">
        <div className="font-medium text-foreground">
          Finalize ({result.slot}) — {result.kind === 'command' ? result.target : `script:${result.target}`}
        </div>
        <div className="text-muted-foreground">
          Exit {result.exit_code} · {result.status}
        </div>
        {result.output_tail && (
          <pre className="mt-1 text-xs text-foreground whitespace-pre-wrap font-mono">{result.output_tail}</pre>
        )}
      </div>
    </div>
  )
}
