import { useState } from 'react'
import { Handle, Position } from '@xyflow/react'
import { Loader2, CheckCircle, XCircle, Timer, Clock, SkipForward, AlertTriangle, RefreshCw } from 'lucide-react'
import { cn, formatElapsedTime, contextLeftColor, isNearRestartThreshold, formatRestartReasons } from '@/lib/utils'
import { useTickingClock } from '@/hooks/useElapsedTime'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { Tooltip } from '@/components/ui/Tooltip'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import type { AgentFlowNodeData } from './types'

function StatusIcon({ result, isRunning, isPending, isSkipped }: { result?: string; isRunning: boolean; isPending?: boolean; isSkipped?: boolean }) {
  if (isSkipped) {
    return <SkipForward className="h-5 w-5 text-gray-400" />
  }
  if (isPending) {
    return <Clock className="h-5 w-5 text-gray-400" />
  }
  if (isRunning) {
    return <Loader2 className="h-5 w-5 text-yellow-600 dark:text-yellow-400 spin-sync" />
  }
  if (result === 'pass') {
    return <CheckCircle className="h-5 w-5 text-green-500" />
  }
  if (result === 'fail') {
    return <XCircle className="h-5 w-5 text-red-500" />
  }
  return null
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

interface AgentFlowNodeProps {
  data: AgentFlowNodeData
}

export function AgentFlowNode({ data }: AgentFlowNodeProps) {
  const [confirmOpen, setConfirmOpen] = useState(false)
  const { phaseName, agent, historyEntry, session, isPending, isSkipped, isCompleted, isError, onToggleExpand, onRetryFailed, retryingSessionId, workflowStatus } = data
  const isRunning = agent && !agent.result
  useTickingClock(!!isRunning)
  const result = agent?.result || historyEntry?.result
  const tag = agent?.tag || historyEntry?.tag
  const hasMessages = session && session.message_count > 0

  // Get model name
  const modelId = agent?.model_id || historyEntry?.model_id
  const modelName = modelId
    ? modelId.split('-').pop() || modelId
    : agent?.cli || historyEntry?.agent_type || 'agent'

  // Get duration: prefer started_at/ended_at, fallback to duration_sec
  const duration = agent?.started_at
    ? formatElapsedTime(agent.started_at, agent.ended_at)
    : historyEntry?.started_at && historyEntry?.ended_at
      ? formatElapsedTime(historyEntry.started_at, historyEntry.ended_at)
      : historyEntry?.duration_sec
        ? formatDuration(historyEntry.duration_sec)
        : '0s'

  // Get context_left from active agent or history entry
  const contextLeft = agent?.context_left ?? historyEntry?.context_left
  const restartCount = agent?.restart_count ?? historyEntry?.restart_count ?? 0
  const restartThreshold = agent?.restart_threshold ?? 25

  // Pending/skipped/completed-placeholder phases render differently
  if (isPending || isSkipped || isCompleted || isError) {
    const statusLabel = isSkipped ? 'skipped' : isCompleted ? 'completed' : isError ? 'error' : 'pending'
    const statusResult = isCompleted ? 'pass' : isError ? 'fail' : undefined
    return (
      <div className="nopan nodrag flex flex-col items-center" style={{ pointerEvents: 'all' }}>
        <Handle
          type="target"
          position={Position.Top}
          className="!bg-transparent !border-0 !w-0 !h-0"
        />

        <div
          className={cn(
            'w-[242px] sm:w-[330px] min-h-[90px] rounded-xl border-2 px-3 sm:px-5 py-4',
            isCompleted && 'border-green-500 bg-green-50 dark:bg-green-950/30',
            isError && 'border-red-500 bg-red-50 dark:bg-red-950/30',
            isSkipped && 'border-dashed border-gray-300 dark:border-gray-600 bg-gray-50 dark:bg-gray-900/50 opacity-60',
            isPending && 'border-dashed border-gray-300 dark:border-gray-600 bg-gray-50 dark:bg-gray-900/50'
          )}
        >
          {/* Phase label */}
          <div className="text-sm text-muted-foreground mb-2 text-center">
            {phaseName.replace(/_/g, ' ')}
          </div>

          {/* Status */}
          <div className="flex items-center justify-center gap-2">
            <StatusIcon isPending={isPending} isSkipped={isSkipped} result={statusResult} isRunning={false} />
            <span className="text-sm text-muted-foreground">
              {statusLabel}
            </span>
          </div>
        </div>

        <Handle
          type="source"
          position={Position.Bottom}
          className="!bg-transparent !border-0 !w-0 !h-0"
        />
      </div>
    )
  }

  return (
    <div className="nopan nodrag flex flex-col items-center" style={{ pointerEvents: 'all' }}>
      <Handle
        type="target"
        position={Position.Top}
        className="!bg-transparent !border-0 !w-0 !h-0"
      />

      <button
        onClick={(e) => {
          e.stopPropagation()
          onToggleExpand()
        }}
        style={{ pointerEvents: 'all' }}
        className={cn(
          'nopan nodrag relative',
          'w-[242px] sm:w-[330px] min-h-[90px] rounded-xl border-2 px-3 sm:px-5 py-4 transition-all',
          'hover:bg-muted/50 hover:scale-[1.02] cursor-pointer',
          isRunning && 'border-transparent bg-yellow-50 dark:bg-yellow-950/30 turbo-border',
          !isRunning && result === 'pass' && 'border-green-500 bg-green-50 dark:bg-green-950/30',
          !isRunning && result === 'fail' && 'border-red-500 bg-red-50 dark:bg-red-950/30',
          !isRunning && !result && 'border-gray-300 bg-white dark:border-gray-600 dark:bg-gray-900'
        )}
      >
        {/* Row 1: duration | phase name | context left */}
        <div className="flex items-center justify-between mb-2">
          <div className="flex items-center gap-1 text-sm text-muted-foreground">
            <Timer className="h-4 w-4" />
            <span>{duration}</span>
          </div>
          <span className="text-sm text-muted-foreground flex-1 text-center truncate px-1">
            {phaseName.replace(/_/g, ' ')}
          </span>
          {contextLeft != null ? (
            <span className={cn(
              'text-base font-mono px-1.5 py-0.5',
              contextLeftColor(contextLeft)
            )}>
              {contextLeft}%
            </span>
          ) : (
            <span className="text-base font-mono px-1.5 py-0.5 invisible" aria-hidden="true">{'\u00A0\u00A0\u00A0'}</span>
          )}
        </div>

        {/* Row 2: status icon + model + tag */}
        <div className="flex items-center justify-center gap-3">
          <StatusIcon result={result} isRunning={!!isRunning} />
          <span className="font-semibold">{modelName}</span>
          {tag && (
            <Badge variant="outline" className="text-xs border-emerald-300 text-emerald-600">
              {tag}
            </Badge>
          )}
        </div>

        {/* Restart count badge - top left corner */}
        {restartCount > 0 && (
          <span className="absolute top-1 left-1">
            <Tooltip text={formatRestartReasons(agent?.restart_details ?? historyEntry?.restart_details, restartCount)} placement="top">
              <span className="text-base font-mono px-1.5 py-0.5 rounded bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400">
                ↻{restartCount}
              </span>
            </Tooltip>
          </span>
        )}


        {/* Threshold proximity warning */}
        {isRunning && contextLeft != null && isNearRestartThreshold(contextLeft, restartThreshold) && (
          <div className="flex items-center gap-1 mt-2 text-xs text-amber-600 dark:text-amber-400">
            <AlertTriangle className="h-3.5 w-3.5" />
            <span>Restart at ≤{restartThreshold}%</span>
          </div>
        )}

        {/* Message count badge */}
        {hasMessages && (
          <Badge variant="secondary" className="text-xs px-2 py-0.5 mt-3">
            {session.message_count} msg{session.message_count !== 1 ? 's' : ''}
          </Badge>
        )}

        {/* Retry button for failed agents - bottom right */}
        {result === 'fail' && workflowStatus === 'failed' && onRetryFailed && historyEntry?.session_id && (
          <Tooltip text="Retry failed agent" placement="left">
            <button
              onClick={(e) => { e.stopPropagation(); setConfirmOpen(true) }}
              disabled={!!retryingSessionId}
              className="absolute bottom-2 right-2 p-1 rounded-md bg-red-100 hover:bg-red-200 dark:bg-red-900/50 dark:hover:bg-red-800/50 transition-colors disabled:opacity-50 border border-red-300 dark:border-red-700"
            >
              {retryingSessionId === historyEntry.session_id ? (
                <Spinner size="sm" />
              ) : (
                <RefreshCw className="h-4 w-4 text-red-600 dark:text-red-400" />
              )}
            </button>
          </Tooltip>
        )}
      </button>

      {/* Confirm dialog for retry (rendered outside button to avoid nesting) */}
      {result === 'fail' && workflowStatus === 'failed' && onRetryFailed && historyEntry?.session_id && (
        <ConfirmDialog
          open={confirmOpen}
          onClose={() => setConfirmOpen(false)}
          onConfirm={() => onRetryFailed!(historyEntry.session_id!)}
          title="Retry Failed Agent"
          message={`This will retry the failed "${historyEntry.agent_type}" agent from the failed layer. All agents in this layer will be re-run.`}
          confirmLabel="Retry"
          variant="destructive"
        />
      )}

      <Handle
        type="source"
        position={Position.Bottom}
        className="!bg-transparent !border-0 !w-0 !h-0"
      />
    </div>
  )
}
