import { cn } from '@/lib/utils'
import { InstanceList } from './ProjectWorkflowComponents'
import { CompletedAgentsTable } from '@/components/workflow/CompletedAgentsTable'
import { AgentLogPanel } from '@/components/workflow/AgentLogPanel'
import type { WorkflowState, AgentSession, CompletedAgentRow } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'

interface TicketCompletedSectionProps {
  instanceIds: string[]
  instances: Record<string, WorkflowState>
  labels: Record<string, string>
  resolvedInstanceId: string
  onSelectInstance: (id: string) => void
  mergedCompletedAgents: CompletedAgentRow[]
  allCompletedSessions: AgentSession[]
  displayedState: WorkflowState | null
  selectedPanelAgent: SelectedAgentData | null
  onAgentSelect: (data: SelectedAgentData | null) => void
  logPanelCollapsed: boolean
  onResumeSession: (sessionId: string) => void
  resumePending: boolean
  projectFindings?: Record<string, unknown>
}

/** Completed sub-tab body: instance picker + merged agents table + log panel. */
export function TicketCompletedSection({
  instanceIds,
  instances,
  labels,
  resolvedInstanceId,
  onSelectInstance,
  mergedCompletedAgents,
  allCompletedSessions,
  displayedState,
  selectedPanelAgent,
  onAgentSelect,
  logPanelCollapsed,
  onResumeSession,
  resumePending,
  projectFindings,
}: TicketCompletedSectionProps) {
  return (
    <div className={cn('flex flex-col md:flex-row gap-0', selectedPanelAgent && 'min-h-[calc(100vh-280px)]')}>
      <div className="flex-1 min-w-0 space-y-4">
        {instanceIds.length > 0 && (
          <InstanceList
            instanceIds={instanceIds}
            instances={instances}
            labels={labels}
            selectedId={resolvedInstanceId}
            onSelect={onSelectInstance}
            tab="completed"
          />
        )}
        {mergedCompletedAgents.length > 0 ? (
          <CompletedAgentsTable
            agentHistory={mergedCompletedAgents}
            sessions={allCompletedSessions}
            onAgentSelect={onAgentSelect}
          />
        ) : (
          <div className="text-center py-8">
            <p className="text-muted-foreground text-sm">No completed workflows</p>
          </div>
        )}
      </div>
      {selectedPanelAgent && (
        <AgentLogPanel
          activeAgents={{}}
          sessions={allCompletedSessions}
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
