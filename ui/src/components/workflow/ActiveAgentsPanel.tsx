import { useState, useEffect } from 'react'
import { Cpu, Terminal, Hash, Clock, CheckCircle, XCircle, Loader2, Timer, RefreshCw, AlertTriangle } from 'lucide-react'
import { cn, formatElapsedTime, contextLeftColor, isNearRestartThreshold } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import type { ActiveAgentV4 } from '@/types/workflow'

interface ActiveAgentsPanelProps {
  agents: Record<string, ActiveAgentV4>
  onRestart?: (sessionId: string) => void
  restartingSessionId?: string | null
}

function AgentStatusIcon({ result }: { result?: string }) {
  if (!result) {
    return <Loader2 className="h-4 w-4 text-yellow-500 animate-spin" />
  }
  if (result === 'pass') {
    return <CheckCircle className="h-4 w-4 text-green-500" />
  }
  if (result === 'fail') {
    return <XCircle className="h-4 w-4 text-red-500" />
  }
  return <Clock className="h-4 w-4 text-gray-400" />
}

function resultColor(result?: string): string {
  if (!result) return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200'
  if (result === 'pass') return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200'
  if (result === 'fail') return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
  return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200'
}

export function ActiveAgentsPanel({ agents, onRestart, restartingSessionId }: ActiveAgentsPanelProps) {
  const agentEntries = Object.entries(agents)
  const runningAgents = agentEntries.filter(([, a]) => !a.result)
  const runningCount = runningAgents.length
  const passedCount = agentEntries.filter(([, a]) => a.result === 'pass').length
  const failedCount = agentEntries.filter(([, a]) => a.result === 'fail').length

  // Update elapsed time every second for running agents
  const [, setTick] = useState(0)
  useEffect(() => {
    if (runningCount === 0) return
    const interval = setInterval(() => setTick(t => t + 1), 1000)
    return () => clearInterval(interval)
  }, [runningCount])

  return (
    <div className="rounded-lg border border-yellow-200 dark:border-yellow-800 bg-yellow-50/50 dark:bg-yellow-900/10 overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2 bg-yellow-100/50 dark:bg-yellow-900/20 border-b border-yellow-200 dark:border-yellow-800">
        <div className="flex items-center gap-2">
          <Cpu className="h-4 w-4 text-yellow-600 dark:text-yellow-400" />
          <span className="text-sm font-medium text-yellow-800 dark:text-yellow-200">
            Active Agents
          </span>
          <Badge variant="secondary" className="text-xs">
            {runningCount} agent{runningCount !== 1 ? 's' : ''}
          </Badge>
        </div>
        <div className="flex items-center gap-2 text-xs">
          {runningCount > 0 && (
            <span className="flex items-center gap-1 text-yellow-600 dark:text-yellow-400">
              <Loader2 className="h-3 w-3 animate-spin" />
              {runningCount} running
            </span>
          )}
          {passedCount > 0 && (
            <span className="flex items-center gap-1 text-green-600 dark:text-green-400">
              <CheckCircle className="h-3 w-3" />
              {passedCount} passed
            </span>
          )}
          {failedCount > 0 && (
            <span className="flex items-center gap-1 text-red-600 dark:text-red-400">
              <XCircle className="h-3 w-3" />
              {failedCount} failed
            </span>
          )}
        </div>
      </div>

      {/* Agent list - only show running agents */}
      <div className="divide-y divide-yellow-200 dark:divide-yellow-800">
        {runningAgents.map(([key, agent]) => (
          <div
            key={key}
            className={cn(
              'px-4 py-3 flex items-center gap-4',
              !agent.result && 'bg-yellow-50/50 dark:bg-yellow-900/5'
            )}
          >
            {/* Status icon */}
            <AgentStatusIcon result={agent.result} />

            {/* Agent info */}
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="font-medium text-sm">
                  {agent.agent_type}
                </span>
                {(agent.restart_count ?? 0) > 0 && (
                  <span className="text-xs font-mono px-1 rounded bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400">
                    ↻{agent.restart_count}
                  </span>
                )}
                {agent.model_id && (
                  <Badge variant="outline" className="text-xs font-mono">
                    {agent.model_id}
                  </Badge>
                )}
                {agent.result && (
                  <Badge className={cn('text-xs', resultColor(agent.result))}>
                    {agent.result}
                  </Badge>
                )}
              </div>
              <div className="flex items-center gap-3 mt-1 text-xs text-muted-foreground">
                {agent.cli && (
                  <span className="flex items-center gap-1">
                    <Terminal className="h-3 w-3" />
                    {agent.cli}
                  </span>
                )}
                {agent.pid && (
                  <span className="flex items-center gap-1">
                    <Hash className="h-3 w-3" />
                    PID {agent.pid}
                  </span>
                )}
                {agent.session_id && (
                  <span className="font-mono truncate max-w-[150px]" title={agent.session_id}>
                    {agent.session_id.slice(0, 8)}...
                  </span>
                )}
                {agent.started_at && (
                  <span className="flex items-center gap-1">
                    <Timer className="h-3 w-3" />
                    {agent.result
                      ? formatElapsedTime(agent.started_at, agent.ended_at)
                      : formatElapsedTime(agent.started_at)}
                  </span>
                )}
                {agent.context_left != null && (
                  <span className={cn(
                    'text-xs font-mono px-1.5 py-0.5 rounded flex items-center gap-1',
                    contextLeftColor(agent.context_left)
                  )}>
                    {!agent.result && isNearRestartThreshold(agent.context_left, agent.restart_threshold ?? 25) && (
                      <AlertTriangle className="h-3 w-3 text-amber-500" />
                    )}
                    {agent.context_left}% ctx
                    {!agent.result && isNearRestartThreshold(agent.context_left, agent.restart_threshold ?? 25) && (
                      <span className="text-amber-600 dark:text-amber-400">(restart ≤{agent.restart_threshold ?? 25}%)</span>
                    )}
                  </span>
                )}
              </div>
            </div>

            {/* Restart button for running agents */}
            {onRestart && agent.session_id && !agent.result && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => onRestart(agent.session_id!)}
                disabled={restartingSessionId === agent.session_id}
                title="Restart agent (save context, relaunch)"
                className="ml-auto shrink-0"
              >
                {restartingSessionId === agent.session_id ? (
                  <Spinner size="sm" />
                ) : (
                  <RefreshCw className="h-3.5 w-3.5" />
                )}
              </Button>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
