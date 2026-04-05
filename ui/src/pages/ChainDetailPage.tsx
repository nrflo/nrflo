import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { ArrowLeft, Play, XCircle, Edit, ListPlus, Trash2 } from 'lucide-react'
import { useGoBack } from '@/hooks/useGoBack'
import { useTickingClock } from '@/hooks/useElapsedTime'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { StatusCell } from '@/components/ui/StatusCell'
import { CreateChainDialog } from '@/components/chains/CreateChainDialog'
import { AppendToChainDialog } from '@/components/chains/AppendToChainDialog'
import { useChain, useStartChain, useCancelChain, useRemoveFromChain } from '@/hooks/useChains'
import {
  statusColor,
  capitalize,
  formatRelativeTime,
  formatElapsedTime,
  formatTokenCount,
} from '@/lib/utils'
import type { ChainExecution, ChainExecutionItem, ChainStatus } from '@/types/chain'

function ItemRow({ item, chainStatus }: { item: ChainExecutionItem; chainStatus: ChainStatus }) {
  const [removeConfirm, setRemoveConfirm] = useState<string | null>(null)
  const removeMutation = useRemoveFromChain()
  const duration = item.started_at
    ? formatElapsedTime(item.started_at, item.ended_at)
    : null
  const canRemove = chainStatus === 'running' && item.status === 'pending'

  const handleRemove = async () => {
    try {
      await removeMutation.mutateAsync({ id: item.chain_id, data: { ticket_ids: [item.ticket_id] } })
      setRemoveConfirm(null)
    } catch {
      // Error handled by mutation state
    }
  }

  if (removeConfirm === item.ticket_id) {
    return (
      <TableRow>
        <TableCell colSpan={7}>
          <div className="flex items-center justify-between">
            <div className="text-sm">
              Remove <span className="font-semibold">{item.ticket_id}</span> from chain?
            </div>
            <div className="flex gap-2">
              <Button variant="ghost" size="sm" onClick={() => setRemoveConfirm(null)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                size="sm"
                onClick={handleRemove}
                disabled={removeMutation.isPending}
              >
                {removeMutation.isPending ? 'Removing...' : 'Remove'}
              </Button>
            </div>
          </div>
        </TableCell>
      </TableRow>
    )
  }

  return (
    <TableRow>
      <TableCell className="w-8 text-center font-mono text-xs text-muted-foreground">
        {item.status === 'running' ? <Spinner size="sm" /> : item.position + 1}
      </TableCell>
      <TableCell>
        <Link
          to={`/tickets/${item.ticket_id}`}
          className="text-sm font-mono text-primary hover:underline"
        >
          {item.ticket_id}
        </Link>
        {item.ticket_title && (
          <span className="ml-2 text-sm text-muted-foreground truncate">
            {item.ticket_title}
          </span>
        )}
      </TableCell>
      <TableCell><StatusCell status={item.status} /></TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {item.started_at ? formatRelativeTime(item.started_at) : '—'}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {duration ?? '—'}
      </TableCell>
      <TableCell className="text-xs font-mono text-muted-foreground text-right">
        {item.total_tokens_used ? `${formatTokenCount(item.total_tokens_used)} tokens` : '—'}
      </TableCell>
      <TableCell className="w-10">
        {canRemove && (
          <Button variant="ghost" size="icon" onClick={() => setRemoveConfirm(item.ticket_id)}>
            <Trash2 className="h-4 w-4" />
          </Button>
        )}
        {removeMutation.isError && (
          <span className="text-xs text-destructive">
            {(removeMutation.error as Error)?.message ?? 'Remove failed'}
          </span>
        )}
      </TableCell>
    </TableRow>
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

  const { data: displayChain, isLoading, error } = useChain(id!, {
    enabled: !!id,
    refetchInterval: (query) =>
      query.state.data?.status === 'running' ? 10_000 : false,
  })

  useTickingClock(displayChain?.status === 'running')

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
            {displayChain.started_at && (
              <span>Duration: {formatElapsedTime(displayChain.started_at, displayChain.completed_at)}</span>
            )}
          </div>
        </div>
        <ChainActions chain={displayChain} onEdit={() => setShowEdit(true)} onAppend={() => setShowAppend(true)} />
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-8 text-center">#</TableHead>
            <TableHead>Ticket</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Started</TableHead>
            <TableHead>Duration</TableHead>
            <TableHead className="text-right">Tokens</TableHead>
            <TableHead className="w-10" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.length === 0 ? (
            <TableRow>
              <TableCell colSpan={7} className="text-center text-muted-foreground text-sm">
                No items in this chain
              </TableCell>
            </TableRow>
          ) : (
            [...items]
              .sort((a, b) => a.position - b.position)
              .map((item) => <ItemRow key={item.id} item={item} chainStatus={displayChain.status} />)
          )}
        </TableBody>
      </Table>

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
