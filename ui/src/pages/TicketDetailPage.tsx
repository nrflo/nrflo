import { useParams, useNavigate, Link } from 'react-router-dom'
import {
  ArrowLeft,
  Edit,
  Trash2,
  CheckCircle,
  Bug,
  Lightbulb,
  CheckSquare,
  Layers,
  ExternalLink,
  Radio,
  FileText,
  GitBranch,
  Info,
} from 'lucide-react'
import { useState, useMemo } from 'react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { Spinner } from '@/components/ui/Spinner'
import { Input } from '@/components/ui/Input'
import { Select } from '@/components/ui/Select'
import { Toggle } from '@/components/ui/Toggle'
import { PhaseTimeline } from '@/components/workflow/PhaseTimeline'
import {
  useTicket,
  useCloseTicket,
  useDeleteTicket,
} from '@/hooks/useTickets'
import type { WorkflowState } from '@/types/workflow'
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

const LIVE_TRACKING_INTERVAL = 5000 // 5 seconds

export function TicketDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [closeReason, setCloseReason] = useState('')
  const [showCloseForm, setShowCloseForm] = useState(false)
  const [liveTracking, setLiveTracking] = useState(false)
  const [selectedWorkflow, setSelectedWorkflow] = useState<string>('')
  const [activeTab, setActiveTab] = useState<'workflow' | 'description' | 'details'>('workflow')

  const { data: ticket, isLoading, isFetching, error } = useTicket(id!, {
    refetchInterval: liveTracking ? LIVE_TRACKING_INTERVAL : false,
  })

  // Parse agents_state from ticket - single source of truth for all workflow data
  // Format: { "workflow-name": { version, phases, findings, agent_history, ... }, ... }
  const parsedState = useMemo((): {
    state: WorkflowState | null
    workflows: string[]
    allWorkflows: Record<string, WorkflowState>
    hasWorkflow: boolean
  } => {
    if (!ticket?.agents_state) {
      return { state: null, workflows: [], allWorkflows: {}, hasWorkflow: false }
    }
    try {
      const parsed = JSON.parse(ticket.agents_state)

      // agents_state format: { "workflow-name": WorkflowState, ... }
      // Each top-level key is a workflow name, value is the workflow state
      const workflowNames = Object.keys(parsed).filter(key => {
        const value = parsed[key]
        // A workflow state has version and/or phases
        return value && typeof value === 'object' && (value.version || value.phases)
      })

      if (workflowNames.length > 0) {
        const allWorkflows: Record<string, WorkflowState> = {}
        for (const name of workflowNames) {
          allWorkflows[name] = parsed[name]
        }
        const defaultWorkflow = workflowNames[0]
        return {
          state: allWorkflows[defaultWorkflow] || null,
          workflows: workflowNames,
          allWorkflows,
          hasWorkflow: true,
        }
      }

      return { state: null, workflows: [], allWorkflows: {}, hasWorkflow: false }
    } catch {
      return { state: null, workflows: [], allWorkflows: {}, hasWorkflow: false }
    }
  }, [ticket?.agents_state])

  const { workflows, allWorkflows, hasWorkflow } = parsedState
  const hasMultipleWorkflows = workflows.length > 1

  // Determine which workflow state to display
  const displayedWorkflowName = selectedWorkflow || parsedState.state?.workflow || workflows[0] || ''
  const displayedState = selectedWorkflow && allWorkflows[selectedWorkflow]
    ? allWorkflows[selectedWorkflow]
    : parsedState.state

  // Get agent_history from displayed workflow state
  const agentHistory = displayedState?.agent_history
  const closeMutation = useCloseTicket()
  const deleteMutation = useDeleteTicket()

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
    <div className="max-w-7xl mx-auto space-y-6">
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
          {ticket.status !== 'closed' && (
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
          <div className="space-y-4">
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
                  </div>
                  <div className="flex items-center gap-3">
                    {liveTracking && isFetching && (
                      <Radio className="h-3 w-3 text-green-500 animate-pulse" />
                    )}
                    <Toggle
                      checked={liveTracking}
                      onChange={setLiveTracking}
                      label="Live"
                    />
                  </div>
                </div>
                <PhaseTimeline
                  workflow={displayedState}
                  agentHistory={agentHistory}
                  ticketId={id}
                  liveTracking={liveTracking}
                />
              </>
            ) : (
              <p className="text-muted-foreground text-sm py-8 text-center">
                No workflow configured for this ticket
              </p>
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
    </div>
  )
}
