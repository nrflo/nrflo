import { useState, useMemo, useEffect } from 'react'
import { WorkflowSubTabBar } from './WorkflowSubTabBar'
import type { WorkflowSubTab } from './WorkflowSubTabBar'
import { InstanceList } from './ProjectWorkflowComponents'
import { WorkflowTabContent } from './WorkflowTabContent'
import { TicketTraceSection } from './TicketTraceSection'
import { TicketCompletedSection } from './TicketCompletedSection'
import type { WorkflowResponse, WorkflowState, AgentSessionsResponse, CompletedAgentRow } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'
import {
  useStopWorkflow,
  useRetryFailedAgent,
  useTakeControl,
  useResumeSession,
  useContinueWorkflow,
  useFailWorkflow,
} from '@/hooks/useTickets'
import { useInteractiveSessionsStore } from '@/stores/interactiveSessionsStore'
import { LaunchObserverButton } from '@/components/observer/LaunchObserverButton'
import { useProjectStore } from '@/stores/projectStore'

interface TicketWorkflowTabProps {
  ticketId: string | undefined
  workflowData: WorkflowResponse | undefined
  sessionsData: AgentSessionsResponse | undefined
  issueType: string | undefined
  activeChainId: string | null
  onShowRunDialog: () => void
  onShowEpicRunDialog: () => void
  onExpandedChange: (expanded: boolean) => void
  projectFindings?: Record<string, unknown>
  blockedReason?: string
}

export function TicketWorkflowTab({
  ticketId,
  workflowData,
  sessionsData,
  issueType,
  activeChainId,
  onShowRunDialog,
  onShowEpicRunDialog,
  onExpandedChange,
  projectFindings,
  blockedReason,
}: TicketWorkflowTabProps) {
  const [activeSubTab, setActiveSubTab] = useState<WorkflowSubTab>('running')
  const [selectedInstanceId, setSelectedInstanceId] = useState('')
  const [logPanelCollapsed, setLogPanelCollapsed] = useState(false)
  const [selectedPanelAgent, setSelectedPanelAgent] = useState<SelectedAgentData | null>(null)

  const addSession = useInteractiveSessionsStore((s) => s.add)
  const currentProject = useProjectStore((s) => s.currentProject)

  const stopMutation = useStopWorkflow()
  const retryFailedMutation = useRetryFailedAgent()
  const takeControlMutation = useTakeControl()
  const resumeSessionMutation = useResumeSession()
  const continueMutation = useContinueWorkflow()
  const failMutation = useFailWorkflow()

  const workflows = workflowData?.workflows ?? []
  const allWorkflows = (workflowData?.all_workflows ?? {}) as Record<string, WorkflowState>
  const hasWorkflow = workflowData?.has_workflow ?? false

  // Partition instances by status (three-way: running/failed/completed)
  const { runningInstances, failedInstances, completedInstances } = useMemo(() => {
    const running: Record<string, WorkflowState> = {}
    const failed: Record<string, WorkflowState> = {}
    const completed: Record<string, WorkflowState> = {}
    for (const [instanceId, state] of Object.entries(allWorkflows)) {
      if (state.status === 'completed' || state.status === 'project_completed') {
        completed[instanceId] = state
      } else if (state.status === 'failed') {
        failed[instanceId] = state
      } else {
        running[instanceId] = state
      }
    }
    return { runningInstances: running, failedInstances: failed, completedInstances: completed }
  }, [allWorkflows])

  const totalInstances = Object.keys(allWorkflows).length
  const completedCount = Object.keys(completedInstances).length
  const failedCount = Object.keys(failedInstances).length
  const runningCount = Object.keys(runningInstances).length

  // Show sub-tabs whenever any instance exists (Trace must stay reachable)
  const showSubTabs = totalInstances > 0
  // Single non-running instance optimization: show it regardless of status tab
  const singleNonRunningOnly = totalInstances === 1 && runningCount === 0

  // Determine which instances to show based on sub-tab; Trace spans all instances
  const tabInstances = (!showSubTabs || singleNonRunningOnly || activeSubTab === 'trace')
    ? allWorkflows
    : activeSubTab === 'running' ? runningInstances
    : activeSubTab === 'failed' ? failedInstances
    : completedInstances

  const instanceIds = Object.keys(tabInstances)
  const hasMultipleInTab = instanceIds.length > 1

  const resolvedInstanceId = (selectedInstanceId && tabInstances[selectedInstanceId])
    ? selectedInstanceId
    : instanceIds[0] || ''
  const displayedState = resolvedInstanceId ? tabInstances[resolvedInstanceId] : null

  // Build display labels
  const { selectorLabels, displayedWorkflowName } = useMemo(() => {
    const labels: Record<string, string> = {}
    for (const iid of instanceIds) {
      const name = tabInstances[iid]?.workflow ?? iid
      const shortId = iid.substring(0, 8)
      labels[iid] = `${name} (#${shortId})`
    }
    return {
      selectorLabels: labels,
      displayedWorkflowName: displayedState?.workflow || workflows[0] || '',
    }
  }, [instanceIds, tabInstances, displayedState?.workflow, workflows])

  // Filter sessions by selected instance
  const sessions = useMemo(() => {
    if (!sessionsData?.sessions || !resolvedInstanceId) return sessionsData?.sessions ?? []
    return sessionsData.sessions.filter(s => s.workflow_instance_id === resolvedInstanceId)
  }, [sessionsData?.sessions, resolvedInstanceId])

  // Merged data for completed sub-tab
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

  // Report layout expansion to parent
  useEffect(() => {
    onExpandedChange(hasActivePhase || !!selectedPanelAgent)
  }, [hasActivePhase, selectedPanelAgent, onExpandedChange])

  const handleSubTabSwitch = (tab: WorkflowSubTab) => {
    setActiveSubTab(tab)
    setSelectedInstanceId('')
    setSelectedPanelAgent(null)
  }

  const pushToStore = (sessionId: string, agentType: string) => {
    if (!ticketId) return
    addSession({
      sessionId,
      agentType,
      scope: { type: 'ticket', ticketId },
      workflow: displayedWorkflowName,
      instanceId: resolvedInstanceId || undefined,
      startedAt: Date.now(),
    })
  }

  const handleResumeSession = (sessionId: string) => {
    if (!ticketId) return
    resumeSessionMutation.mutate(
      { ticketId, params: { session_id: sessionId } },
      { onSuccess: (data) => pushToStore(data.session_id, 'agent') }
    )
  }

  // --- Trace and Completed sub-tabs (shared bar + section body) ---
  const isTraceTab = showSubTabs && activeSubTab === 'trace'
  const isCompletedTab = showSubTabs && activeSubTab === 'completed'

  if (isTraceTab || isCompletedTab) {
    return (
      <>
        <WorkflowSubTabBar
          activeSubTab={activeSubTab}
          onSwitch={handleSubTabSwitch}
          runningCount={runningCount}
          failedCount={failedCount}
          completedCount={completedCount}
        />
        {isTraceTab ? (
          <TicketTraceSection
            instanceIds={instanceIds}
            instances={tabInstances}
            labels={selectorLabels}
            resolvedInstanceId={resolvedInstanceId}
            onSelectInstance={setSelectedInstanceId}
            displayedState={displayedState}
            sessions={sessions}
            activeAgents={activeAgents}
            selectedPanelAgent={selectedPanelAgent}
            onAgentSelect={setSelectedPanelAgent}
            logPanelCollapsed={logPanelCollapsed}
            onResumeSession={handleResumeSession}
            resumePending={resumeSessionMutation.isPending}
            projectFindings={projectFindings}
          />
        ) : (
          <TicketCompletedSection
            instanceIds={instanceIds}
            instances={tabInstances}
            labels={selectorLabels}
            resolvedInstanceId={resolvedInstanceId}
            onSelectInstance={setSelectedInstanceId}
            mergedCompletedAgents={mergedCompletedAgents}
            allCompletedSessions={allCompletedSessions}
            displayedState={displayedState}
            selectedPanelAgent={selectedPanelAgent}
            onAgentSelect={setSelectedPanelAgent}
            logPanelCollapsed={logPanelCollapsed}
            onResumeSession={handleResumeSession}
            resumePending={resumeSessionMutation.isPending}
            projectFindings={projectFindings}
          />
        )}
      </>
    )
  }

  // --- Running or Failed sub-tab (or single-instance view) ---
  const isFailedTab = activeSubTab === 'failed'
  return (
    <>
      {showSubTabs && (
        <WorkflowSubTabBar
          activeSubTab={activeSubTab}
          onSwitch={handleSubTabSwitch}
          runningCount={runningCount}
          failedCount={failedCount}
          completedCount={completedCount}
        />
      )}
      {hasMultipleInTab && (
        <InstanceList
          instanceIds={instanceIds}
          instances={tabInstances}
          labels={selectorLabels}
          selectedId={resolvedInstanceId}
          onSelect={setSelectedInstanceId}
          tab="running"
        />
      )}
      <WorkflowTabContent
        ticketId={ticketId}
        hasWorkflow={hasWorkflow}
        displayedState={displayedState}
        displayedWorkflowName={displayedWorkflowName}
        hasMultipleWorkflows={false}
        workflows={[]}
        selectedWorkflow=""
        onSelectWorkflow={() => {}}
        isOrchestrated={isFailedTab ? false : isOrchestrated}
        hasActivePhase={isFailedTab ? false : hasActivePhase}
        activeAgents={isFailedTab ? {} : activeAgents}
        sessions={sessions}
        logPanelCollapsed={logPanelCollapsed}
        onToggleLogPanel={() => setLogPanelCollapsed(p => !p)}
        selectedPanelAgent={selectedPanelAgent}
        onAgentSelect={setSelectedPanelAgent}
        onStop={isFailedTab ? () => {} : () =>
          ticketId && stopMutation.mutate({
            ticketId,
            workflow: displayedWorkflowName || undefined,
            instance_id: resolvedInstanceId || undefined,
          })
        }
        stopPending={isFailedTab ? false : stopMutation.isPending}
        issueType={issueType}
        onShowRunDialog={onShowRunDialog}
        onShowEpicRunDialog={onShowEpicRunDialog}
        activeChainId={activeChainId}
        onRetryFailed={(sessionId) =>
          ticketId && retryFailedMutation.mutate({
            ticketId,
            params: { workflow: displayedWorkflowName, session_id: sessionId, instance_id: resolvedInstanceId || undefined },
          })
        }
        retryingSessionId={retryFailedMutation.isPending ? (retryFailedMutation.variables?.params.session_id ?? null) : null}
        onTakeControl={isFailedTab ? () => {} : (sessionId) => {
          if (!ticketId) return
          const agent = Object.values(activeAgents).find((a) => a.session_id === sessionId)
          takeControlMutation.mutate(
            { ticketId, params: { workflow: displayedWorkflowName, session_id: sessionId, instance_id: resolvedInstanceId || undefined } },
            { onSuccess: (data) => pushToStore(data.session_id, agent?.agent_type ?? 'agent') }
          )
        }}
        takeControlPending={isFailedTab ? false : takeControlMutation.isPending}
        onResumeSession={handleResumeSession}
        resumeSessionPending={resumeSessionMutation.isPending}
        projectFindings={projectFindings}
        blockedReason={blockedReason}
        headerExtra={
          <LaunchObserverButton
            payload={{ scope: 'workflow', project_id: currentProject, workflow_id: displayedWorkflowName }}
          />
        }
        onContinue={(instructions) =>
          ticketId && continueMutation.mutate({
            ticketId,
            params: { workflow: displayedWorkflowName || undefined, instructions: instructions || undefined },
          })
        }
        continuePending={continueMutation.isPending}
        onFail={(reason) =>
          ticketId && failMutation.mutate({
            ticketId,
            params: { workflow: displayedWorkflowName || undefined, reason },
          })
        }
        failPending={failMutation.isPending}
      />
    </>
  )
}
