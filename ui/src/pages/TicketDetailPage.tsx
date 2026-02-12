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
  ExternalLink,
  FileText,
  GitBranch,
  Info,
  Play,
  Square,
} from 'lucide-react'
import { useState, useEffect } from 'react'
import { useProjectStore } from '@/stores/projectStore'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { Spinner } from '@/components/ui/Spinner'
import { Input } from '@/components/ui/Input'
import { Select } from '@/components/ui/Select'
import { PhaseTimeline } from '@/components/workflow/PhaseTimeline'
import { RunWorkflowDialog } from '@/components/workflow/RunWorkflowDialog'
import { RunningAgentLog } from '@/components/workflow/RunningAgentLog'
import { AgentMessagesModal } from '@/components/workflow/PhaseGraph/AgentMessagesModal'
import {
  useTicket,
  useWorkflow,
  useAgentSessions,
  useCloseTicket,
  useReopenTicket,
  useDeleteTicket,
  useStopWorkflow,
} from '@/hooks/useTickets'
import { useWebSocket } from '@/hooks/useWebSocket'
import type { WorkflowState, ActiveAgentV4, AgentSession } from '@/types/workflow'
import {
  cn,
  statusColor,
  formatDateTime,
  priorityLabel,
} from '@/lib/utils'

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
  const [selectedLogAgent, setSelectedLogAgent] = useState<{
    agent: ActiveAgentV4
    session?: AgentSession
  } | null>(null)

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

  // Get agent_history from displayed workflow state
  const agentHistory = displayedState?.agent_history
  const activeAgents = displayedState?.active_agents ?? {}

  // Fetch agent sessions for the running agent log panel
  const { data: sessionsData } = useAgentSessions(id!, undefined, { enabled: !!id })
  const sessions = sessionsData?.sessions ?? []

  const closeMutation = useCloseTicket()
  const reopenMutation = useReopenTicket()
  const deleteMutation = useDeleteTicket()
  const stopMutation = useStopWorkflow()

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
      activeTab === 'workflow' && hasActivePhase ? 'max-w-full px-4' : 'max-w-7xl'
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
        {/* Workflow Tab */}
        {activeTab === 'workflow' && (
          <div className={cn(
            'flex gap-0',
            hasActivePhase && 'min-h-[calc(100vh-280px)]'
          )}>
            <div className="flex-1 min-w-0 space-y-4 max-w-4xl">
              {hasWorkflow && displayedState ? (
                <>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      {hasMultipleWorkflows ? (
                        <Select
                          value={selectedWorkflow || displayedWorkflowName}
                          onChange={(e) => setSelectedWorkflow(e.target.value)}
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
                          onClick={() =>
                            id && stopMutation.mutate({
                              ticketId: id,
                              workflow: displayedWorkflowName || undefined,
                            })
                          }
                          disabled={stopMutation.isPending}
                          className="text-destructive hover:text-destructive"
                        >
                          {stopMutation.isPending ? (
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
                        onClick={() => setShowRunDialog(true)}
                      >
                        <Play className="h-4 w-4 mr-2" />
                        Run Workflow
                      </Button>
                    )}
                  </div>
                  <PhaseTimeline
                    workflow={displayedState}
                    agentHistory={agentHistory}
                    ticketId={id}
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
                    onClick={() => setShowRunDialog(true)}
                  >
                    <Play className="h-4 w-4 mr-2" />
                    Run Workflow
                  </Button>
                </div>
              )}
            </div>
            {hasActivePhase && (
              <RunningAgentLog
                activeAgents={activeAgents}
                sessions={sessions}
                collapsed={logPanelCollapsed}
                onToggleCollapse={() => setLogPanelCollapsed(p => !p)}
                onAgentClick={(agent, session) => setSelectedLogAgent({ agent, session })}
              />
            )}
          </div>
        )}

        {/* Description Tab */}
        {activeTab === 'description' && (
          <div className="space-y-6">
            <Card>
              <CardContent className="pt-6">
                {ticket.description ? (
                  <p className="whitespace-pre-wrap">{ticket.description}</p>
                ) : (
                  <p className="text-muted-foreground italic">No description</p>
                )}
              </CardContent>
            </Card>

            {/* Dependencies */}
            {((ticket.blockers?.length ?? 0) > 0 || (ticket.blocks?.length ?? 0) > 0) && (
              <Card>
                <CardHeader>
                  <CardTitle>Dependencies</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  {(ticket.blockers?.length ?? 0) > 0 && (
                    <div>
                      <h4 className="text-sm font-medium mb-2">Blocked by</h4>
                      <div className="space-y-1">
                        {ticket.blockers?.map((dep) => (
                          <Link
                            key={dep.depends_on_id}
                            to={`/tickets/${encodeURIComponent(dep.depends_on_id)}`}
                            className="flex items-center gap-2 text-sm text-primary hover:underline"
                          >
                            <ExternalLink className="h-3 w-3" />
                            {dep.depends_on_id}
                          </Link>
                        ))}
                      </div>
                    </div>
                  )}
                  {(ticket.blocks?.length ?? 0) > 0 && (
                    <div>
                      <h4 className="text-sm font-medium mb-2">Blocks</h4>
                      <div className="space-y-1">
                        {ticket.blocks?.map((dep) => (
                          <Link
                            key={dep.issue_id}
                            to={`/tickets/${encodeURIComponent(dep.issue_id)}`}
                            className="flex items-center gap-2 text-sm text-primary hover:underline"
                          >
                            <ExternalLink className="h-3 w-3" />
                            {dep.issue_id}
                          </Link>
                        ))}
                      </div>
                    </div>
                  )}
                </CardContent>
              </Card>
            )}
          </div>
        )}

        {/* Details Tab */}
        {activeTab === 'details' && (
          <Card>
            <CardContent className="pt-6">
              <dl className="grid grid-cols-2 gap-4">
                <div>
                  <dt className="text-sm text-muted-foreground">Priority</dt>
                  <dd className="font-medium">{priorityLabel(ticket.priority)}</dd>
                </div>
                <div>
                  <dt className="text-sm text-muted-foreground">Type</dt>
                  <dd className="font-medium capitalize">{ticket.issue_type}</dd>
                </div>
                <div>
                  <dt className="text-sm text-muted-foreground">Created by</dt>
                  <dd className="font-medium">{ticket.created_by}</dd>
                </div>
                <div>
                  <dt className="text-sm text-muted-foreground">Status</dt>
                  <dd>
                    <Badge className={cn(statusColor(ticket.status))}>
                      {ticket.status.replace('_', ' ')}
                    </Badge>
                  </dd>
                </div>
                <div>
                  <dt className="text-sm text-muted-foreground">Created</dt>
                  <dd className="font-medium">{formatDateTime(ticket.created_at)}</dd>
                </div>
                <div>
                  <dt className="text-sm text-muted-foreground">Updated</dt>
                  <dd className="font-medium">{formatDateTime(ticket.updated_at)}</dd>
                </div>
                {ticket.closed_at && (
                  <div>
                    <dt className="text-sm text-muted-foreground">Closed</dt>
                    <dd className="font-medium">{formatDateTime(ticket.closed_at)}</dd>
                  </div>
                )}
                {ticket.close_reason && (
                  <div className="col-span-2">
                    <dt className="text-sm text-muted-foreground">Close reason</dt>
                    <dd className="font-medium">{ticket.close_reason}</dd>
                  </div>
                )}
              </dl>
            </CardContent>
          </Card>
        )}
      </div>

      {/* Agent Messages Modal (from log panel click) */}
      {selectedLogAgent && (
        <AgentMessagesModal
          open={true}
          onClose={() => setSelectedLogAgent(null)}
          phaseName={selectedLogAgent.agent.phase || selectedLogAgent.agent.agent_type || ''}
          agent={selectedLogAgent.agent}
          session={selectedLogAgent.session}
        />
      )}

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
