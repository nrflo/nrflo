import { cn } from '@/lib/utils'
import { InstanceList } from './ProjectWorkflowComponents'
import { AgentLogPanel } from '@/components/workflow/AgentLogPanel'
import { TraceView } from '@/components/workflow/Trace'
import type { WorkflowState, AgentSession, ActiveAgentV4 } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'

interface TicketTraceSectionProps {
  instanceIds: string[]
  instances: Record<string, WorkflowState>
  labels: Record<string, string>
  resolvedInstanceId: string
  onSelectInstance: (id: string) => void
  displayedState: WorkflowState | null
  sessions: AgentSession[]
  activeAgents: Record<string, ActiveAgentV4>
  selectedPanelAgent: SelectedAgentData | null
  onAgentSelect: (data: SelectedAgentData | null) => void
  logPanelCollapsed: boolean
  onResumeSession: (sessionId: string) => void
  resumePending: boolean
  projectFindings?: Record<string, unknown>
}

/** Trace sub-tab body: instance picker + timeline + agent log side panel. */
export function TicketTraceSection({
  instanceIds,
  instances,
  labels,
  resolvedInstanceId,
  onSelectInstance,
  displayedState,
  sessions,
  activeAgents,
  selectedPanelAgent,
  onAgentSelect,
  logPanelCollapsed,
  onResumeSession,
  resumePending,
  projectFindings,
}: TicketTraceSectionProps) {
  return (
    <div className={cn('flex flex-col md:flex-row gap-0', selectedPanelAgent && 'min-h-[calc(100vh-280px)]')}>
      <div className="flex-1 min-w-0 space-y-4">
        {instanceIds.length > 1 && (
          <InstanceList
            instanceIds={instanceIds}
            instances={instances}
            labels={labels}
            selectedId={resolvedInstanceId}
            onSelect={onSelectInstance}
            tab="running"
          />
        )}
        {resolvedInstanceId ? (
          <TraceView
            instanceId={resolvedInstanceId}
            sessions={sessions}
            workflowState={displayedState ?? undefined}
            onAgentSelect={onAgentSelect}
          />
        ) : (
          <div className="text-center py-8">
            <p className="text-muted-foreground text-sm">No workflow runs to trace</p>
          </div>
        )}
      </div>
      {selectedPanelAgent && (
        <AgentLogPanel
          activeAgents={activeAgents}
          sessions={sessions}
          collapsed={logPanelCollapsed}
          selectedAgent={selectedPanelAgent}
          onAgentSelect={onAgentSelect}
          onResumeSession={onResumeSession}
          resumePending={resumePending}
          agentFindings={displayedState?.findings}
          projectFindings={projectFindings}
          phaseLayers={displayedState?.phase_layers}
          workflowFindings={displayedState?.workflow_findings}
        />
      )}
    </div>
  )
}
