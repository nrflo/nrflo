import { useState, useMemo, useEffect } from 'react'
import { cn } from '@/lib/utils'
import { WorkflowSubTabBar } from './WorkflowSubTabBar'
import type { WorkflowSubTab } from './WorkflowSubTabBar'
import { InstanceList } from './ProjectWorkflowComponents'
import { CompletedAgentsTable } from '@/components/workflow/CompletedAgentsTable'
import { AgentLogPanel } from '@/components/workflow/AgentLogPanel'
import { AgentTerminalDialog } from '@/components/workflow/AgentTerminalDialog'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowResponse, WorkflowState, AgentSessionsResponse, CompletedAgentRow } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'
import {
  useStopWorkflow,
  useRetryFailedAgent,
  useTakeControl,
  useExitInteractive,
  useResumeSession,
} from '@/hooks/useTickets'

interface TicketWorkflowTabProps {
  ticketId: string | undefined
  workflowData: WorkflowResponse | undefined
  sessionsData: AgentSessionsResponse | undefined
  issueType: string | undefined
  activeChainId: string | null
  interactiveSession: { sessionId: string; agentType: string } | null
  onInteractiveStart: (session: { sessionId: string; agentType: string }) => void
  onInteractiveEnd: () => void
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
  interactiveSession,
  onInteractiveStart,
  onInteractiveEnd,
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

  const stopMutation = useStopWorkflow()
  const retryFailedMutation = useRetryFailedAgent()
  const takeControlMutation = useTakeControl()
  const exitInteractiveMutation = useExitInteractive()
  const resumeSessionMutation = useResumeSession()

  const workflows = workflowData?.workflows ?? []
  const allWorkflows = (workflowData?.all_workflows ?? {}) as Record<string, WorkflowState>
  const hasWorkflow = workflowData?.has_workflow ?? false

  // Partition instances by status
  const { runningInstances, completedInstances } = useMemo(() => {
    const running: Record<string, WorkflowState> = {}
    const completed: Record<string, WorkflowState> = {}
    for (const [instanceId, state] of Object.entries(allWorkflows)) {
      if (state.status === 'completed' || state.status === 'project_completed') {
        completed[instanceId] = state
      } else {
        // undefined status, 'failed', or any active status → running bucket
        running[instanceId] = state
      }
    }
    return { runningInstances: running, completedInstances: completed }
  }, [allWorkflows])

  const totalInstances = Object.keys(allWorkflows).length
  const completedCount = Object.keys(completedInstances).length
  const runningCount = Object.keys(runningInstances).length

  // Show sub-tabs when multiple instances or any completed
  const showSubTabs = totalInstances > 1 || completedCount > 0
  // Single completed instance optimization: skip sub-tabs
  const singleCompletedOnly = totalInstances === 1 && completedCount === 1

  // Determine which instances to show based on sub-tab
  const tabInstances = (!showSubTabs || singleCompletedOnly)
    ? allWorkflows
    : activeSubTab === 'running' ? runningInstances : completedInstances

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

  // --- Completed sub-tab rendering ---
  const isCompletedTab = showSubTabs && !singleCompletedOnly && activeSubTab === 'completed'

  if (isCompletedTab) {
    return (
      <>
        <WorkflowSubTabBar
          activeSubTab={activeSubTab}
          onSwitch={handleSubTabSwitch}
          runningCount={runningCount}
          completedCount={completedCount}
        />
        <div className={cn(
          'flex gap-0',
          selectedPanelAgent && 'min-h-[calc(100vh-280px)]'
        )}>
          <div className="flex-1 min-w-0 space-y-4">
            {instanceIds.length > 0 && (
              <InstanceList
                instanceIds={instanceIds}
                instances={tabInstances}
                labels={selectorLabels}
                selectedId={resolvedInstanceId}
                onSelect={setSelectedInstanceId}
                tab="completed"
              />
            )}
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
              selectedAgent={selectedPanelAgent}
              onAgentSelect={setSelectedPanelAgent}
              onResumeSession={(sessionId) => {
                if (!ticketId) return
                resumeSessionMutation.mutate(
                  { ticketId, params: { session_id: sessionId } },
                  { onSuccess: (data) => onInteractiveStart({ sessionId: data.session_id, agentType: 'agent' }) }
                )
              }}
              resumePending={resumeSessionMutation.isPending}
              agentFindings={displayedState?.findings}
              projectFindings={projectFindings}
            />
          )}
        </div>
        {renderTerminalDialog()}
      </>
    )
  }

  // --- Running sub-tab (or single-instance view) ---
  return (
    <>
      {showSubTabs && !singleCompletedOnly && (
        <WorkflowSubTabBar
          activeSubTab={activeSubTab}
          onSwitch={handleSubTabSwitch}
          runningCount={runningCount}
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
        isOrchestrated={isOrchestrated}
        hasActivePhase={hasActivePhase}
        activeAgents={activeAgents}
        sessions={sessions}
        logPanelCollapsed={logPanelCollapsed}
        onToggleLogPanel={() => setLogPanelCollapsed(p => !p)}
        selectedPanelAgent={selectedPanelAgent}
        onAgentSelect={setSelectedPanelAgent}
        onStop={() =>
          ticketId && stopMutation.mutate({
            ticketId,
            workflow: displayedWorkflowName || undefined,
            instance_id: resolvedInstanceId || undefined,
          })
        }
        stopPending={stopMutation.isPending}
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
        onTakeControl={(sessionId) => {
          if (!ticketId) return
          const agent = Object.values(activeAgents).find((a) => a.session_id === sessionId)
          takeControlMutation.mutate(
            { ticketId, params: { workflow: displayedWorkflowName, session_id: sessionId, instance_id: resolvedInstanceId || undefined } },
            { onSuccess: (data) => onInteractiveStart({ sessionId: data.session_id, agentType: agent?.agent_type ?? 'agent' }) }
          )
        }}
        takeControlPending={takeControlMutation.isPending}
        onResumeSession={(sessionId) => {
          if (!ticketId) return
          resumeSessionMutation.mutate(
            { ticketId, params: { session_id: sessionId } },
            { onSuccess: (data) => onInteractiveStart({ sessionId: data.session_id, agentType: 'agent' }) }
          )
        }}
        resumeSessionPending={resumeSessionMutation.isPending}
        projectFindings={projectFindings}
        blockedReason={blockedReason}
      />
      {renderTerminalDialog()}
    </>
  )

  function renderTerminalDialog() {
    if (!interactiveSession || !ticketId) return null
    return (
      <AgentTerminalDialog
        open={!!interactiveSession}
        onClose={onInteractiveEnd}
        onExitSession={() => {
          exitInteractiveMutation.mutate(
            { ticketId, params: { workflow: displayedWorkflowName, session_id: interactiveSession.sessionId, instance_id: resolvedInstanceId || undefined } },
            { onSuccess: onInteractiveEnd }
          )
        }}
        exitPending={exitInteractiveMutation.isPending}
        sessionId={interactiveSession.sessionId}
        agentType={interactiveSession.agentType}
      />
    )
  }
}

