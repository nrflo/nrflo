import { MessageSquare } from 'lucide-react'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { parseToolName, ToolBadge } from './LogMessage'
import { cn } from '@/lib/utils'
import type { MessageCategory, MessageWithTime } from '@/types/workflow'

export const CATEGORY_TABS: { value: MessageCategory | 'all'; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'text', label: 'Text' },
  { value: 'tool', label: 'Tools' },
  { value: 'subagent', label: 'Sub-agents' },
  { value: 'skill', label: 'Skills' },
  { value: 'user_input', label: 'User input' },
  { value: 'error', label: 'Errors' },
  { value: 'result', label: 'Results' },
  { value: 'validation', label: 'Validation' },
  { value: 'thinking', label: 'Thinking' },
]

export function formatTime(dateStr: string): string {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  return d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
}

interface MessageTableProps {
  filteredMessages: MessageWithTime[]
  categoryFilter: MessageCategory | 'all'
  setCategoryFilter: (cat: MessageCategory | 'all') => void
  categoryCounts: Record<string, number>
  normalizedCount: number
}

export function MessageTable({ filteredMessages, categoryFilter, setCategoryFilter, categoryCounts, normalizedCount }: MessageTableProps) {
  return (
    <div>
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-2">
        <MessageSquare className="h-3 w-3" />
        <span>
          {categoryFilter !== 'all'
            ? `${filteredMessages.length} of ${normalizedCount} messages`
            : `${normalizedCount} messages`}
        </span>
      </div>
      <div className="flex items-center gap-0.5 mb-2 border-b border-border" role="tablist">
        {CATEGORY_TABS.map((tab) => (
          <button
            key={tab.value}
            role="tab"
            aria-selected={categoryFilter === tab.value}
            onClick={() => setCategoryFilter(tab.value)}
            className={cn(
              'px-2 py-1 text-xs font-medium transition-colors rounded-t',
              categoryFilter === tab.value
                ? 'border-b-2 border-primary text-foreground bg-muted'
                : 'text-muted-foreground hover:text-foreground hover:bg-muted/50',
            )}
          >
            {tab.label}
            <span className={cn(
              'ml-1 px-1 py-0.5 rounded text-[10px]',
              categoryFilter === tab.value
                ? 'bg-primary/10 text-primary'
                : 'bg-muted text-muted-foreground',
            )}>
              {categoryCounts[tab.value] ?? 0}
            </span>
          </button>
        ))}
      </div>
      <Table className="[&>table]:text-xs [&>table]:table-fixed" data-testid="message-table">
        <TableHeader>
          <TableRow data-testid="message-table-header">
            <TableHead className="w-[80px] px-2">Time</TableHead>
            <TableHead className="w-[112px] px-2">Tool</TableHead>
            <TableHead>Message</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {[...filteredMessages].reverse().map((msg, i) => {
            const { toolName, rest } = parseToolName(msg.content)
            const isUserInput = msg.category === 'user_input'
            const isError = msg.category === 'error'
            const isResult = msg.category === 'result'
            const isValidation = msg.category === 'validation'
            const isThinking = msg.category === 'thinking'
            return (
              <TableRow
                key={i}
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
          })}
        </TableBody>
      </Table>
    </div>
  )
}
