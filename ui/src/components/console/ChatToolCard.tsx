import { ToolBadge, parseToolName } from '@/components/workflow/LogMessage'
import { cn } from '@/lib/utils'
import type { ToolPair } from './chatStream'

function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.max(0, ms)}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

interface ChatToolCardProps {
  pair: ToolPair
}

// One card per invoke+result pair from pairToolMessages: ToolBadge, the
// invoke detail, a collapsible result, destructive styling when the result
// row is category='error', and a duration chip from payload.ended_at.
export function ChatToolCard({ pair }: ChatToolCardProps) {
  const { toolName, rest } = parseToolName(pair.invoke.content)
  const isError = pair.result?.category === 'error'
  const resultText = pair.result ? parseToolName(pair.result.content).rest : null

  return (
    <div
      className={cn(
        'rounded-md border border-border bg-muted/30 px-3 py-2 text-xs',
        isError && 'border-l-4 border-l-destructive bg-destructive/5'
      )}
      data-testid="chat-tool-card"
    >
      <div className="flex items-center gap-2 flex-wrap">
        {toolName && <ToolBadge name={toolName} />}
        {pair.running && <span className="text-muted-foreground italic">running…</span>}
        {pair.durationMs != null && (
          <span className="ml-auto inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-muted text-muted-foreground border border-border">
            {formatDuration(pair.durationMs)}
          </span>
        )}
      </div>
      {rest && <div className="mt-1 whitespace-pre-wrap break-words font-mono text-foreground/90">{rest}</div>}
      {(pair.input != null || pair.inputTruncated) && (
        <details className="mt-1">
          <summary className="cursor-pointer select-none text-[10px] text-muted-foreground">
            input
            {pair.inputTruncated && <span className="ml-1 italic">(truncated)</span>}
          </summary>
          <div className="mt-1 whitespace-pre-wrap break-words font-mono text-foreground/90">
            {pair.input != null ? JSON.stringify(pair.input, null, 2) : '(input too large to display)'}
          </div>
        </details>
      )}
      {pair.result && (
        <details className="mt-1">
          <summary className="cursor-pointer select-none text-[10px] text-muted-foreground">result</summary>
          <div
            className={cn(
              'mt-1 whitespace-pre-wrap break-words font-mono',
              isError ? 'text-destructive' : 'text-foreground/90'
            )}
          >
            {resultText}
          </div>
        </details>
      )}
    </div>
  )
}
