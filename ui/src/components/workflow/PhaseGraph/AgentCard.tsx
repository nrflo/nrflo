import { Loader2, CheckCircle, XCircle, Timer } from 'lucide-react'
import { cn, formatElapsedTime, contextLeftColor } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import type { AgentCardProps } from './types'

function AgentStatusIcon({ result }: { result?: string }) {
  if (!result) {
    return <Loader2 className="h-3.5 w-3.5 text-yellow-600 dark:text-yellow-400 animate-spin" />
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
  const isRunning = !agent.result
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
        'flex flex-col items-center gap-1 px-3 py-2 rounded-lg border transition-all',
        'hover:bg-muted/50 cursor-pointer w-full',
        isRunning && 'border-yellow-400 bg-yellow-50/50 dark:bg-yellow-900/20 animate-pulse-glow',
        !isRunning && agent.result === 'pass' && 'border-green-400 bg-green-50/50 dark:bg-green-900/20',
        !isRunning && agent.result === 'fail' && 'border-red-400 bg-red-50/50 dark:bg-red-900/20',
        isExpanded && 'ring-2 ring-primary ring-offset-1'
      )}
    >
      {/* Status + Model */}
      <div className="flex items-center gap-1.5">
        <AgentStatusIcon result={agent.result} />
        <span className="text-xs font-medium">{modelName}</span>
      </div>

      {/* Elapsed time + context */}
      <div className="flex items-center gap-1 text-xs text-muted-foreground">
        <Timer className="h-3 w-3" />
        <span>{elapsedTime}</span>
        {agent.context_left != null && (
          <span className={cn(
            'text-[10px] font-mono px-1 rounded',
            contextLeftColor(agent.context_left)
          )}>
            {agent.context_left}%
          </span>
        )}
      </div>

      {/* Session stats if available */}
      {session && session.message_count > 0 && (
        <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
          {session.message_count} msg{session.message_count !== 1 ? 's' : ''}
        </Badge>
      )}
    </button>
  )
}
