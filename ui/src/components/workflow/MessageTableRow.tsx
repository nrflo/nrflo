import { memo } from 'react'
import { TableRow, TableCell } from '@/components/ui/Table'
import { parseToolName, ToolBadge } from './LogMessage'
import { cn } from '@/lib/utils'
import type { MessageWithTime } from '@/types/workflow'
import { formatTime } from './MessageTable'

interface MessageTableRowProps {
  msg: MessageWithTime
}

function MessageTableRowInner({ msg }: MessageTableRowProps) {
  const { toolName, rest } = parseToolName(msg.content)
  const isUserInput = msg.category === 'user_input'
  const isError = msg.category === 'error'
  const isResult = msg.category === 'result'
  const isValidation = msg.category === 'validation'
  const isThinking = msg.category === 'thinking'
  return (
    <TableRow
      className={cn(
        toolName === 'rate_limit' && "bg-orange-50 dark:bg-orange-950/20",
        isUserInput && "border-l-4 border-l-primary bg-primary/5 dark:bg-primary/10",
        isError && "border-l-4 border-l-destructive bg-destructive/5 dark:bg-destructive/10",
        isResult && "border-l-4 border-l-emerald-500 bg-emerald-50/50 dark:bg-emerald-950/20",
        isValidation && "border-l-4 border-l-destructive bg-destructive/5 dark:bg-destructive/10",
      )}
      data-testid="message-row"
    >
      <TableCell className="py-1 px-2 w-[80px] text-muted-foreground whitespace-nowrap overflow-hidden text-ellipsis">
        {formatTime(msg.created_at)}
      </TableCell>
      <TableCell className="py-1 px-2 w-[112px] overflow-hidden">
        {isUserInput ? (
          <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold mr-1.5 shrink-0 bg-primary/10 text-primary border border-primary/40">
            User
          </span>
        ) : isError ? (
          <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold mr-1.5 shrink-0 bg-destructive/10 text-destructive border border-destructive/40">
            Error
          </span>
        ) : isResult ? (
          <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold mr-1.5 shrink-0 bg-emerald-100 text-emerald-700 border border-emerald-300 dark:bg-emerald-900/30 dark:text-emerald-400 dark:border-emerald-700">
            Result
          </span>
        ) : isValidation ? (
          <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold mr-1.5 shrink-0 bg-destructive/10 text-destructive border border-destructive/40">
            Validation
          </span>
        ) : isThinking ? (
          <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold mr-1.5 shrink-0 bg-muted text-muted-foreground border border-border">
            Thinking
          </span>
        ) : (
          toolName && <ToolBadge name={toolName} compact />
        )}
      </TableCell>
      <TableCell className={cn('py-1 whitespace-pre-wrap break-words align-top', isThinking ? 'text-muted-foreground italic' : 'text-foreground/90')}>
        {rest}
        {msg.payload && (
          <details className="mt-1">
            <summary className="text-[10px] text-muted-foreground cursor-pointer select-none">payload</summary>
            <pre className="text-[10px] mt-1 p-1 bg-muted rounded overflow-auto whitespace-pre-wrap break-words not-prose">
              {JSON.stringify(msg.payload, null, 2)}
            </pre>
          </details>
        )}
      </TableCell>
    </TableRow>
  )
}

// Messages are immutable once emitted; content+category+timestamp identify a
// row even though the fetch layer rebuilds every message object per refetch.
export const MessageTableRow = memo(
  MessageTableRowInner,
  (prev, next) =>
    prev.msg.content === next.msg.content &&
    prev.msg.category === next.msg.category &&
    prev.msg.created_at === next.msg.created_at
)
