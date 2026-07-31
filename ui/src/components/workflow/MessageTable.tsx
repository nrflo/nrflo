import { useState } from 'react'
import { MessageSquare } from 'lucide-react'
import { Table, TableHeader, TableBody, TableRow, TableHead } from '@/components/ui/Table'
import { MessageTableRow } from './MessageTableRow'
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
  { value: 'task_notification', label: 'Tasks' },
  { value: 'system_notice', label: 'Notices' },
]

// Live transcripts run to thousands of rows; render the newest slice and let
// the user pull in older history explicitly.
const INITIAL_VISIBLE = 100
const VISIBLE_INCREMENT = 200

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
  const [visibleCount, setVisibleCount] = useState(INITIAL_VISIBLE)
  const total = filteredMessages.length
  const hiddenCount = Math.max(0, total - visibleCount)

  // Newest first; key rows by their position in the original (append-only)
  // order so an appended message doesn't re-key every existing row.
  const rows: { msg: MessageWithTime; originalIndex: number }[] = []
  for (let i = total - 1; i >= Math.max(0, total - visibleCount); i--) {
    rows.push({ msg: filteredMessages[i], originalIndex: i })
  }

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
      <div className="flex items-center flex-wrap gap-0.5 mb-2 border-b border-border" role="tablist">
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
          {rows.map(({ msg, originalIndex }) => (
            <MessageTableRow key={originalIndex} msg={msg} />
          ))}
        </TableBody>
      </Table>
      {hiddenCount > 0 && (
        <button
          onClick={() => setVisibleCount(c => c + VISIBLE_INCREMENT)}
          className="w-full py-2 text-xs text-muted-foreground hover:text-foreground hover:bg-muted/50 transition-colors rounded"
          data-testid="show-earlier-messages"
        >
          Show {Math.min(hiddenCount, VISIBLE_INCREMENT)} earlier message{hiddenCount !== 1 ? 's' : ''} ({hiddenCount} hidden)
        </button>
      )}
    </div>
  )
}
