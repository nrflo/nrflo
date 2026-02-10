import { useMemo } from 'react'
import { Clock, Cpu } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import { PhaseGraph } from './PhaseGraph'
import { WorkflowFindings } from './WorkflowFindings'
import { useAgentSessions } from '@/hooks/useTickets'
import type { WorkflowState, AgentHistoryEntry } from '@/types/workflow'

interface PhaseTimelineProps {
  workflow: WorkflowState
  agentHistory?: AgentHistoryEntry[]
  ticketId?: string
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

export function PhaseTimeline({ workflow, agentHistory, ticketId }: PhaseTimelineProps) {
  const phases = workflow.phases || {}
  const activeAgents = useMemo(() => workflow.active_agents || {}, [workflow.active_agents])

  // Check if any agents are running (no result yet)
  const hasRunningAgents = useMemo(() => {
    return Object.values(activeAgents).some(a => !a.result)
  }, [activeAgents])

  // Legacy v3 format check
  const hasV4Agents = Object.keys(activeAgents).length > 0
  const hasV3ActiveAgent = workflow.active_agent !== null && workflow.active_agent !== undefined

  // Fetch agent sessions (for history too) - real-time updates via WebSocket messages.updated events
  const { data: sessionsData } = useAgentSessions(
    ticketId || '',
    undefined, // all phases
    { enabled: !!ticketId }
  )

  if (Object.keys(phases).length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        No workflow phases defined yet
      </p>
    )
  }

  return (
    <div className="space-y-4">
      {/* Workflow metadata badges */}
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
        {hasRunningAgents && (
          <Badge className="text-xs bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200 animate-pulse">
            <Cpu className="h-3 w-3 mr-1" />
            Agents running
          </Badge>
        )}
      </div>

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

      {/* Phase Graph */}
      <PhaseGraph
        phases={phases}
        currentPhase={workflow.current_phase}
        activeAgents={activeAgents}
        agentHistory={agentHistory}
        phaseOrder={workflow.phase_order}
        sessions={sessionsData?.sessions}
      />

      {/* History (legacy display for workflows with history array) */}
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

      {/* Workflow Findings */}
      {workflow.findings && Object.keys(workflow.findings).length > 0 && (
        <WorkflowFindings findings={workflow.findings} />
      )}
    </div>
  )
}
