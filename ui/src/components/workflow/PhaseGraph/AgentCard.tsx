import { Loader2, CheckCircle, XCircle, Timer, AlertTriangle, Terminal } from 'lucide-react'
import { cn, formatElapsedTime, contextLeftColor, isNearRestartThreshold, formatRestartReasons } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import { Tooltip } from '@/components/ui/Tooltip'
import type { AgentCardProps } from './types'

function AgentStatusIcon({ result, isInteractive }: { result?: string; isInteractive?: boolean }) {
  if (isInteractive) {
    return <Terminal className="h-3.5 w-3.5 text-blue-500" />
  }
  if (!result) {
    return <Loader2 className="h-3.5 w-3.5 text-yellow-600 dark:text-yellow-400 spin-sync" />
  }
  if (result === 'pass') {
    return <CheckCircle className="h-3.5 w-3.5 text-green-500" />
  }
  if (result === 'fail') {
    return <XCircle className="h-3.5 w-3.5 text-red-500" />
  }
  return null
}

export function AgentCard({ agent, session, onExpand, isExpanded }: AgentCardProps) {
  const isInteractive = session?.status === 'user_interactive'
  const isRunning = !agent.result && !isInteractive
  const elapsedTime = agent.started_at
    ? formatElapsedTime(agent.started_at, agent.ended_at)
    : '0s'

  // Extract model name from model_id (e.g., "claude-3-5-sonnet" -> "sonnet")
  const modelName = agent.model_id
    ? agent.model_id.split('-').pop() || agent.model_id
    : agent.cli || 'agent'

  const handleClick = () => {
    if (onExpand) {
      onExpand()
    }
  }

  return (
    <button
      onClick={handleClick}
      className={cn(
        'relative flex flex-col items-center gap-1 px-3 py-2 rounded-lg border transition-all',
        'hover:bg-muted/50 cursor-pointer w-full',
        isInteractive && 'border-blue-400 bg-blue-50/50 dark:bg-blue-900/20 animate-pulse-glow-blue',
        isRunning && 'border-yellow-400 bg-yellow-50/50 dark:bg-yellow-900/20 animate-pulse-glow',
        !isRunning && !isInteractive && agent.result === 'pass' && 'border-green-400 bg-green-50/50 dark:bg-green-900/20',
        !isRunning && !isInteractive && agent.result === 'fail' && 'border-red-400 bg-red-50/50 dark:bg-red-900/20',
        isExpanded && 'ring-2 ring-primary ring-offset-1'
      )}
    >
      {/* Restart count badge - top left corner */}
      {(agent.restart_count ?? 0) > 0 && (
        <span className="absolute top-1 left-1">
          <Tooltip text={formatRestartReasons(agent.restart_reasons, agent.restart_count)} placement="top">
            <span className="text-xs font-mono px-1 rounded bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400">
              ↻{agent.restart_count}
            </span>
          </Tooltip>
        </span>
      )}

      {/* Status + Model */}
      <div className="flex items-center gap-1.5">
        <AgentStatusIcon result={agent.result} isInteractive={isInteractive} />
        <span className="text-xs font-medium">{isInteractive ? 'Interactive' : modelName}</span>
      </div>

      {/* Elapsed time + context */}
      <div className="flex items-center gap-1 text-xs text-muted-foreground">
        <Timer className="h-3 w-3" />
        <span>{elapsedTime}</span>
      </div>

      {/* Context left badge - top right corner */}
      {agent.context_left != null && (
        <span className={cn(
          'absolute top-1 right-1 text-lg font-mono px-1 flex items-center gap-0.5',
          contextLeftColor(agent.context_left)
        )}>
          {isRunning && isNearRestartThreshold(agent.context_left, agent.restart_threshold ?? 25) && (
            <AlertTriangle className="h-4 w-4 text-amber-500" />
          )}
          {agent.context_left}%
        </span>
      )}

      {/* Session stats if available */}
      {session && session.message_count > 0 && (
        <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
          {session.message_count} msg{session.message_count !== 1 ? 's' : ''}
        </Badge>
      )}
    </button>
  )
}
