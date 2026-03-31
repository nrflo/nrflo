import { Handle, Position } from '@xyflow/react'
import { Check, Circle, AlertCircle, SkipForward, Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import { AgentCard } from './AgentCard'
import { HistoryAgentCard } from './HistoryAgentCard'
import { AgentSessionCard } from '../AgentSessionCard'
import type { PhaseNodeData } from './types'
import type { ActiveAgentV4, PhaseStatus, AgentSession, AgentHistoryEntry } from '@/types/workflow'

function StatusIcon({ status }: { status: PhaseStatus }) {
  switch (status) {
    case 'completed':
      return <Check className="h-4 w-4 text-green-500" />
    case 'in_progress':
      return <Loader2 className="h-4 w-4 text-yellow-500 spin-sync" />
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

export interface PhaseFlowNodeData extends PhaseNodeData {
  sessions?: AgentSession[]
  expandedAgentKey?: string | null
  onAgentClick?: (agentKey: string | null) => void
  onRetryFailed?: (sessionId: string) => void
  retryingSessionId?: string | null
  workflowStatus?: string
  [key: string]: unknown // Index signature for React Flow compatibility
}

interface PhaseFlowNodeProps {
  data: PhaseFlowNodeData
}

export function PhaseFlowNode({ data }: PhaseFlowNodeProps) {
  const { name, status, result, isCurrent, activeAgents, historyEntries, sessions, expandedAgentKey, onAgentClick, onRetryFailed, retryingSessionId, workflowStatus } = data
  const hasActiveAgents = activeAgents.length > 0 && status === 'in_progress'

  // Find sessions for active (running) agents in this phase
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

  // Find sessions for completed history entries
  const getSessionForHistoryEntry = (
    entry: AgentHistoryEntry,
    phaseName: string
  ): AgentSession | undefined => {
    if (!sessions) return undefined

    // Prefer exact session_id match
    if (entry.session_id) {
      const byId = sessions.find(s => s.id === entry.session_id)
      if (byId) return byId
    }

    // First try exact match with model_id
    const exactMatch = sessions.find(s =>
      s.agent_type === entry.agent_type &&
      s.phase === phaseName &&
      s.model_id === entry.model_id &&
      s.status !== 'running'
    )
    if (exactMatch) return exactMatch

    // Fallback: match by agent_type and phase only
    return sessions.find(s =>
      s.agent_type === entry.agent_type &&
      s.phase === phaseName &&
      s.status !== 'running'
    )
  }

  return (
    <div className="flex flex-col items-center">
      {/* Top handle for incoming edge */}
      <Handle
        type="target"
        position={Position.Top}
        className="!bg-transparent !border-0 !w-0 !h-0"
      />

      {/* Node container */}
      <div
        className={cn(
          getNodeStyles(status),
          'min-w-[380px] p-4',
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

        {/* Active agents row - grid layout for full-width distribution */}
        {hasActiveAgents && (
          <div className="grid grid-cols-[repeat(auto-fit,minmax(120px,1fr))] gap-2 mt-2 pt-2 border-t border-yellow-200 dark:border-yellow-800">
            {activeAgents.map((agent, i) => {
              const agentKey = `${name}-${i}`
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

        {/* Agent history cards for completed phases - grid layout for full-width distribution */}
        {status === 'completed' && historyEntries.length > 0 && (
          <div className="grid grid-cols-[repeat(auto-fit,minmax(100px,1fr))] gap-2 mt-2 pt-2 border-t border-green-200 dark:border-green-800">
            {historyEntries.map((entry, i) => {
              const agentKey = `${name}-history-${i}`
              const session = getSessionForHistoryEntry(entry, name)
              return (
                <HistoryAgentCard
                  key={agentKey}
                  entry={entry}
                  session={session}
                  isExpanded={expandedAgentKey === agentKey}
                  onExpand={() => onAgentClick?.(expandedAgentKey === agentKey ? null : agentKey)}
                  onRetryFailed={onRetryFailed}
                  retryingSessionId={retryingSessionId}
                  workflowStatus={workflowStatus}
                />
              )
            })}
          </div>
        )}
      </div>

      {/* Expanded agent session messages */}
      {expandedAgentKey && expandedAgentKey.startsWith(`${name}-`) && (
        <div className="mt-2 w-full max-w-md">
          {/* Handle active (running) agents */}
          {activeAgents.map((agent, i) => {
            const agentKey = `${name}-${i}`
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
          {/* Handle completed agents from history */}
          {historyEntries.map((entry, i) => {
            const agentKey = `${name}-history-${i}`
            if (expandedAgentKey !== agentKey) return null
            const session = getSessionForHistoryEntry(entry, name)
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

      {/* Bottom handle for outgoing edge */}
      <Handle
        type="source"
        position={Position.Bottom}
        className="!bg-transparent !border-0 !w-0 !h-0"
      />
    </div>
  )
}
