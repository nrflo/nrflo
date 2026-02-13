import { CheckCircle, XCircle, Timer } from 'lucide-react'
import { cn, formatElapsedTime, contextLeftColor } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import type { AgentHistoryEntry, AgentSession } from '@/types/workflow'

interface HistoryAgentCardProps {
  entry: AgentHistoryEntry
  session?: AgentSession
  isExpanded?: boolean
  onExpand?: () => void
}

function formatDuration(durationSec?: number): string {
  if (!durationSec) return '0s'
  const mins = Math.floor(durationSec / 60)
  const secs = durationSec % 60
  if (mins > 0) {
    return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`
  }
  return `${secs}s`
}

export function HistoryAgentCard({ entry, session, isExpanded, onExpand }: HistoryAgentCardProps) {
  const isPassed = entry.result === 'pass'
  const isFailed = entry.result === 'fail'

  // Extract model name from model_id (e.g., "claude-3-5-sonnet" -> "sonnet")
  const modelName = entry.model_id
    ? entry.model_id.split('-').pop() || entry.model_id
    : entry.agent_type

  const hasSession = session && session.message_count > 0

  return (
    <button
      onClick={onExpand}
      disabled={!hasSession}
      className={cn(
        'flex flex-col items-center gap-0.5 px-2 py-1.5 rounded-md border transition-all',
        'w-full text-xs',
        hasSession && 'hover:bg-muted/50 cursor-pointer',
        !hasSession && 'cursor-default opacity-80',
        isPassed && 'border-green-300 bg-green-50/50 dark:border-green-700 dark:bg-green-900/20',
        isFailed && 'border-red-300 bg-red-50/50 dark:border-red-700 dark:bg-red-900/20',
        !isPassed && !isFailed && 'border-gray-300 bg-gray-50/50 dark:border-gray-700 dark:bg-gray-900/20',
        isExpanded && 'ring-2 ring-primary ring-offset-1'
      )}
    >
      {/* Status + Model */}
      <div className="flex items-center gap-1">
        {isPassed && <CheckCircle className="h-3 w-3 text-green-500" />}
        {isFailed && <XCircle className="h-3 w-3 text-red-500" />}
        <span className="font-medium">{modelName}</span>
        {(entry.restart_count ?? 0) > 0 && (
          <span className="text-[9px] font-mono px-0.5 rounded bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400">
            ↻{entry.restart_count}
          </span>
        )}
      </div>

      {/* Duration + context */}
      <div className="flex items-center gap-1 text-muted-foreground">
        <Timer className="h-2.5 w-2.5" />
        <span>
          {entry.started_at && entry.ended_at
            ? formatElapsedTime(entry.started_at, entry.ended_at)
            : formatDuration(entry.duration_sec)}
        </span>
        {entry.context_left != null && (
          <span className={cn(
            'text-[9px] font-mono px-0.5 rounded',
            contextLeftColor(entry.context_left)
          )}>
            {entry.context_left}%
          </span>
        )}
      </div>

      {/* Message count badge if session available */}
      {hasSession && (
        <Badge variant="secondary" className="text-[9px] px-1 py-0 mt-0.5">
          {session.message_count} msg{session.message_count !== 1 ? 's' : ''}
        </Badge>
      )}
    </button>
  )
}
