import { useState, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle, Play } from 'lucide-react'
import { useProjectStore } from '@/stores/projectStore'
import { useWebSocket } from '@/hooks/useWebSocket'
import {
  useProjectWorkflow,
  useProjectAgentSessions,
  useRunProjectWorkflow,
  useStopProjectWorkflow,
  useRetryFailedProjectAgent,
} from '@/hooks/useTickets'
import { listWorkflowDefs } from '@/api/workflows'
import { WorkflowTabContent } from './WorkflowTabContent'
import { RunWorkflowForm, InstanceList } from './ProjectWorkflowComponents'
import { cn } from '@/lib/utils'
import type { WorkflowState } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'

type TabId = 'run' | 'running' | 'completed'

export function ProjectWorkflowsPage() {
  const currentProject = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)

  const [activeTab, setActiveTab] = useState<TabId>('run')
  const [selectedInstanceId, setSelectedInstanceId] = useState('')
  const [logPanelCollapsed, setLogPanelCollapsed] = useState(false)
  const [selectedPanelAgent, setSelectedPanelAgent] = useState<SelectedAgentData | null>(null)

  // Run Workflow form state
  const [selectedWorkflowDef, setSelectedWorkflowDef] = useState('')
  const [instructions, setInstructions] = useState('')

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

  const { data: workflowDefs, isLoading: defsLoading } = useQuery({
    queryKey: ['workflows', 'defs', currentProject],
    queryFn: listWorkflowDefs,
    enabled: projectsLoaded,
  })

  const runMutation = useRunProjectWorkflow()
  const stopMutation = useStopProjectWorkflow()
  const retryFailedMutation = useRetryFailedProjectAgent()

  // Filter to project-scoped workflows only
  const projectWorkflows = workflowDefs
    ? Object.entries(workflowDefs).filter(([, def]) => def.scope_type === 'project')
    : []

  // Auto-select first workflow def
  useEffect(() => {
    if (projectWorkflows.length > 0 && !selectedWorkflowDef) {
      setSelectedWorkflowDef(projectWorkflows[0][0])
    }
  }, [projectWorkflows, selectedWorkflowDef])

  // all_workflows keyed by instance_id
  const allWorkflows = (workflowData?.all_workflows ?? {}) as Record<string, WorkflowState>

  const { runningInstances, completedInstances } = useMemo(() => {
    const running: Record<string, WorkflowState> = {}
    const completed: Record<string, WorkflowState> = {}
    for (const [instanceId, state] of Object.entries(allWorkflows)) {
      if (state.status === 'completed' || state.status === 'project_completed') {
        completed[instanceId] = state
      } else {
        running[instanceId] = state
      }
    }
    return { runningInstances: running, completedInstances: completed }
  }, [allWorkflows])

  const tabInstances = activeTab === 'running' ? runningInstances : completedInstances
  const instanceIds = Object.keys(tabInstances)
  const hasWorkflow = instanceIds.length > 0
  const hasMultipleWorkflows = instanceIds.length > 1

  const defaultState = instanceIds.length > 0 ? tabInstances[instanceIds[0]] : null
  const resolvedInstanceId = (selectedInstanceId && tabInstances[selectedInstanceId])
    ? selectedInstanceId
    : instanceIds[0] || ''
  const displayedState = (selectedInstanceId && tabInstances[selectedInstanceId])
    ? tabInstances[selectedInstanceId]
    : defaultState

  // Build display labels: "workflow-name (#short-id)"
  const { selectorLabels, displayedWorkflowName } = useMemo(() => {
    const labels: Record<string, string> = {}
    for (const id of instanceIds) {
      const name = tabInstances[id]?.workflow ?? id
      const shortId = id.substring(0, 8)
      labels[id] = `${name} (#${shortId})`
    }
    return {
      selectorLabels: labels,
      displayedWorkflowName: labels[resolvedInstanceId] ?? '',
    }
  }, [instanceIds, tabInstances, resolvedInstanceId])

  const activeAgents = displayedState?.active_agents ?? {}

  const orchestrationStatus = displayedState?.findings?.['_orchestration'] as
    | { status?: string }
    | undefined
  const isOrchestrated = orchestrationStatus?.status === 'running'

  const hasActivePhase = displayedState?.phases
    ? Object.values(displayedState.phases).some((p) => p.status === 'in_progress')
    : false

  const runningCount = Object.keys(runningInstances).length
  const completedCount = Object.keys(completedInstances).length

  const handleTabSwitch = (tab: TabId) => {
    setActiveTab(tab)
    setSelectedInstanceId('')
    setSelectedPanelAgent(null)
  }

  const handleRun = async () => {
    if (!selectedWorkflowDef || !currentProject) return
    try {
      const result = await runMutation.mutateAsync({
        projectId: currentProject,
        params: {
          workflow: selectedWorkflowDef,
          instructions: instructions || undefined,
        },
      })
      setInstructions('')
      setSelectedInstanceId(result.instance_id)
      setActiveTab('running')
    } catch {
      // Error handled by mutation state
    }
  }

  return (
    <div className={
      (activeTab !== 'run' && (hasActivePhase || selectedPanelAgent))
        ? 'max-w-full px-4 space-y-6'
        : 'max-w-7xl mx-auto p-6 space-y-6'
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
            onClick={() => handleTabSwitch('run')}
            className={cn(
              'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === 'run'
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            )}
          >
            <Play className="h-4 w-4" />
            Run Workflow
          </button>
          <button
            onClick={() => handleTabSwitch('running')}
            className={cn(
              'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === 'running'
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            )}
          >
            Running ({runningCount})
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

      {activeTab === 'run' && (
        <RunWorkflowForm
          projectWorkflows={projectWorkflows}
          defsLoading={defsLoading}
          selectedWorkflowDef={selectedWorkflowDef}
          onSelectWorkflowDef={setSelectedWorkflowDef}
          instructions={instructions}
          onInstructionsChange={setInstructions}
          onRun={handleRun}
          runPending={runMutation.isPending}
          runError={runMutation.isError ? runMutation.error : null}
        />
      )}

      {(activeTab === 'running' || activeTab === 'completed') && (
        <>
          {instanceIds.length > 0 && (
            <InstanceList
              instanceIds={instanceIds}
              instances={tabInstances}
              labels={selectorLabels}
              selectedId={resolvedInstanceId}
              onSelect={setSelectedInstanceId}
              tab={activeTab}
            />
          )}
          <WorkflowTabContent
            ticketId={undefined}
            hasWorkflow={hasWorkflow}
            displayedState={displayedState}
            displayedWorkflowName={displayedWorkflowName}
            hasMultipleWorkflows={hasMultipleWorkflows}
            workflows={instanceIds}
            workflowLabels={selectorLabels}
            selectedWorkflow={selectedInstanceId}
            onSelectWorkflow={setSelectedInstanceId}
            isOrchestrated={isOrchestrated}
            hasActivePhase={hasActivePhase}
            activeAgents={activeAgents}
            sessions={sessionsData?.sessions ?? []}
            logPanelCollapsed={logPanelCollapsed}
            onToggleLogPanel={() => setLogPanelCollapsed((p) => !p)}
            selectedPanelAgent={selectedPanelAgent}
            onAgentSelect={setSelectedPanelAgent}
            isCompletedProjectWorkflow={activeTab === 'completed'}
            onStop={() =>
              currentProject &&
              stopMutation.mutate({
                projectId: currentProject,
                params: {
                  workflow: displayedState?.workflow || undefined,
                  instance_id: resolvedInstanceId || undefined,
                },
              })
            }
            stopPending={stopMutation.isPending}
            onRetryFailed={(sessionId) =>
              currentProject &&
              retryFailedMutation.mutate({
                projectId: currentProject,
                params: {
                  workflow: displayedState?.workflow ?? '',
                  session_id: sessionId,
                  instance_id: resolvedInstanceId || undefined,
                },
              })
            }
            retryingSessionId={
              retryFailedMutation.isPending
                ? (retryFailedMutation.variables?.params.session_id ?? null)
                : null
            }
          />
        </>
      )}
    </div>
  )
}
