import type { ReactNode } from 'react'
import { WorkflowTabContent } from './WorkflowTabContent'
import { WorkflowInstanceTable } from './WorkflowInstanceTable'
import type { WorkflowState, AgentSession } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'

interface ProjectTerminalTabProps {
  instanceIds: string[]
  instances: Record<string, WorkflowState>
  resolvedInstanceId: string
  selectedInstanceId: string
  onSelectInstance: (id: string) => void
  onDelete: (id: string) => void
  onTrace: (id: string) => void
  displayedState: WorkflowState | null
  displayedWorkflowName: string
  selectorLabels: Record<string, string>
  sessions: AgentSession[]
  logPanelCollapsed: boolean
  onToggleLogPanel: () => void
  selectedPanelAgent: SelectedAgentData | null
  onAgentSelect: (data: SelectedAgentData | null) => void
  onResumeSession: (sessionId: string) => void
  resumePending: boolean
  projectFindings?: Record<string, unknown>
  onRetryFailed?: (sessionId: string) => void
  retryingSessionId?: string | null
  headerExtra?: ReactNode
}

/** Completed/Failed project tab: paginated instance table + read-only workflow view. */
export function ProjectTerminalTab({
  instanceIds,
  instances,
  resolvedInstanceId,
  selectedInstanceId,
  onSelectInstance,
  onDelete,
  onTrace,
  displayedState,
  displayedWorkflowName,
  selectorLabels,
  sessions,
  logPanelCollapsed,
  onToggleLogPanel,
  selectedPanelAgent,
  onAgentSelect,
  onResumeSession,
  resumePending,
  projectFindings,
  onRetryFailed,
  retryingSessionId,
  headerExtra,
}: ProjectTerminalTabProps) {
  return (
    <>
      <WorkflowInstanceTable
        instanceIds={instanceIds}
        instances={instances}
        selectedId={resolvedInstanceId}
        onSelect={onSelectInstance}
        onDelete={onDelete}
        onTrace={onTrace}
      />
      <WorkflowTabContent
        ticketId={undefined}
        hasWorkflow={instanceIds.length > 0}
        displayedState={displayedState}
        displayedWorkflowName={displayedWorkflowName}
        hasMultipleWorkflows={instanceIds.length > 1}
        workflows={instanceIds}
        workflowLabels={selectorLabels}
        selectedWorkflow={selectedInstanceId}
        onSelectWorkflow={onSelectInstance}
        isOrchestrated={false}
        hasActivePhase={false}
        activeAgents={{}}
        sessions={sessions}
        logPanelCollapsed={logPanelCollapsed}
        onToggleLogPanel={onToggleLogPanel}
        selectedPanelAgent={selectedPanelAgent}
        onAgentSelect={onAgentSelect}
        onStop={() => {}}
        stopPending={false}
        onRetryFailed={onRetryFailed ?? (() => {})}
        retryingSessionId={retryingSessionId ?? null}
        onTakeControl={() => {}}
        takeControlPending={false}
        onResumeSession={onResumeSession}
        resumeSessionPending={resumePending}
        projectFindings={projectFindings}
        headerExtra={headerExtra}
      />
    </>
  )
}
