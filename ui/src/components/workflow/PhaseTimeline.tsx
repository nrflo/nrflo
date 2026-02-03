import { Check, Clock, AlertCircle, SkipForward, Circle, Cpu } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import { PhaseCard } from './PhaseCard'
import { ActiveAgentsPanel } from './ActiveAgentsPanel'
import { AgentMessagesPanel } from './AgentMessagesPanel'
import { useAgentSessions } from '@/hooks/useTickets'
import type { WorkflowState, PhaseStatus, AgentHistoryEntry } from '@/types/workflow'

interface PhaseTimelineProps {
  workflow: WorkflowState
  agentHistory?: AgentHistoryEntry[]
  ticketId?: string
  liveTracking?: boolean
}

function StatusIcon({ status }: { status: PhaseStatus }) {
  switch (status) {
    case 'completed':
      return <Check className="h-4 w-4 text-green-500" />
    case 'in_progress':
      return <Clock className="h-4 w-4 text-yellow-500 animate-pulse" />
    case 'error':
      return <AlertCircle className="h-4 w-4 text-red-500" />
    case 'skipped':
      return <SkipForward className="h-4 w-4 text-gray-400" />
    case 'pending':
    default:
      return <Circle className="h-4 w-4 text-gray-300" />
  }
}

function statusBorderColor(status: PhaseStatus): string {
  switch (status) {
    case 'completed':
      return 'border-green-500'
    case 'in_progress':
      return 'border-yellow-500'
    case 'error':
      return 'border-red-500'
    case 'skipped':
      return 'border-gray-400'
    default:
      return 'border-gray-300'
  }
}

function categoryColor(category: string): string {
  switch (category) {
    case 'full':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
    case 'simple':
      return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200'
    case 'docs':
      return 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200'
    default:
      return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200'
  }
}

export function PhaseTimeline({ workflow, agentHistory, ticketId, liveTracking }: PhaseTimelineProps) {
  const phases = workflow.phases || {}
  const activeAgents = workflow.active_agents || {}
  // Only count running agents (those without a result)
  const runningAgentCount = Object.values(activeAgents).filter(a => !a.result).length
  // Total v4 agents count (for v3 fallback check)
  const hasV4Agents = Object.keys(activeAgents).length > 0
  const hasV3ActiveAgent = workflow.active_agent !== null && workflow.active_agent !== undefined

  // Fetch agent sessions when live tracking is enabled and we have a ticket
  const { data: sessionsData, isLoading: sessionsLoading } = useAgentSessions(
    ticketId || '',
    undefined, // all phases
    {
      enabled: !!ticketId && liveTracking,
      refetchInterval: liveTracking ? 5000 : false,
    }
  )

  // Build a map of phase -> earliest start time from agent_history
  const phaseStartTimes: Record<string, number> = {}
  if (agentHistory) {
    for (const entry of agentHistory) {
      if (entry.started_at && entry.phase) {
        const time = new Date(entry.started_at).getTime()
        if (!phaseStartTimes[entry.phase] || time < phaseStartTimes[entry.phase]) {
          phaseStartTimes[entry.phase] = time
        }
      }
    }
  }

  // Sort phases by their earliest start time from agent_history (phases without history go to the end)
  const sortedPhaseEntries = Object.entries(phases).sort(([nameA], [nameB]) => {
    const timeA = phaseStartTimes[nameA]
    const timeB = phaseStartTimes[nameB]
    if (!timeA && !timeB) return 0
    if (!timeA) return 1
    if (!timeB) return -1
    return timeA - timeB
  })

  if (sortedPhaseEntries.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        No workflow phases defined yet
      </p>
    )
  }

  return (
    <div className="space-y-4">
      {/* Workflow metadata */}
      <div className="flex items-center gap-2 flex-wrap">
        {workflow.version && (
          <Badge variant="outline" className="text-xs">
            v{workflow.version}
          </Badge>
        )}
        {workflow.category && (
          <Badge className={cn('text-xs', categoryColor(workflow.category))}>
            {workflow.category}
          </Badge>
        )}
        {workflow.current_phase && (
          <Badge variant="secondary" className="text-xs">
            <Clock className="h-3 w-3 mr-1" />
            {workflow.current_phase.replace(/_/g, ' ')}
          </Badge>
        )}
      </div>

      {/* Active agents panel (v4 format - only show when agents are running) */}
      {runningAgentCount > 0 && (
        <ActiveAgentsPanel agents={activeAgents} />
      )}

      {/* Agent sessions panel with real-time messages (when live tracking) */}
      {liveTracking && ticketId && (
        <AgentMessagesPanel
          sessions={sessionsData?.sessions || []}
          isLoading={sessionsLoading}
        />
      )}

      {/* Legacy active agent info (v3 format) */}
      {hasV3ActiveAgent && !hasV4Agents && (
        <div className="flex items-center gap-2 p-3 bg-yellow-50 dark:bg-yellow-900/20 rounded-lg border border-yellow-200 dark:border-yellow-800">
          <Cpu className="h-4 w-4 text-yellow-600 dark:text-yellow-400 animate-pulse" />
          <span className="text-sm font-medium text-yellow-800 dark:text-yellow-200">
            Active: {workflow.active_agent!.type}
          </span>
          {workflow.active_agent!.pid && (
            <span className="text-xs text-yellow-600 dark:text-yellow-400">
              PID: {workflow.active_agent!.pid}
            </span>
          )}
          {workflow.active_agent!.session_id && (
            <span className="text-xs text-yellow-600 dark:text-yellow-400 font-mono truncate max-w-[200px]">
              Session: {workflow.active_agent!.session_id.slice(0, 8)}...
            </span>
          )}
        </div>
      )}

      {/* Timeline */}
      <div className="relative">
        {sortedPhaseEntries.map(([phaseName, phase], index) => {
          const isLast = index === sortedPhaseEntries.length - 1
          const isCurrent = workflow.current_phase === phaseName

          return (
            <div key={phaseName} className="relative flex gap-4">
              {/* Timeline line and dot */}
              <div className="flex flex-col items-center">
                <div
                  className={cn(
                    'flex h-8 w-8 items-center justify-center rounded-full border-2 bg-background',
                    statusBorderColor(phase.status),
                    isCurrent && 'ring-2 ring-primary ring-offset-2'
                  )}
                >
                  <StatusIcon status={phase.status} />
                </div>
                {!isLast && (
                  <div
                    className={cn(
                      'w-0.5 flex-1 min-h-[24px]',
                      phase.status === 'completed'
                        ? 'bg-green-500'
                        : 'bg-gray-300 dark:bg-gray-600'
                    )}
                  />
                )}
              </div>

              {/* Phase content */}
              <div className={cn('flex-1 pb-6', isLast && 'pb-0')}>
                <PhaseCard
                  name={phaseName}
                  phase={phase}
                  isCurrent={isCurrent}
                  findings={workflow.findings}
                  activeAgents={workflow.active_agents}
                  agentHistory={agentHistory}
                />
              </div>
            </div>
          )
        })}
      </div>

      {/* History */}
      {workflow.history && workflow.history.length > 0 && (
        <div className="mt-6 pt-4 border-t border-border">
          <h4 className="text-sm font-medium mb-3">Agent History</h4>
          <div className="space-y-2">
            {workflow.history.map((entry, index) => (
              <div
                key={index}
                className="flex items-center gap-3 text-sm text-muted-foreground"
              >
                <span className="font-mono">{entry.type}</span>
                <span className="text-xs">({entry.phase})</span>
                <span
                  className={cn(
                    'text-xs px-1.5 py-0.5 rounded',
                    entry.status === 'completed'
                      ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200'
                      : entry.status === 'error'
                        ? 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
                        : 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200'
                  )}
                >
                  {entry.status}
                </span>
                {entry.started_at && (
                  <span className="text-xs ml-auto">
                    {new Date(entry.started_at).toLocaleString()}
                  </span>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
