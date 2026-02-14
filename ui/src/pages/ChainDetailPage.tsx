import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { ArrowLeft, Play, XCircle, Edit, ListPlus } from 'lucide-react'
import { useGoBack } from '@/hooks/useGoBack'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { CreateChainDialog } from '@/components/chains/CreateChainDialog'
import { AppendToChainDialog } from '@/components/chains/AppendToChainDialog'
import { useChain, useStartChain, useCancelChain } from '@/hooks/useChains'
import {
  statusColor,
  capitalize,
  formatRelativeTime,
  formatElapsedTime,
} from '@/lib/utils'
import type { ChainExecution, ChainExecutionItem } from '@/types/chain'

function ItemStatusBadge({ status }: { status: string }) {
  return (
    <Badge className={statusColor(status)}>
      {capitalize(status)}
    </Badge>
  )
}

function ItemRow({ item }: { item: ChainExecutionItem }) {
  const duration = item.started_at
    ? formatElapsedTime(item.started_at, item.ended_at)
    : null

  return (
    <div className="flex items-center gap-4 px-4 py-3 border-b border-border last:border-b-0">
      <span className="text-xs font-mono text-muted-foreground w-6 text-right shrink-0">
        {item.position + 1}
      </span>
      <Link
        to={`/tickets/${item.ticket_id}`}
        className="text-sm font-mono text-primary hover:underline shrink-0"
      >
        {item.ticket_id}
      </Link>
      {item.ticket_title && (
        <span className="text-sm text-muted-foreground truncate min-w-0">
          {item.ticket_title}
        </span>
      )}
      <div className="flex-1" />
      <ItemStatusBadge status={item.status} />
      {item.started_at && (
        <span className="text-xs text-muted-foreground shrink-0">
          {formatRelativeTime(item.started_at)}
        </span>
      )}
      {duration && (
        <span className="text-xs text-muted-foreground shrink-0">
          {duration}
        </span>
      )}
    </div>
  )
}

function ChainActions({
  chain,
  onEdit,
  onAppend,
}: {
  chain: ChainExecution
  onEdit: () => void
  onAppend: () => void
}) {
  const startMutation = useStartChain()
  const cancelMutation = useCancelChain()

  const handleStart = async () => {
    try {
      await startMutation.mutateAsync(chain.id)
    } catch {
      // Error handled by mutation state
    }
  }

  const handleCancel = async () => {
    try {
      await cancelMutation.mutateAsync(chain.id)
    } catch {
      // Error handled by mutation state
    }
  }

  return (
    <div className="flex items-center gap-2">
      {chain.status === 'pending' && (
        <>
          <Button variant="outline" size="sm" onClick={onEdit}>
            <Edit className="h-4 w-4 mr-1" />
            Edit
          </Button>
          <Button size="sm" onClick={handleStart} disabled={startMutation.isPending}>
            {startMutation.isPending ? (
              <Spinner size="sm" className="mr-1" />
            ) : (
              <Play className="h-4 w-4 mr-1" />
            )}
            Start
          </Button>
        </>
      )}
      {chain.status === 'running' && (
        <>
          <Button variant="outline" size="sm" onClick={onAppend}>
            <ListPlus className="h-4 w-4 mr-1" />
            Append Tickets
          </Button>
          <Button
            variant="destructive"
            size="sm"
            onClick={handleCancel}
            disabled={cancelMutation.isPending}
          >
            {cancelMutation.isPending ? (
              <Spinner size="sm" className="mr-1" />
            ) : (
              <XCircle className="h-4 w-4 mr-1" />
            )}
            Cancel
          </Button>
        </>
      )}
      {(startMutation.isError || cancelMutation.isError) && (
        <span className="text-xs text-destructive">
          {((startMutation.error || cancelMutation.error) as Error)?.message ?? 'Action failed'}
        </span>
      )}
    </div>
  )
}

export function ChainDetailPage() {
  const { id } = useParams<{ id: string }>()
  const goBack = useGoBack('/chains')
  const [showEdit, setShowEdit] = useState(false)
  const [showAppend, setShowAppend] = useState(false)
  const [polling, setPolling] = useState(false)

  const { data: displayChain, isLoading, error } = useChain(id!, {
    enabled: !!id,
    refetchInterval: polling ? 5000 : false,
  })

  useEffect(() => {
    setPolling(displayChain?.status === 'running')
  }, [displayChain?.status])

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  if (error || !displayChain) {
    return (
      <div className="space-y-4">
        <Button variant="ghost" onClick={goBack}>
          <ArrowLeft className="h-4 w-4 mr-2" />
          Back to Chains
        </Button>
        <p className="text-destructive">
          {error instanceof Error ? error.message : 'Chain not found'}
        </p>
      </div>
    )
  }

  const items = displayChain.items ?? []
  const completedCount = items.filter((i) => i.status === 'completed').length

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="sm" onClick={goBack}>
          <ArrowLeft className="h-4 w-4 mr-1" />
          Chains
        </Button>
      </div>

      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold tracking-tight">{displayChain.name}</h1>
            <Badge className={statusColor(displayChain.status)}>
              {capitalize(displayChain.status)}
            </Badge>
          </div>
          <div className="flex items-center gap-3 text-sm text-muted-foreground">
            <span>Workflow: {displayChain.workflow_name}</span>
            <span>Created {formatRelativeTime(displayChain.created_at)}</span>
            <span>
              {completedCount}/{items.length} items completed
            </span>
          </div>
        </div>
        <ChainActions chain={displayChain} onEdit={() => setShowEdit(true)} onAppend={() => setShowAppend(true)} />
      </div>

      <div className="border border-border rounded-lg">
        <div className="px-4 py-2 border-b border-border bg-muted/30">
          <div className="flex items-center gap-4 text-xs font-medium text-muted-foreground uppercase tracking-wider">
            <span className="w-6 text-right">#</span>
            <span>Ticket</span>
            <span className="flex-1" />
            <span>Status</span>
            <span className="w-16">Started</span>
            <span className="w-16">Duration</span>
          </div>
        </div>
        {items.length === 0 ? (
          <div className="p-4 text-center text-muted-foreground text-sm">
            No items in this chain
          </div>
        ) : (
          [...items]
            .sort((a, b) => a.position - b.position)
            .map((item) => <ItemRow key={item.id} item={item} />)
        )}
      </div>

      <CreateChainDialog
        open={showEdit}
        onClose={() => setShowEdit(false)}
        editChain={displayChain}
      />

      {displayChain.status === 'running' && (
        <AppendToChainDialog
          open={showAppend}
          onClose={() => setShowAppend(false)}
          chain={displayChain}
        />
      )}
    </div>
  )
}
