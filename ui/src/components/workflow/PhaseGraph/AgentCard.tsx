import { Loader2, CheckCircle, XCircle, Timer, AlertTriangle, Terminal, Clock } from 'lucide-react'
import { cn, formatElapsedTime, contextLeftColor, isNearRestartThreshold, formatRestartReasons } from '@/lib/utils'
import { formatRateLimitCountdown } from '@/lib/rateLimit'
import { useTickingClock } from '@/hooks/useElapsedTime'
import { Badge } from '@/components/ui/Badge'
import { Tooltip } from '@/components/ui/Tooltip'
import type { AgentCardProps } from './types'

function AgentStatusIcon({ result, isInteractive, isRateLimited }: { result?: string; isInteractive?: boolean; isRateLimited?: boolean }) {
  if (isInteractive) {
    return <Terminal className="h-3.5 w-3.5 text-blue-500" />
  }
  if (isRateLimited) {
    return <Clock className="h-3.5 w-3.5 text-amber-500" />
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
  const isRateLimited = !!agent.waiting_for_rate_limit
  const isRunning = !agent.result && !isInteractive
  const elapsedTime = agent.started_at
    ? formatElapsedTime(agent.started_at, agent.ended_at)
    : '0s'

  useTickingClock(isRateLimited)

  const rateLimitCountdown = isRateLimited && agent.rate_limit_until_ts
    ? formatRateLimitCountdown(new Date(agent.rate_limit_until_ts), new Date())
    : null

  // Extract model name from model_id (e.g., "claude-3-5-sonnet" -> "sonnet")
  const modelName = agent.model_id
    ? agent.model_id.split('-').pop() || agent.model_id
    : 'agent'

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
        isRunning && 'border-amber-400 dark:border-amber-500 bg-amber-50/50 dark:bg-amber-900/20',
        !isRunning && !isInteractive && agent.result === 'pass' && 'border-green-400 bg-green-50/50 dark:bg-green-900/20',
        !isRunning && !isInteractive && agent.result === 'fail' && 'border-red-400 bg-red-50/50 dark:bg-red-900/20',
        isExpanded && 'ring-2 ring-primary ring-offset-1'
      )}
    >
      {/* Restart count badge - top left corner */}
      {(agent.restart_count ?? 0) > 0 && (
        <span className="absolute top-1 left-1">
          <Tooltip text={formatRestartReasons(agent.restart_details, agent.restart_count)} placement="top">
            <span className="text-xs font-mono px-1 rounded bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400">
              ↻{agent.restart_count}
            </span>
          </Tooltip>
        </span>
      )}

      {/* Nudge count badge - bottom left corner */}
      {(agent.nudge_count ?? 0) > 0 && (
        <span className="absolute bottom-1 left-1">
          <Tooltip text="Idle reminder sent — agent has not called nrflo agent continue/fail" placement="top">
            <span className="text-xs font-mono px-1 rounded bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
              ⏰{agent.nudge_count}/5
            </span>
          </Tooltip>
        </span>
      )}

      {/* Status + Model + Tag */}
      <div className="flex items-center gap-1.5">
        <AgentStatusIcon result={agent.result} isInteractive={isInteractive} isRateLimited={isRateLimited} />
        <span className="text-xs font-medium">{isInteractive ? 'Interactive' : modelName}</span>
        {agent.tag && (
          <Badge variant="outline" className="text-xs border-emerald-300 text-emerald-600">
            {agent.tag}
          </Badge>
        )}
      </div>

      {/* Elapsed time or rate-limit countdown */}
      {isRateLimited ? (
        <div className="flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
          <span>Waiting · retry #{agent.rate_limit_retry_count ?? 0} · resumes {rateLimitCountdown || 'Resuming…'}</span>
        </div>
      ) : (
        <div className="flex items-center gap-1 text-xs text-muted-foreground">
          <Timer className="h-3 w-3" />
          <span>{elapsedTime}</span>
        </div>
      )}

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
