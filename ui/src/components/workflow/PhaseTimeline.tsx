import { useMemo } from 'react'
import { Clock, Cpu } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { PhaseGraph } from './PhaseGraph'
import { WorkflowFindings } from './WorkflowFindings'
import { useAgentSessions } from '@/hooks/useTickets'
import type { WorkflowState, AgentHistoryEntry, AgentSession } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

interface PhaseTimelineProps {
  workflow: WorkflowState
  agentHistory?: AgentHistoryEntry[]
  ticketId?: string
  sessions?: AgentSession[]
  onAgentSelect?: (data: SelectedAgentData) => void
  logPanelCollapsed?: boolean
}

export function PhaseTimeline({ workflow, agentHistory, ticketId, sessions: sessionsProp, onAgentSelect, logPanelCollapsed }: PhaseTimelineProps) {
  const phases = workflow.phases || {}
  const activeAgents = useMemo(() => workflow.active_agents || {}, [workflow.active_agents])

  // Check if any agents are running (no result yet)
  const hasRunningAgents = useMemo(() => {
    return Object.values(activeAgents).some(a => !a.result)
  }, [activeAgents])

  // Fetch agent sessions (for history too) - real-time updates via WebSocket messages.updated events
  // Skip fetch when sessions are provided via prop (project scope)
  const { data: sessionsData } = useAgentSessions(
    ticketId || '',
    undefined, // all phases
    { enabled: !!ticketId && !sessionsProp }
  )
  const sessions = sessionsProp ?? sessionsData?.sessions

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

      {/* Phase Graph */}
      <PhaseGraph
        phases={phases}
        currentPhase={workflow.current_phase}
        activeAgents={activeAgents}
        agentHistory={agentHistory}
        phaseOrder={workflow.phase_order}
        sessions={sessions}
        onAgentSelect={onAgentSelect}
        logPanelCollapsed={logPanelCollapsed}
      />

      {/* Workflow Findings */}
      {workflow.findings && Object.keys(workflow.findings).length > 0 && (
        <WorkflowFindings findings={workflow.findings} />
      )}
    </div>
  )
}
