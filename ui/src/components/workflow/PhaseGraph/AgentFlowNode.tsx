import { Handle, Position } from '@xyflow/react'
import { Loader2, CheckCircle, XCircle, Timer, Clock, SkipForward } from 'lucide-react'
import { cn, formatElapsedTime } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import type { AgentFlowNodeData } from './types'

function StatusIcon({ result, isRunning, isPending, isSkipped }: { result?: string; isRunning: boolean; isPending?: boolean; isSkipped?: boolean }) {
  if (isSkipped) {
    return <SkipForward className="h-5 w-5 text-gray-400" />
  }
  if (isPending) {
    return <Clock className="h-5 w-5 text-gray-400" />
  }
  if (isRunning) {
    return <Loader2 className="h-5 w-5 text-yellow-600 dark:text-yellow-400 animate-spin" />
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
  const { phaseName, agent, historyEntry, session, isPending, isSkipped, isCompleted, isError, onToggleExpand } = data
  const isRunning = agent && !agent.result
  const result = agent?.result || historyEntry?.result
  const hasMessages = session && session.message_count > 0

  // Get model name
  const modelId = agent?.model_id || historyEntry?.model_id
  const modelName = modelId
    ? modelId.split('-').pop() || modelId
    : agent?.cli || historyEntry?.agent_type || 'agent'

  // Get duration
  const duration = agent?.started_at
    ? formatElapsedTime(agent.started_at, agent.ended_at)
    : historyEntry?.duration_sec
      ? formatDuration(historyEntry.duration_sec)
      : '0s'

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
            'min-w-[180px] rounded-xl border-2 px-5 py-4',
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
          'nopan nodrag',
          'min-w-[220px] rounded-xl border-2 px-5 py-4 transition-all',
          'hover:bg-muted/50 hover:scale-[1.02] cursor-pointer',
          isRunning && 'border-yellow-500 bg-yellow-50 dark:bg-yellow-950/30 animate-pulse-glow',
          !isRunning && result === 'pass' && 'border-green-500 bg-green-50 dark:bg-green-950/30',
          !isRunning && result === 'fail' && 'border-red-500 bg-red-50 dark:bg-red-950/30',
          !isRunning && !result && 'border-gray-300 bg-white dark:border-gray-600 dark:bg-gray-900'
        )}
      >
        {/* Phase label */}
        <div className="text-sm text-muted-foreground mb-2">
          {phaseName.replace(/_/g, ' ')}
        </div>

        {/* Agent info: status icon, model, duration */}
        <div className="flex items-center justify-center gap-3">
          <StatusIcon result={result} isRunning={!!isRunning} />
          <span className="font-semibold">{modelName}</span>
          <div className="flex items-center gap-1 text-sm text-muted-foreground">
            <Timer className="h-4 w-4" />
            <span>{duration}</span>
          </div>
        </div>

        {/* Message count badge */}
        {hasMessages && (
          <Badge variant="secondary" className="text-xs px-2 py-0.5 mt-3">
            {session.message_count} msg{session.message_count !== 1 ? 's' : ''}
          </Badge>
        )}
      </button>

      <Handle
        type="source"
        position={Position.Bottom}
        className="!bg-transparent !border-0 !w-0 !h-0"
      />
    </div>
  )
}
