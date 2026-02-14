import { useState, useEffect, useMemo } from 'react'
import { CheckCircle } from 'lucide-react'
import { useProjectStore } from '@/stores/projectStore'
import { useWebSocket } from '@/hooks/useWebSocket'
import {
  useProjectWorkflow,
  useProjectAgentSessions,
  useStopProjectWorkflow,
  useRestartProjectAgent,
  useRetryFailedProjectAgent,
} from '@/hooks/useTickets'
import { RunProjectWorkflowDialog } from '@/components/workflow/RunProjectWorkflowDialog'
import { WorkflowTabContent } from './WorkflowTabContent'
import { cn } from '@/lib/utils'
import type { WorkflowState } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'

export function ProjectWorkflowsPage() {
  const currentProject = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)

  const [activeTab, setActiveTab] = useState<'active' | 'completed'>('active')
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
  const retryFailedMutation = useRetryFailedProjectAgent()

  // all_workflows is keyed by instance_id; each state has .workflow for the workflow name
  const allWorkflows = (workflowData?.all_workflows ?? {}) as Record<string, WorkflowState>

  const { activeWorkflows, completedWorkflows } = useMemo(() => {
    const active: Record<string, WorkflowState> = {}
    const completed: Record<string, WorkflowState> = {}
    for (const [instanceId, state] of Object.entries(allWorkflows)) {
      if (state.status === 'completed' || state.status === 'project_completed') {
        completed[instanceId] = state
      } else {
        active[instanceId] = state
      }
    }
    return { activeWorkflows: active, completedWorkflows: completed }
  }, [allWorkflows])

  const tabWorkflows = activeTab === 'active' ? activeWorkflows : completedWorkflows
  const instanceIds = Object.keys(tabWorkflows)
  const hasWorkflow = instanceIds.length > 0
  const hasMultipleWorkflows = instanceIds.length > 1

  const defaultState = instanceIds.length > 0 ? tabWorkflows[instanceIds[0]] : null
  const selectedInstanceId = (selectedWorkflow && tabWorkflows[selectedWorkflow])
    ? selectedWorkflow
    : instanceIds[0] || ''
  const displayedState = (selectedWorkflow && tabWorkflows[selectedWorkflow])
    ? tabWorkflows[selectedWorkflow]
    : defaultState

  // Build display labels for the selector: "workflow-name" or "workflow-name (#short-id)" when duplicates
  const { selectorLabels, displayedWorkflowName } = useMemo(() => {
    const nameCount: Record<string, number> = {}
    for (const id of instanceIds) {
      const name = tabWorkflows[id]?.workflow ?? id
      nameCount[name] = (nameCount[name] ?? 0) + 1
    }
    const labels: Record<string, string> = {}
    const nameIndex: Record<string, number> = {}
    for (const id of instanceIds) {
      const name = tabWorkflows[id]?.workflow ?? id
      if (nameCount[name] > 1) {
        nameIndex[name] = (nameIndex[name] ?? 0) + 1
        labels[id] = `${name} (${nameIndex[name]})`
      } else {
        labels[id] = name
      }
    }
    return {
      selectorLabels: labels,
      displayedWorkflowName: labels[selectedInstanceId] ?? '',
    }
  }, [instanceIds, tabWorkflows, selectedInstanceId])

  const activeAgents = displayedState?.active_agents ?? {}

  const orchestrationStatus = displayedState?.findings?.['_orchestration'] as
    | { status?: string }
    | undefined
  const isOrchestrated = orchestrationStatus?.status === 'running'

  const hasActivePhase = displayedState?.phases
    ? Object.values(displayedState.phases).some((p) => p.status === 'in_progress')
    : false

  const activeCount = Object.keys(activeWorkflows).length
  const completedCount = Object.keys(completedWorkflows).length

  const handleTabSwitch = (tab: 'active' | 'completed') => {
    setActiveTab(tab)
    setSelectedWorkflow('')
    setSelectedPanelAgent(null)
  }

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

      <div className="border-b border-border">
        <div className="flex gap-1">
          <button
            onClick={() => handleTabSwitch('active')}
            className={cn(
              'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === 'active'
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            )}
          >
            Active ({activeCount})
          </button>
          <button
            onClick={() => handleTabSwitch('completed')}
            className={cn(
              'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === 'completed'
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            )}
          >
            <CheckCircle className="h-4 w-4" />
            Completed ({completedCount})
          </button>
        </div>
      </div>

      <WorkflowTabContent
        ticketId={undefined}
        hasWorkflow={hasWorkflow}
        displayedState={displayedState}
        displayedWorkflowName={displayedWorkflowName}
        hasMultipleWorkflows={hasMultipleWorkflows}
        workflows={instanceIds}
        workflowLabels={selectorLabels}
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
            params: {
              workflow: displayedState?.workflow || undefined,
              instance_id: selectedInstanceId || undefined,
            },
          })
        }
        stopPending={stopMutation.isPending}
        onShowRunDialog={activeTab === 'active' ? () => setShowRunDialog(true) : undefined}
        onRestart={(sessionId) =>
          currentProject &&
          restartMutation.mutate({
            projectId: currentProject,
            params: {
              workflow: displayedState?.workflow ?? '',
              session_id: sessionId,
              instance_id: selectedInstanceId || undefined,
            },
          })
        }
        restartingSessionId={
          restartMutation.isPending
            ? (restartMutation.variables?.params.session_id ?? null)
            : null
        }
        onRetryFailed={(sessionId) =>
          currentProject &&
          retryFailedMutation.mutate({
            projectId: currentProject,
            params: {
              workflow: displayedState?.workflow ?? '',
              session_id: sessionId,
              instance_id: selectedInstanceId || undefined,
            },
          })
        }
        retryingSessionId={
          retryFailedMutation.isPending
            ? (retryFailedMutation.variables?.params.session_id ?? null)
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
