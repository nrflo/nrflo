import { useParams, useNavigate, useSearchParams, Link } from 'react-router-dom'
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
import { useState, useCallback } from 'react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Card, CardContent } from '@/components/ui/Card'
import { Spinner } from '@/components/ui/Spinner'
import { Input } from '@/components/ui/Input'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { RunWorkflowDialog } from '@/components/workflow/RunWorkflowDialog'
import { RunEpicWorkflowDialog } from '@/components/workflow/RunEpicWorkflowDialog'
import { useGoBack } from '@/hooks/useGoBack'
import { useChainList } from '@/hooks/useChains'
import {
  useTicket,
  useWorkflow,
  useAgentSessions,
  useProjectFindings,
  useCloseTicket,
  useReopenTicket,
  useDeleteTicket,
} from '@/hooks/useTickets'
import { useProjectStore } from '@/stores/projectStore'
import { useWebSocketSubscription } from '@/hooks/useWebSocketSubscription'
import { IssueTypeIcon } from '@/components/tickets/IssueTypeIcon'
import { cn, statusColor } from '@/lib/utils'
import { TicketWorkflowTab } from './TicketWorkflowTab'
import { HierarchyTabContent } from './HierarchyTabContent'
import { DescriptionTabContent } from './DescriptionTabContent'
import { DetailsTabContent } from './DetailsTabContent'

export function TicketDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const goBack = useGoBack('/tickets')
  const [closeReason, setCloseReason] = useState('')
  const [showCloseForm, setShowCloseForm] = useState(false)
  const tabParam = searchParams.get('tab')
  const [activeTab, setActiveTab] = useState<'hierarchy' | 'workflow' | 'description' | 'details'>(
    tabParam === 'workflow' || tabParam === 'description' || tabParam === 'details' || tabParam === 'hierarchy' ? tabParam : 'workflow'
  )
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [showRunDialog, setShowRunDialog] = useState(false)
  const [showEpicRunDialog, setShowEpicRunDialog] = useState(false)
  const [interactiveSession, setInteractiveSession] = useState<{ sessionId: string; agentType: string } | null>(null)
  const [workflowExpanded, setWorkflowExpanded] = useState(false)

  // WebSocket subscription for this ticket's real-time updates
  useWebSocketSubscription(id)

  const { data: ticket, isLoading, error } = useTicket(id!)

  // Fetch workflow state from workflow API (uses workflow_instances + agent_sessions tables)
  const { data: workflowData } = useWorkflow(id!, { enabled: !!id })

  // Fetch agent sessions for the running agent log panel
  const { data: sessionsData } = useAgentSessions(id!, undefined, { enabled: !!id })

  // Fetch project findings for the findings tab
  const currentProject = useProjectStore((s) => s.currentProject)
  const { data: projectFindings } = useProjectFindings(currentProject)

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

  const blockedReason = ticket
    ? ticket.status === 'closed'
      ? 'Cannot run workflow on a closed ticket'
      : ticket.is_blocked
        ? `Cannot run workflow — blocked by: ${ticket.blocked_by?.join(', ') || 'open dependencies'}`
        : undefined
    : undefined

  const handleExpandedChange = useCallback((expanded: boolean) => {
    setWorkflowExpanded(expanded)
  }, [])

  const handleClose = async () => {
    if (!id) return
    await closeMutation.mutateAsync({ id, reason: closeReason })
    setShowCloseForm(false)
    setCloseReason('')
  }

  const handleDelete = () => {
    setShowDeleteConfirm(true)
  }

  const doDelete = async () => {
    if (!id) return
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
      activeTab === 'workflow' && workflowExpanded ? 'max-w-full px-4' : 'max-w-7xl'
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
        </div>
      </div>

      {/* Tab Content */}
      <div className="flex-1">
        {activeTab === 'hierarchy' && (
          <HierarchyTabContent ticket={ticket} />
        )}

        {activeTab === 'workflow' && (
          <TicketWorkflowTab
            ticketId={id}
            workflowData={workflowData}
            sessionsData={sessionsData}
            issueType={ticket?.issue_type}
            activeChainId={activeEpicChain?.id ?? null}
            interactiveSession={interactiveSession}
            onInteractiveStart={setInteractiveSession}
            onInteractiveEnd={() => setInteractiveSession(null)}
            onShowRunDialog={() => setShowRunDialog(true)}
            onShowEpicRunDialog={() => setShowEpicRunDialog(true)}
            onExpandedChange={handleExpandedChange}
            projectFindings={projectFindings}
            blockedReason={blockedReason}
          />
        )}

        {activeTab === 'description' && (
          <DescriptionTabContent ticket={ticket} />
        )}

        {activeTab === 'details' && (
          <DetailsTabContent ticket={ticket} />
        )}

      </div>

      {/* Delete Confirm Dialog */}
      <ConfirmDialog
        open={showDeleteConfirm}
        onClose={() => setShowDeleteConfirm(false)}
        onConfirm={doDelete}
        title="Delete Ticket"
        message="Are you sure you want to delete this ticket?"
        confirmLabel="Delete"
        variant="destructive"
      />

      {/* Run Workflow Dialog */}
      {id && (
        <RunWorkflowDialog
          open={showRunDialog}
          onClose={() => setShowRunDialog(false)}
          ticketId={id}
          blockedReason={blockedReason}
          onInteractiveStart={(sessionId, agentType) => {
            setShowRunDialog(false)
            setInteractiveSession({ sessionId, agentType })
          }}
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

    </div>
  )
}
