import { useState } from 'react'
import { Link } from 'react-router-dom'
import {
  CheckCircle,
  ChevronLeft,
  ChevronRight,
  Clock,
  ExternalLink,
  Layers,
  Play,
  Square,
  Terminal,
  Zap,
  XCircle,
  RefreshCw,
} from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { Dropdown } from '@/components/ui/Dropdown'
import { Tooltip } from '@/components/ui/Tooltip'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { PhaseTimeline } from '@/components/workflow/PhaseTimeline'
import { AgentLogPanel } from '@/components/workflow/AgentLogPanel'
import type { WorkflowState, AgentSession, ActiveAgentV4 } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'
import { cn, formatDateTime, formatDurationSec, formatTokenCount } from '@/lib/utils'

interface WorkflowTabContentProps {
  ticketId: string | undefined
  hasWorkflow: boolean
  displayedState: WorkflowState | null
  displayedWorkflowName: string
  hasMultipleWorkflows: boolean
  workflows: string[]
  workflowLabels?: Record<string, string>  // optional display labels for workflow selector options
  selectedWorkflow: string
  onSelectWorkflow: (wf: string) => void
  isOrchestrated: boolean
  hasActivePhase: boolean
  activeAgents: Record<string, ActiveAgentV4>
  sessions: AgentSession[]
  logPanelCollapsed: boolean
  onToggleLogPanel: () => void
  selectedPanelAgent: SelectedAgentData | null
  onAgentSelect: (data: SelectedAgentData | null) => void
  onStop: () => void
  stopPending: boolean
  issueType?: string
  onShowRunDialog?: () => void
  onShowEpicRunDialog?: () => void
  activeChainId?: string | null
  onRetryFailed?: (sessionId: string) => void
  retryingSessionId?: string | null
  onTakeControl?: (sessionId: string) => void
  takeControlPending?: boolean
  onResumeSession?: (sessionId: string) => void
  resumeSessionPending?: boolean
}

export function WorkflowTabContent({
  ticketId,
  hasWorkflow,
  displayedState,
  displayedWorkflowName,
  hasMultipleWorkflows,
  workflows,
  workflowLabels,
  selectedWorkflow,
  onSelectWorkflow,
  isOrchestrated,
  hasActivePhase,
  activeAgents,
  sessions,
  logPanelCollapsed,
  onToggleLogPanel,
  selectedPanelAgent,
  onAgentSelect,
  onStop,
  stopPending,
  issueType,
  onShowRunDialog,
  onShowEpicRunDialog,
  activeChainId,
  onRetryFailed,
  retryingSessionId,
  onTakeControl,
  takeControlPending,
  onResumeSession,
  resumeSessionPending,
}: WorkflowTabContentProps) {
  const agentHistory = displayedState?.agent_history
  const [bannerConfirmOpen, setBannerConfirmOpen] = useState(false)
  const failedAgent = agentHistory?.find(a => a.result === 'fail')

  // Find a running Claude agent for Take Control
  const runningClaudeAgent = Object.values(activeAgents).find(
    (a) => !a.result && a.cli === 'claude' && a.session_id
  )
  // Use selected panel agent if it's running and claude, else fallback
  const takeControlTarget =
    selectedPanelAgent?.agent && !selectedPanelAgent.agent.result && selectedPanelAgent.agent.cli === 'claude' && selectedPanelAgent.agent.session_id
      ? selectedPanelAgent.agent
      : runningClaudeAgent

  return (
    <div className={cn(
      'flex flex-col md:flex-row gap-0',
      (hasActivePhase || selectedPanelAgent) && 'md:min-h-[calc(100vh-280px)]'
    )}>
      <div className="flex-1 min-w-0 space-y-4">
        {hasWorkflow && displayedState ? (
          <>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                {hasMultipleWorkflows ? (
                  <Dropdown
                    value={selectedWorkflow || workflows[0] || ''}
                    onChange={onSelectWorkflow}
                    className="w-48 h-8 text-sm"
                    options={workflows.map((wf) => ({
                      value: wf,
                      label: workflowLabels?.[wf] ?? wf,
                    }))}
                  />
                ) : displayedWorkflowName ? (
                  <Badge variant="secondary">{displayedWorkflowName}</Badge>
                ) : null}
                {isOrchestrated && (
                  <Badge className="bg-yellow-500/20 text-yellow-600 dark:text-yellow-400 border-yellow-500/30">
                    Auto
                  </Badge>
                )}
                {(isOrchestrated || hasActivePhase) && (
                  <>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={onStop}
                      disabled={stopPending}
                      className="text-destructive hover:text-destructive"
                    >
                      {stopPending ? (
                        <Spinner size="sm" className="mr-2" />
                      ) : (
                        <Square className="h-4 w-4 mr-2" />
                      )}
                      Stop
                    </Button>
                    {onTakeControl && takeControlTarget?.session_id && (
                      <Tooltip text="Take interactive control of agent" placement="top">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => onTakeControl(takeControlTarget.session_id!)}
                          disabled={takeControlPending}
                          className="text-blue-600 hover:text-blue-700"
                        >
                          {takeControlPending ? (
                            <Spinner size="sm" className="mr-2" />
                          ) : (
                            <Terminal className="h-4 w-4 mr-2" />
                          )}
                          Take Control
                        </Button>
                      </Tooltip>
                    )}
                  </>
                )}
                {(hasActivePhase || selectedPanelAgent) && (
                  <Tooltip text={logPanelCollapsed ? 'Expand agent log' : 'Collapse agent log'} placement="top">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={onToggleLogPanel}
                      title={logPanelCollapsed ? 'Expand agent log' : 'Collapse agent log'}
                    >
                      {logPanelCollapsed ? <ChevronRight className="h-4 w-4" /> : <ChevronLeft className="h-4 w-4" />}
                    </Button>
                  </Tooltip>
                )}
              </div>
              {!(isOrchestrated || hasActivePhase) && (
                <>
                  {activeChainId ? (
                    <Link to={`/chains/${encodeURIComponent(activeChainId)}`}>
                      <Button variant="outline" size="sm">
                        <ExternalLink className="h-4 w-4 mr-2" />
                        View Chain
                      </Button>
                    </Link>
                  ) : issueType === 'epic' && onShowEpicRunDialog ? (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={onShowEpicRunDialog}
                    >
                      <Layers className="h-4 w-4 mr-2" />
                      Run Epic Workflow
                    </Button>
                  ) : onShowRunDialog ? (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={onShowRunDialog}
                    >
                      <Play className="h-4 w-4 mr-2" />
                      Run Workflow
                    </Button>
                  ) : null}
                </>
              )}
            </div>
            {displayedState.status === 'failed' && onRetryFailed && failedAgent?.session_id && (
              <div className="flex items-center gap-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm dark:border-red-800 dark:bg-red-950/30">
                <div className="flex items-center gap-2 text-red-700 dark:text-red-400">
                  <XCircle className="h-4 w-4" />
                  <span className="font-medium">Workflow Failed</span>
                </div>
                <Tooltip text="Retry the failed layer — all agents in that layer will be re-run" placement="top">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setBannerConfirmOpen(true)}
                    disabled={!!retryingSessionId}
                    className="ml-auto text-red-600 hover:text-red-700 border-red-300 dark:border-red-700"
                  >
                    {retryingSessionId ? (
                      <Spinner size="sm" className="mr-2" />
                    ) : (
                      <RefreshCw className="h-4 w-4 mr-2" />
                    )}
                    Retry Failed
                  </Button>
                </Tooltip>
                <ConfirmDialog
                  open={bannerConfirmOpen}
                  onClose={() => setBannerConfirmOpen(false)}
                  onConfirm={() => onRetryFailed(failedAgent.session_id!)}
                  title="Retry Failed Workflow"
                  message={`This will retry the failed "${failedAgent.agent_type}" agent from the failed layer. All agents in this layer will be re-run.`}
                  confirmLabel="Retry"
                  variant="destructive"
                />
              </div>
            )}
            {(displayedState.status === 'completed' || displayedState.status === 'project_completed') && (
              <div className="flex items-center gap-6 rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm dark:border-green-800 dark:bg-green-950/30">
                <div className="flex items-center gap-2 text-green-700 dark:text-green-400">
                  <CheckCircle className="h-4 w-4" />
                  <span className="font-medium">Completed</span>
                  {displayedState.completed_at && (
                    <span className="text-green-600 dark:text-green-500">
                      {formatDateTime(displayedState.completed_at)}
                    </span>
                  )}
                </div>
                {displayedState.total_duration_sec != null && (
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <Clock className="h-3.5 w-3.5" />
                    <span>{formatDurationSec(displayedState.total_duration_sec)}</span>
                  </div>
                )}
                {displayedState.total_tokens_used != null && displayedState.total_tokens_used > 0 && (
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <Zap className="h-3.5 w-3.5" />
                    <span>{formatTokenCount(displayedState.total_tokens_used)} tokens</span>
                  </div>
                )}
              </div>
            )}
            <PhaseTimeline
              workflow={displayedState}
              agentHistory={agentHistory}
              ticketId={ticketId}
              sessions={!ticketId ? sessions : undefined}
              onAgentSelect={onAgentSelect}
              logPanelCollapsed={logPanelCollapsed}
              onRetryFailed={onRetryFailed}
              retryingSessionId={retryingSessionId}
            />
          </>
        ) : (
          <div className="text-center py-8 space-y-3">
            <p className="text-muted-foreground text-sm">
              {(onShowRunDialog || onShowEpicRunDialog) ? 'No workflow configured for this ticket' : 'No workflows in this tab'}
            </p>
            {activeChainId ? (
              <Link to={`/chains/${encodeURIComponent(activeChainId)}`}>
                <Button variant="outline" size="sm">
                  <ExternalLink className="h-4 w-4 mr-2" />
                  View Chain
                </Button>
              </Link>
            ) : issueType === 'epic' && onShowEpicRunDialog ? (
              <Button
                variant="outline"
                size="sm"
                onClick={onShowEpicRunDialog}
              >
                <Layers className="h-4 w-4 mr-2" />
                Run Epic Workflow
              </Button>
            ) : onShowRunDialog ? (
              <Button
                variant="outline"
                size="sm"
                onClick={onShowRunDialog}
              >
                <Play className="h-4 w-4 mr-2" />
                Run Workflow
              </Button>
            ) : null}
          </div>
        )}
      </div>
      {(hasActivePhase || selectedPanelAgent) && (
        <AgentLogPanel
          activeAgents={activeAgents}
          sessions={sessions}
          collapsed={logPanelCollapsed}
          onToggleCollapse={onToggleLogPanel}
          selectedAgent={selectedPanelAgent}
          onAgentSelect={onAgentSelect}
          onRetryFailed={onRetryFailed}
          retryingSessionId={retryingSessionId}
          workflowStatus={displayedState?.status}
          onResumeSession={onResumeSession}
          resumePending={resumeSessionPending}
        />
      )}
    </div>
  )
}
