import { Check, Circle, AlertCircle, SkipForward, Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import { AgentCard } from './AgentCard'
import { AgentSessionCard } from '../AgentSessionCard'
import type { PhaseNodeProps } from './types'
import type { ActiveAgentV4, PhaseStatus, AgentSession } from '@/types/workflow'

function StatusIcon({ status }: { status: PhaseStatus }) {
  switch (status) {
    case 'completed':
      return <Check className="h-4 w-4 text-green-500" />
    case 'in_progress':
      return <Loader2 className="h-4 w-4 text-yellow-500 animate-spin" />
    case 'error':
      return <AlertCircle className="h-4 w-4 text-red-500" />
    case 'skipped':
      return <SkipForward className="h-4 w-4 text-gray-400" />
    case 'pending':
    default:
      return <Circle className="h-4 w-4 text-gray-300" />
  }
}

function getNodeStyles(status: PhaseStatus) {
  const base = 'rounded-lg border-2 transition-all'

  switch (status) {
    case 'completed':
      return cn(base, 'border-green-500 bg-green-50 dark:bg-green-950/30')
    case 'in_progress':
      return cn(base, 'border-yellow-500 bg-yellow-50 dark:bg-yellow-950/30')
    case 'error':
      return cn(base, 'border-red-500 bg-red-50 dark:bg-red-950/30')
    case 'skipped':
      return cn(base, 'border-gray-400 border-dashed bg-gray-50 dark:bg-gray-900/30')
    case 'pending':
    default:
      return cn(base, 'border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900')
  }
}

function getResultBadge(result?: string | null) {
  if (!result) return null

  if (result === 'pass') {
    return (
      <Badge className="text-xs bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
        pass
      </Badge>
    )
  }
  if (result === 'fail') {
    return (
      <Badge className="text-xs bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200">
        fail
      </Badge>
    )
  }
  if (result === 'skipped') {
    return (
      <Badge className="text-xs bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400">
        skipped
      </Badge>
    )
  }
  return (
    <Badge variant="secondary" className="text-xs">
      {result}
    </Badge>
  )
}

export function PhaseNode({
  node,
  sessions,
  expandedAgentKey,
  onAgentClick,
}: PhaseNodeProps) {
  const { name, status, result, isCurrent, activeAgents, historyEntries } = node
  const hasActiveAgents = activeAgents.length > 0 && status === 'in_progress'

  // Find sessions for agents in this phase
  const getSessionForAgent = (agent: ActiveAgentV4): AgentSession | undefined => {
    if (!sessions) return undefined
    if (agent.session_id) {
      const byId = sessions.find(s => s.id === agent.session_id)
      if (byId) return byId
    }
    return sessions.find(s =>
      s.agent_type === agent.agent_type &&
      s.status === 'running' &&
      (!agent.model_id || s.model_id === agent.model_id)
    )
  }

  return (
    <div className="flex flex-col items-center">
      {/* Node container */}
      <div
        className={cn(
          getNodeStyles(status),
          'min-w-[200px] p-3',
          isCurrent && 'ring-2 ring-primary ring-offset-2'
        )}
      >
        {/* Header: icon + name + result */}
        <div className="flex items-center justify-center gap-2 mb-1">
          <StatusIcon status={status} />
          <span className={cn(
            'text-sm font-medium',
            status === 'skipped' && 'text-gray-500 dark:text-gray-400'
          )}>
            {name.replace(/_/g, ' ')}
          </span>
          {getResultBadge(result)}
        </div>

        {/* Active agents row */}
        {hasActiveAgents && (
          <div className="flex flex-wrap justify-center gap-2 mt-2 pt-2 border-t border-yellow-200 dark:border-yellow-800">
            {activeAgents.map((agent, i) => {
              const agentKey = `${node.name}-${i}`
              const session = getSessionForAgent(agent)
              return (
                <AgentCard
                  key={agentKey}
                  agent={agent}
                  session={session}
                  isExpanded={expandedAgentKey === agentKey}
                  onExpand={() => {
                    if (onAgentClick) {
                      onAgentClick(expandedAgentKey === agentKey ? null : agentKey)
                    }
                  }}
                />
              )
            })}
          </div>
        )}

        {/* History badge for completed phases */}
        {status === 'completed' && historyEntries.length > 0 && (
          <div className="flex justify-center mt-1">
            <span className="text-xs text-muted-foreground">
              {historyEntries.length} run{historyEntries.length !== 1 ? 's' : ''}
            </span>
          </div>
        )}
      </div>

      {/* Expanded agent session messages */}
      {expandedAgentKey && expandedAgentKey.startsWith(`${node.name}-`) && (
        <div className="mt-2 w-full max-w-md">
          {activeAgents.map((agent, i) => {
            const agentKey = `${node.name}-${i}`
            if (expandedAgentKey !== agentKey) return null
            const session = getSessionForAgent(agent)
            if (!session) {
              return (
                <div key={agentKey} className="text-sm text-muted-foreground text-center p-2 border rounded-lg">
                  No session data available
                </div>
              )
            }
            return (
              <AgentSessionCard
                key={agentKey}
                session={session}
                defaultExpanded={true}
              />
            )
          })}
        </div>
      )}
    </div>
  )
}
