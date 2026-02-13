import {
  CheckCircle,
  Clock,
  Play,
  Square,
  Zap,
} from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { Select } from '@/components/ui/Select'
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
  onShowRunDialog: () => void
  onRestart?: (sessionId: string) => void
  restartingSessionId?: string | null
}

export function WorkflowTabContent({
  ticketId,
  hasWorkflow,
  displayedState,
  displayedWorkflowName,
  hasMultipleWorkflows,
  workflows,
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
  onShowRunDialog,
  onRestart,
  restartingSessionId,
}: WorkflowTabContentProps) {
  const agentHistory = displayedState?.agent_history

  return (
    <div className={cn(
      'flex gap-0',
      (hasActivePhase || selectedPanelAgent) && 'min-h-[calc(100vh-280px)]'
    )}>
      <div className="flex-1 min-w-0 space-y-4 max-w-6xl">
        {hasWorkflow && displayedState ? (
          <>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                {hasMultipleWorkflows ? (
                  <Select
                    value={selectedWorkflow || displayedWorkflowName}
                    onChange={(e) => onSelectWorkflow(e.target.value)}
                    className="w-32 h-8 text-sm"
                  >
                    {workflows.map((wf) => (
                      <option key={wf} value={wf}>
                        {wf}
                      </option>
                    ))}
                  </Select>
                ) : displayedWorkflowName ? (
                  <Badge variant="secondary">{displayedWorkflowName}</Badge>
                ) : null}
                {isOrchestrated && (
                  <Badge className="bg-yellow-500/20 text-yellow-600 dark:text-yellow-400 border-yellow-500/30">
                    Auto
                  </Badge>
                )}
                {(isOrchestrated || hasActivePhase) && (
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
                )}
              </div>
              {!(isOrchestrated || hasActivePhase) && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={onShowRunDialog}
                >
                  <Play className="h-4 w-4 mr-2" />
                  Run Workflow
                </Button>
              )}
            </div>
            {displayedState.status === 'completed' && (
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
            />
          </>
        ) : (
          <div className="text-center py-8 space-y-3">
            <p className="text-muted-foreground text-sm">
              No workflow configured for this ticket
            </p>
            <Button
              variant="outline"
              size="sm"
              onClick={onShowRunDialog}
            >
              <Play className="h-4 w-4 mr-2" />
              Run Workflow
            </Button>
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
          onRestart={onRestart}
          restartingSessionId={restartingSessionId}
        />
      )}
    </div>
  )
}
