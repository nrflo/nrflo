import { useParams, useNavigate, Link } from 'react-router-dom'
import {
  ArrowLeft,
  Edit,
  Trash2,
  CheckCircle,
  RotateCcw,
  Bug,
  Lightbulb,
  CheckSquare,
  Layers,
  FileText,
  GitBranch,
  Info,
} from 'lucide-react'
import { useState, useEffect } from 'react'
import { useProjectStore } from '@/stores/projectStore'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Card, CardContent } from '@/components/ui/Card'
import { Spinner } from '@/components/ui/Spinner'
import { Input } from '@/components/ui/Input'
import { RunWorkflowDialog } from '@/components/workflow/RunWorkflowDialog'
import {
  useTicket,
  useWorkflow,
  useAgentSessions,
  useCloseTicket,
  useReopenTicket,
  useDeleteTicket,
  useStopWorkflow,
  useRestartAgent,
  useRetryFailedAgent,
} from '@/hooks/useTickets'
import { useWebSocket } from '@/hooks/useWebSocket'
import type { WorkflowState } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'
import { cn, statusColor } from '@/lib/utils'
import { WorkflowTabContent } from './WorkflowTabContent'
import { DescriptionTabContent } from './DescriptionTabContent'
import { DetailsTabContent } from './DetailsTabContent'

function IssueTypeIcon({ type }: { type: string }) {
  switch (type) {
    case 'bug':
      return <Bug className="h-5 w-5 text-red-500" />
    case 'feature':
      return <Lightbulb className="h-5 w-5 text-purple-500" />
    case 'task':
      return <CheckSquare className="h-5 w-5 text-blue-500" />
    case 'epic':
      return <Layers className="h-5 w-5 text-green-500" />
    default:
      return <CheckSquare className="h-5 w-5 text-gray-500" />
  }
}

export function TicketDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [closeReason, setCloseReason] = useState('')
  const [showCloseForm, setShowCloseForm] = useState(false)
  const [selectedWorkflow, setSelectedWorkflow] = useState<string>('')
  const [activeTab, setActiveTab] = useState<'workflow' | 'description' | 'details'>('workflow')
  const [showRunDialog, setShowRunDialog] = useState(false)
  const [logPanelCollapsed, setLogPanelCollapsed] = useState(false)
  const [selectedPanelAgent, setSelectedPanelAgent] = useState<SelectedAgentData | null>(null)

  // WebSocket for real-time updates
  const { subscribe, unsubscribe } = useWebSocket()
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  const currentProject = useProjectStore((s) => s.currentProject)

  // Subscribe to this ticket's updates (wait for real project ID)
  useEffect(() => {
    if (id && projectsLoaded) {
      subscribe(id)
      return () => unsubscribe(id)
    }
  }, [id, projectsLoaded, currentProject, subscribe, unsubscribe])

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

  const closeMutation = useCloseTicket()
  const reopenMutation = useReopenTicket()
  const deleteMutation = useDeleteTicket()
  const stopMutation = useStopWorkflow()
  const restartMutation = useRestartAgent()
  const retryFailedMutation = useRetryFailedAgent()

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
        <Link to="/tickets">
          <Button variant="link" className="mt-4">
            Back to tickets
          </Button>
        </Link>
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
        <Link to="/tickets">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div className="flex-1">
          <div className="flex items-center gap-3 mb-2">
            <IssueTypeIcon type={ticket.issue_type} />
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
        </div>
      </div>

      {/* Tab Content */}
      <div className="flex-1">
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
            onShowRunDialog={() => setShowRunDialog(true)}
            onRestart={(sessionId) =>
              id && restartMutation.mutate({
                ticketId: id,
                params: { workflow: displayedWorkflowName, session_id: sessionId },
              })
            }
            restartingSessionId={restartMutation.isPending ? (restartMutation.variables?.params.session_id ?? null) : null}
            onRetryFailed={(sessionId) =>
              id && retryFailedMutation.mutate({
                ticketId: id,
                params: { workflow: displayedWorkflowName, session_id: sessionId },
              })
            }
            retryingSessionId={retryFailedMutation.isPending ? (retryFailedMutation.variables?.params.session_id ?? null) : null}
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
    </div>
  )
}
