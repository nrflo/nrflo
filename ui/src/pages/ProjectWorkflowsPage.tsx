import { useState, useEffect } from 'react'
import { useProjectStore } from '@/stores/projectStore'
import { useWebSocket } from '@/hooks/useWebSocket'
import {
  useProjectWorkflow,
  useProjectAgentSessions,
  useStopProjectWorkflow,
  useRestartProjectAgent,
} from '@/hooks/useTickets'
import { RunProjectWorkflowDialog } from '@/components/workflow/RunProjectWorkflowDialog'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowState } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'

export function ProjectWorkflowsPage() {
  const currentProject = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)

  const [selectedWorkflow, setSelectedWorkflow] = useState('')
  const [showRunDialog, setShowRunDialog] = useState(false)
  const [logPanelCollapsed, setLogPanelCollapsed] = useState(false)
  const [selectedPanelAgent, setSelectedPanelAgent] = useState<SelectedAgentData | null>(null)

  // WebSocket: subscribe to project-level events (empty ticket_id)
  const { subscribe, unsubscribe } = useWebSocket()
  useEffect(() => {
    if (projectsLoaded) {
      subscribe('')
      return () => unsubscribe('')
    }
  }, [projectsLoaded, currentProject, subscribe, unsubscribe])

  const { data: workflowData } = useProjectWorkflow(currentProject, {
    enabled: !!currentProject,
  })

  const { data: sessionsData } = useProjectAgentSessions(currentProject, {
    enabled: !!currentProject,
  })

  const stopMutation = useStopProjectWorkflow()
  const restartMutation = useRestartProjectAgent()

  const workflows = workflowData?.workflows ?? []
  const allWorkflows = (workflowData?.all_workflows ?? {}) as Record<string, WorkflowState>
  const hasWorkflow = workflowData?.has_workflow ?? false
  const hasMultipleWorkflows = workflows.length > 1

  const defaultState = (workflowData?.state ?? null) as WorkflowState | null
  const displayedWorkflowName = selectedWorkflow || defaultState?.workflow || workflows[0] || ''
  const displayedState = selectedWorkflow && allWorkflows[selectedWorkflow]
    ? allWorkflows[selectedWorkflow]
    : defaultState

  const activeAgents = displayedState?.active_agents ?? {}

  const orchestrationStatus = displayedState?.findings?.['_orchestration'] as
    | { status?: string }
    | undefined
  const isOrchestrated = orchestrationStatus?.status === 'running'

  const hasActivePhase = displayedState?.phases
    ? Object.values(displayedState.phases).some((p) => p.status === 'in_progress')
    : false

  return (
    <div className={
      hasActivePhase || selectedPanelAgent ? 'max-w-full px-4 space-y-6' : 'max-w-7xl mx-auto p-6 space-y-6'
    }>
      <div>
        <h1 className="text-2xl font-bold">Project Workflows</h1>
        <p className="text-sm text-muted-foreground mt-1">
          Workflows that run at project level without a ticket.
        </p>
      </div>

      <WorkflowTabContent
        ticketId={undefined}
        hasWorkflow={hasWorkflow}
        displayedState={displayedState}
        displayedWorkflowName={displayedWorkflowName}
        hasMultipleWorkflows={hasMultipleWorkflows}
        workflows={workflows}
        selectedWorkflow={selectedWorkflow}
        onSelectWorkflow={setSelectedWorkflow}
        isOrchestrated={isOrchestrated}
        hasActivePhase={hasActivePhase}
        activeAgents={activeAgents}
        sessions={sessionsData?.sessions ?? []}
        logPanelCollapsed={logPanelCollapsed}
        onToggleLogPanel={() => setLogPanelCollapsed((p) => !p)}
        selectedPanelAgent={selectedPanelAgent}
        onAgentSelect={setSelectedPanelAgent}
        onStop={() =>
          currentProject &&
          stopMutation.mutate({
            projectId: currentProject,
            workflow: displayedWorkflowName || undefined,
          })
        }
        stopPending={stopMutation.isPending}
        onShowRunDialog={() => setShowRunDialog(true)}
        onRestart={(sessionId) =>
          currentProject &&
          restartMutation.mutate({
            projectId: currentProject,
            params: { workflow: displayedWorkflowName, session_id: sessionId },
          })
        }
        restartingSessionId={
          restartMutation.isPending
            ? (restartMutation.variables?.params.session_id ?? null)
            : null
        }
      />

      {currentProject && (
        <RunProjectWorkflowDialog
          open={showRunDialog}
          onClose={() => setShowRunDialog(false)}
          projectId={currentProject}
        />
      )}
    </div>
  )
}
