import { useState } from 'react'
import { CheckCircle, XCircle, Timer, RefreshCw } from 'lucide-react'
import { cn, formatElapsedTime, contextLeftColor } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { Tooltip } from '@/components/ui/Tooltip'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import type { AgentHistoryEntry, AgentSession } from '@/types/workflow'

interface HistoryAgentCardProps {
  entry: AgentHistoryEntry
  session?: AgentSession
  isExpanded?: boolean
  onExpand?: () => void
  onRetryFailed?: (sessionId: string) => void
  retryingSessionId?: string | null
  workflowStatus?: string
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

export function HistoryAgentCard({ entry, session, isExpanded, onExpand, onRetryFailed, retryingSessionId, workflowStatus }: HistoryAgentCardProps) {
  const [confirmOpen, setConfirmOpen] = useState(false)
  const isPassed = entry.result === 'pass'
  const isFailed = entry.result === 'fail'
  const showRetry = isFailed && workflowStatus === 'failed' && onRetryFailed && entry.session_id

  // Extract model name from model_id (e.g., "claude-3-5-sonnet" -> "sonnet")
  const modelName = entry.model_id
    ? entry.model_id.split('-').pop() || entry.model_id
    : entry.agent_type

  const hasSession = session && session.message_count > 0

  return (
    <div className="relative">
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

      {/* Retry button - positioned at top-right of the card */}
      {showRetry && (
        <>
          <Tooltip text="Retry failed agent" placement="top">
            <button
              onClick={(e) => { e.stopPropagation(); setConfirmOpen(true) }}
              disabled={!!retryingSessionId}
              className="absolute -top-1.5 -right-1.5 p-0.5 rounded-full bg-red-100 hover:bg-red-200 dark:bg-red-900/50 dark:hover:bg-red-800/50 transition-colors disabled:opacity-50 border border-red-300 dark:border-red-700"
            >
              {retryingSessionId === entry.session_id ? (
                <Spinner size="sm" />
              ) : (
                <RefreshCw className="h-3 w-3 text-red-600 dark:text-red-400" />
              )}
            </button>
          </Tooltip>
          <ConfirmDialog
            open={confirmOpen}
            onClose={() => setConfirmOpen(false)}
            onConfirm={() => onRetryFailed!(entry.session_id!)}
            title="Retry Failed Agent"
            message={`This will retry the failed "${entry.agent_type}" agent from the failed layer. All agents in this layer will be re-run.`}
            confirmLabel="Retry"
            variant="destructive"
          />
        </>
      )}
    </div>
  )
}
