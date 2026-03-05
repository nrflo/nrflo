import { useState, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle, Play } from 'lucide-react'
import { useProjectStore } from '@/stores/projectStore'
import {
  useProjectWorkflow,
  useProjectAgentSessions,
  useRunProjectWorkflow,
  useStopProjectWorkflow,
  useRetryFailedProjectAgent,
  useTakeControlProject,
  useExitInteractiveProject,
  useResumeSessionProject,
} from '@/hooks/useTickets'
import { listWorkflowDefs } from '@/api/workflows'
import { WorkflowTabContent } from './WorkflowTabContent'
import { RunWorkflowForm, InstanceList } from './ProjectWorkflowComponents'
import { CompletedAgentsTable } from '@/components/workflow/CompletedAgentsTable'
import { AgentLogPanel } from '@/components/workflow/AgentLogPanel'
import { AgentTerminalDialog } from '@/components/workflow/AgentTerminalDialog'
import { cn } from '@/lib/utils'
import type { WorkflowState, CompletedAgentRow } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'

type TabId = 'run' | 'running' | 'completed'

export function ProjectWorkflowsPage() {
  const currentProject = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)

  const [activeTab, setActiveTab] = useState<TabId>('run')
  const [selectedInstanceId, setSelectedInstanceId] = useState('')
  const [logPanelCollapsed, setLogPanelCollapsed] = useState(false)
  const [selectedPanelAgent, setSelectedPanelAgent] = useState<SelectedAgentData | null>(null)
  const [interactiveSession, setInteractiveSession] = useState<{ sessionId: string; agentType: string } | null>(null)

  // Run Workflow form state
  const [selectedWorkflowDef, setSelectedWorkflowDef] = useState('')
  const [instructions, setInstructions] = useState('')

  // WebSocket subscription is handled by WebSocketProvider (project-wide)

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
  const takeControlMutation = useTakeControlProject()
  const resumeSessionMutation = useResumeSessionProject()
  const exitInteractiveMutation = useExitInteractiveProject()

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

  const filteredSessions = useMemo(() => {
    if (!sessionsData?.sessions || !resolvedInstanceId) return sessionsData?.sessions ?? []
    return sessionsData.sessions.filter(s => s.workflow_instance_id === resolvedInstanceId)
  }, [sessionsData?.sessions, resolvedInstanceId])

  // Merged data for completed tab: flat array of all completed agents + all their sessions
  const mergedCompletedAgents = useMemo<CompletedAgentRow[]>(() => {
    const rows: CompletedAgentRow[] = []
    for (const [instanceId, state] of Object.entries(completedInstances)) {
      const label = selectorLabels[instanceId] ?? instanceId.substring(0, 8)
      for (const entry of state.agent_history ?? []) {
        rows.push({ ...entry, workflow_label: label })
      }
    }
    return rows
  }, [completedInstances, selectorLabels])

  const allCompletedSessions = useMemo(() => {
    if (!sessionsData?.sessions) return []
    const completedIds = new Set(Object.keys(completedInstances))
    return sessionsData.sessions.filter(s => completedIds.has(s.workflow_instance_id))
  }, [sessionsData?.sessions, completedInstances])

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

  const handleRun = async (startMode: 'normal' | 'interactive' | 'plan' = 'normal') => {
    if (!selectedWorkflowDef || !currentProject) return
    try {
      const result = await runMutation.mutateAsync({
        projectId: currentProject,
        params: {
          workflow: selectedWorkflowDef,
          instructions: instructions || undefined,
          ...(startMode === 'interactive' && { interactive: true }),
          ...(startMode === 'plan' && { plan_mode: true }),
        },
      })

      if ((startMode === 'interactive' || startMode === 'plan') && result.session_id) {
        setInteractiveSession({
          sessionId: result.session_id,
          agentType: startMode === 'plan' ? 'planner' : selectedWorkflowDef,
        })
      }

      setInstructions('')
      setSelectedInstanceId(result.instance_id)
      setActiveTab('running')
    } catch {
      // Error handled by mutation state
    }
  }

  return (
    <div className="max-w-full px-4 space-y-6">
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

      {activeTab === 'completed' && (
        <div className={cn(
          'flex gap-0',
          selectedPanelAgent && 'min-h-[calc(100vh-280px)]'
        )}>
          <div className="flex-1 min-w-0 space-y-4">
            {mergedCompletedAgents.length > 0 ? (
              <CompletedAgentsTable
                agentHistory={mergedCompletedAgents}
                sessions={allCompletedSessions}
                onAgentSelect={setSelectedPanelAgent}
                showWorkflowColumn
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
              onToggleCollapse={() => setLogPanelCollapsed((p) => !p)}
              selectedAgent={selectedPanelAgent}
              onAgentSelect={setSelectedPanelAgent}
              onResumeSession={(sessionId) => {
                if (!currentProject) return
                resumeSessionMutation.mutate(
                  { projectId: currentProject, params: { session_id: sessionId } },
                  { onSuccess: (data) => setInteractiveSession({ sessionId: data.session_id, agentType: 'agent' }) }
                )
              }}
              resumePending={resumeSessionMutation.isPending}
            />
          )}
        </div>
      )}

      {activeTab === 'running' && (
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
            sessions={filteredSessions}
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
            onTakeControl={(sessionId) => {
              if (!currentProject) return
              const agent = Object.values(activeAgents).find((a) => a.session_id === sessionId)
              takeControlMutation.mutate(
                {
                  projectId: currentProject,
                  params: {
                    workflow: displayedState?.workflow ?? '',
                    session_id: sessionId,
                    instance_id: resolvedInstanceId || undefined,
                  },
                },
                { onSuccess: (data) => setInteractiveSession({ sessionId: data.session_id, agentType: agent?.agent_type ?? 'agent' }) }
              )
            }}
            takeControlPending={takeControlMutation.isPending}
            onResumeSession={(sessionId) => {
              if (!currentProject) return
              resumeSessionMutation.mutate(
                { projectId: currentProject, params: { session_id: sessionId } },
                { onSuccess: (data) => setInteractiveSession({ sessionId: data.session_id, agentType: 'agent' }) }
              )
            }}
            resumeSessionPending={resumeSessionMutation.isPending}
          />
        </>
      )}

      {/* Interactive Terminal Dialog */}
      {interactiveSession && currentProject && (
        <AgentTerminalDialog
          open={!!interactiveSession}
          onClose={() => setInteractiveSession(null)}
          onExitSession={() => {
            exitInteractiveMutation.mutate(
              {
                projectId: currentProject,
                params: {
                  workflow: displayedState?.workflow ?? '',
                  session_id: interactiveSession.sessionId,
                  instance_id: resolvedInstanceId || undefined,
                },
              },
              { onSuccess: () => setInteractiveSession(null) }
            )
          }}
          exitPending={exitInteractiveMutation.isPending}
          sessionId={interactiveSession.sessionId}
          agentType={interactiveSession.agentType}
        />
      )}
    </div>
  )
}
