import { useParams, useNavigate, Link } from 'react-router-dom'
import {
  ArrowLeft,
  Edit,
  Trash2,
  CheckCircle,
  RotateCcw,
  FileText,
  GitBranch,
  Info,
  Network,
} from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Card, CardContent } from '@/components/ui/Card'
import { Spinner } from '@/components/ui/Spinner'
import { Input } from '@/components/ui/Input'
import { RunWorkflowDialog } from '@/components/workflow/RunWorkflowDialog'
import { RunEpicWorkflowDialog } from '@/components/workflow/RunEpicWorkflowDialog'
import { useGoBack } from '@/hooks/useGoBack'
import { useChainList } from '@/hooks/useChains'
import {
  useTicket,
  useWorkflow,
  useAgentSessions,
  useCloseTicket,
  useReopenTicket,
  useDeleteTicket,
  useStopWorkflow,
  useRetryFailedAgent,
  useTakeControl,
  useExitInteractive,
  useResumeSession,
} from '@/hooks/useTickets'
import { useWebSocketSubscription } from '@/hooks/useWebSocketSubscription'
import type { WorkflowState } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'
import { IssueTypeIcon } from '@/components/tickets/IssueTypeIcon'
import { cn, statusColor } from '@/lib/utils'
import { AgentTerminalDialog } from '@/components/workflow/AgentTerminalDialog'
import { WorkflowTabContent } from './WorkflowTabContent'
import { HierarchyTabContent } from './HierarchyTabContent'
import { DescriptionTabContent } from './DescriptionTabContent'
import { DetailsTabContent } from './DetailsTabContent'

export function TicketDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const goBack = useGoBack('/tickets')
  const [closeReason, setCloseReason] = useState('')
  const [showCloseForm, setShowCloseForm] = useState(false)
  const [selectedWorkflow, setSelectedWorkflow] = useState<string>('')
  const [activeTab, setActiveTab] = useState<'hierarchy' | 'workflow' | 'description' | 'details'>('hierarchy')
  const [showRunDialog, setShowRunDialog] = useState(false)
  const [showEpicRunDialog, setShowEpicRunDialog] = useState(false)
  const [logPanelCollapsed, setLogPanelCollapsed] = useState(false)
  const [selectedPanelAgent, setSelectedPanelAgent] = useState<SelectedAgentData | null>(null)
  const [interactiveSession, setInteractiveSession] = useState<{ sessionId: string; agentType: string } | null>(null)

  // WebSocket subscription for this ticket's real-time updates
  useWebSocketSubscription(id)

  const { data: ticket, isLoading, error } = useTicket(id!)

  // Fetch workflow state from workflow API (uses workflow_instances + agent_sessions tables)
  const { data: workflowData } = useWorkflow(id!, { enabled: !!id })

  const workflows = workflowData?.workflows ?? []
  const allWorkflows = (workflowData?.all_workflows ?? {}) as Record<string, WorkflowState>
  const hasWorkflow = workflowData?.has_workflow ?? false
  const hasMultipleWorkflows = workflows.length > 1

  // Determine which workflow state to display
  const defaultState = (workflowData?.state ?? null) as WorkflowState | null
  const displayedWorkflowName = selectedWorkflow || defaultState?.workflow || workflows[0] || ''
  const displayedState = selectedWorkflow && allWorkflows[selectedWorkflow]
    ? allWorkflows[selectedWorkflow]
    : defaultState

  // Get active agents from displayed workflow state
  const activeAgents = displayedState?.active_agents ?? {}

  // Fetch agent sessions for the running agent log panel
  const { data: sessionsData } = useAgentSessions(id!, undefined, { enabled: !!id })
  const sessions = sessionsData?.sessions ?? []

  // Query active chains for epic tickets
  const isEpic = ticket?.issue_type === 'epic'
  const { data: epicChains } = useChainList(
    { epic_ticket_id: id },
    { enabled: !!id && isEpic }
  )
  const activeEpicChain = epicChains?.find(
    (c) => c.status === 'pending' || c.status === 'running'
  )

  const closeMutation = useCloseTicket()
  const reopenMutation = useReopenTicket()
  const deleteMutation = useDeleteTicket()
  const stopMutation = useStopWorkflow()
  const retryFailedMutation = useRetryFailedAgent()
  const takeControlMutation = useTakeControl()
  const resumeSessionMutation = useResumeSession()
  const exitInteractiveMutation = useExitInteractive()

  // Detect if orchestration is running (via _orchestration findings key)
  const orchestrationStatus = displayedState?.findings?.['_orchestration'] as
    | { status?: string }
    | undefined
  const isOrchestrated = orchestrationStatus?.status === 'running'

  // Detect if any phase is in_progress
  const hasActivePhase = displayedState?.phases
    ? Object.values(displayedState.phases).some((p) => p.status === 'in_progress')
    : false

  const handleClose = async () => {
    if (!id) return
    await closeMutation.mutateAsync({ id, reason: closeReason })
    setShowCloseForm(false)
    setCloseReason('')
  }

  const handleDelete = async () => {
    if (!id || !confirm('Are you sure you want to delete this ticket?')) return
    await deleteMutation.mutateAsync(id)
    navigate('/tickets')
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  if (error || !ticket) {
    return (
      <div className="text-center py-12">
        <p className="text-destructive">
          {error ? `Error: ${error.message}` : 'Ticket not found'}
        </p>
        <Button variant="link" className="mt-4" onClick={goBack}>
          Back to tickets
        </Button>
      </div>
    )
  }

  return (
    <div className={cn(
      'mx-auto space-y-6',
      activeTab === 'workflow' && (hasActivePhase || selectedPanelAgent) ? 'max-w-full px-4' : 'max-w-7xl'
    )}>
      {/* Header */}
      <div className="flex items-start gap-4">
        <Button variant="ghost" size="icon" onClick={goBack}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <div className="flex items-center gap-3 mb-2">
            <IssueTypeIcon type={ticket.issue_type} size="md" />
            <span className="text-sm text-muted-foreground font-mono">
              {ticket.id}
            </span>
            <Badge className={cn(statusColor(ticket.status))}>
              {ticket.status.replace('_', ' ')}
            </Badge>
          </div>
          <h1 className="text-2xl font-bold tracking-tight">{ticket.title}</h1>
        </div>
        <div className="flex items-center gap-2">
          {ticket.status === 'closed' ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => id && reopenMutation.mutateAsync({ id })}
              disabled={reopenMutation.isPending}
            >
              <RotateCcw className="h-4 w-4 mr-2" />
              Reopen
            </Button>
          ) : (
            <>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setShowCloseForm(!showCloseForm)}
              >
                <CheckCircle className="h-4 w-4 mr-2" />
                Close
              </Button>
              <Link to={`/tickets/${encodeURIComponent(ticket.id)}/edit`}>
                <Button variant="outline" size="sm">
                  <Edit className="h-4 w-4 mr-2" />
                  Edit
                </Button>
              </Link>
            </>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={handleDelete}
            disabled={deleteMutation.isPending}
            className="text-destructive hover:text-destructive"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Close form */}
      {showCloseForm && (
        <Card className="border-primary">
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <Input
                placeholder="Close reason (optional)"
                value={closeReason}
                onChange={(e) => setCloseReason(e.target.value)}
                className="flex-1"
              />
              <Button
                onClick={handleClose}
                disabled={closeMutation.isPending}
              >
                {closeMutation.isPending && (
                  <Spinner size="sm" className="mr-2" />
                )}
                Confirm Close
              </Button>
              <Button variant="ghost" onClick={() => setShowCloseForm(false)}>
                Cancel
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Tabs */}
      <div className="border-b border-border">
        <div className="flex gap-1">
          <button
            onClick={() => setActiveTab('hierarchy')}
            className={cn(
              'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === 'hierarchy'
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            )}
          >
            <Network className="h-4 w-4" />
            Hierarchy
          </button>
          <button
            onClick={() => setActiveTab('description')}
            className={cn(
              'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === 'description'
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            )}
          >
            <FileText className="h-4 w-4" />
            Description
          </button>
          <button
            onClick={() => setActiveTab('details')}
            className={cn(
              'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === 'details'
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            )}
          >
            <Info className="h-4 w-4" />
            Details
          </button>
          <button
            onClick={() => setActiveTab('workflow')}
            className={cn(
              'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === 'workflow'
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            )}
          >
            <GitBranch className="h-4 w-4" />
            Workflow
          </button>
        </div>
      </div>

      {/* Tab Content */}
      <div className="flex-1">
        {activeTab === 'hierarchy' && (
          <HierarchyTabContent ticket={ticket} />
        )}

        {activeTab === 'workflow' && (
          <WorkflowTabContent
            ticketId={id}
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
            sessions={sessions}
            logPanelCollapsed={logPanelCollapsed}
            onToggleLogPanel={() => setLogPanelCollapsed(p => !p)}
            selectedPanelAgent={selectedPanelAgent}
            onAgentSelect={setSelectedPanelAgent}
            onStop={() =>
              id && stopMutation.mutate({
                ticketId: id,
                workflow: displayedWorkflowName || undefined,
              })
            }
            stopPending={stopMutation.isPending}
            issueType={ticket?.issue_type}
            onShowRunDialog={() => setShowRunDialog(true)}
            onShowEpicRunDialog={() => setShowEpicRunDialog(true)}
            activeChainId={activeEpicChain?.id ?? null}
            onRetryFailed={(sessionId) =>
              id && retryFailedMutation.mutate({
                ticketId: id,
                params: { workflow: displayedWorkflowName, session_id: sessionId },
              })
            }
            retryingSessionId={retryFailedMutation.isPending ? (retryFailedMutation.variables?.params.session_id ?? null) : null}
            onTakeControl={(sessionId) => {
              if (!id) return
              const agent = Object.values(activeAgents).find((a) => a.session_id === sessionId)
              takeControlMutation.mutate(
                { ticketId: id, params: { workflow: displayedWorkflowName, session_id: sessionId } },
                { onSuccess: (data) => setInteractiveSession({ sessionId: data.session_id, agentType: agent?.agent_type ?? 'agent' }) }
              )
            }}
            takeControlPending={takeControlMutation.isPending}
            onResumeSession={(sessionId) => {
              if (!id) return
              resumeSessionMutation.mutate(
                { ticketId: id, params: { session_id: sessionId } },
                { onSuccess: (data) => setInteractiveSession({ sessionId: data.session_id, agentType: 'agent' }) }
              )
            }}
            resumeSessionPending={resumeSessionMutation.isPending}
          />
        )}

        {activeTab === 'description' && (
          <DescriptionTabContent ticket={ticket} />
        )}

        {activeTab === 'details' && (
          <DetailsTabContent ticket={ticket} />
        )}

      </div>

      {/* Run Workflow Dialog */}
      {id && (
        <RunWorkflowDialog
          open={showRunDialog}
          onClose={() => setShowRunDialog(false)}
          ticketId={id}
        />
      )}

      {/* Run Epic Workflow Dialog */}
      {id && ticket && (
        <RunEpicWorkflowDialog
          open={showEpicRunDialog}
          onClose={() => setShowEpicRunDialog(false)}
          ticketId={id}
          ticketTitle={ticket.title}
        />
      )}

      {/* Interactive Terminal Dialog */}
      {interactiveSession && id && (
        <AgentTerminalDialog
          open={!!interactiveSession}
          onClose={() => setInteractiveSession(null)}
          onExitSession={() => {
            exitInteractiveMutation.mutate(
              { ticketId: id, params: { workflow: displayedWorkflowName, session_id: interactiveSession.sessionId } },
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
