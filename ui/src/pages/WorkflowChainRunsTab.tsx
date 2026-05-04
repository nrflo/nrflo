import { useState } from 'react'
import { Play, XCircle, ChevronDown, ChevronRight } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { Textarea } from '@/components/ui/Textarea'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { useIsAdmin } from '@/stores/authStore'
import {
  useChainRunsList,
  useChainRun,
  useStartChainRun,
  useCancelChainRun,
} from '@/hooks/useWorkflowChains'
import { formatRelativeTime, statusColor, capitalize } from '@/lib/utils'
import type { WorkflowChainRun } from '@/types/workflowChain'

function StartRunDialog({
  chainId,
  open,
  onClose,
}: {
  chainId: string
  open: boolean
  onClose: () => void
}) {
  const [instructions, setInstructions] = useState('')
  const startMutation = useStartChainRun()

  const handleSubmit = () => {
    startMutation.mutate(
      { chainId, data: { instructions: instructions.trim() || undefined } },
      {
        onSuccess: () => {
          toast.success('Chain run started')
          setInstructions('')
          onClose()
        },
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : 'Failed to start run')
        },
      }
    )
  }

  const handleClose = () => {
    setInstructions('')
    onClose()
  }

  return (
    <Dialog open={open} onClose={handleClose}>
      <DialogHeader onClose={handleClose}>
        <h2 className="text-lg font-semibold">Start Chain Run</h2>
      </DialogHeader>
      <DialogBody className="space-y-4">
        <div className="space-y-1.5">
          <label className="text-sm font-medium">Initial instructions (optional)</label>
          <Textarea
            value={instructions}
            onChange={(e) => setInstructions(e.target.value)}
            placeholder="Instructions for the first step agent…"
            rows={4}
            autoFocus
          />
        </div>
        {startMutation.error && (
          <p className="text-sm text-destructive">
            {startMutation.error instanceof Error
              ? startMutation.error.message
              : 'Failed to start run'}
          </p>
        )}
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={handleClose} disabled={startMutation.isPending}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={startMutation.isPending}>
          {startMutation.isPending ? 'Starting…' : 'Start run'}
        </Button>
      </DialogFooter>
    </Dialog>
  )
}

function RunSteps({ chainId, runId }: { chainId: string; runId: string }) {
  const { data: detail, isLoading } = useChainRun(chainId, runId)

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 py-2 px-4 text-sm text-muted-foreground">
        <Spinner size="sm" />
        Loading steps…
      </div>
    )
  }

  if (!detail?.steps?.length) {
    return <p className="py-2 px-4 text-sm text-muted-foreground">No steps.</p>
  }

  return (
    <div className="px-4 pb-3 space-y-1">
      {detail.steps.map((step) => (
        <div key={step.id} className="flex items-start gap-3 py-1.5 text-sm">
          <span className="text-muted-foreground w-6 shrink-0 text-right">{step.position + 1}.</span>
          <span className="font-medium min-w-0">{step.workflow_name}</span>
          <span
            className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium shrink-0 ${statusColor(step.status)}`}
          >
            {capitalize(step.status)}
          </span>
          <span className="text-muted-foreground text-xs shrink-0">{step.scope_type}</span>
          {step.instructions_used && (
            <span className="text-muted-foreground text-xs truncate">{step.instructions_used}</span>
          )}
        </div>
      ))}
    </div>
  )
}

function RunRow({
  run,
  chainId,
  isAdmin,
}: {
  run: WorkflowChainRun
  chainId: string
  isAdmin: boolean
}) {
  const [expanded, setExpanded] = useState(false)
  const [cancelTarget, setCancelTarget] = useState(false)
  const cancelMutation = useCancelChainRun()

  const isTerminal = run.status === 'completed' || run.status === 'failed' || run.status === 'canceled'
  const canCancel = isAdmin && !isTerminal

  return (
    <>
      <TableRow
        className="cursor-pointer"
        onClick={() => setExpanded((v) => !v)}
      >
        <TableCell className="w-6">
          {expanded ? (
            <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
          )}
        </TableCell>
        <TableCell>
          <span
            className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${statusColor(run.status)}`}
          >
            {capitalize(run.status)}
          </span>
        </TableCell>
        <TableCell className="text-sm text-muted-foreground">
          {run.triggered_by || '—'}
        </TableCell>
        <TableCell className="text-sm text-muted-foreground max-w-xs truncate">
          {run.initial_instructions || '—'}
        </TableCell>
        <TableCell className="text-sm text-muted-foreground">
          {formatRelativeTime(run.created_at)}
        </TableCell>
        <TableCell onClick={(e) => e.stopPropagation()}>
          {canCancel && (
            <button
              onClick={() => setCancelTarget(true)}
              className="p-1 text-muted-foreground hover:text-destructive transition-colors"
              title="Cancel run"
            >
              <XCircle className="h-3.5 w-3.5" />
            </button>
          )}
        </TableCell>
      </TableRow>
      {expanded && (
        <TableRow>
          <TableCell colSpan={6} className="p-0 bg-muted/30">
            <RunSteps chainId={chainId} runId={run.id} />
          </TableCell>
        </TableRow>
      )}
      {isAdmin && (
        <ConfirmDialog
          open={cancelTarget}
          onClose={() => setCancelTarget(false)}
          onConfirm={() => {
            cancelMutation.mutate(
              { chainId, runId: run.id },
              {
                onSuccess: () => toast.success('Run canceled'),
                onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to cancel'),
                onSettled: () => setCancelTarget(false),
              }
            )
          }}
          title="Cancel Run"
          message="Are you sure you want to cancel this chain run? This cannot be undone."
          confirmLabel="Cancel run"
          variant="destructive"
        />
      )}
    </>
  )
}

export function WorkflowChainRunsTab({ chainId }: { chainId: string }) {
  const [showStart, setShowStart] = useState(false)
  const isAdmin = useIsAdmin()
  const { data: runs, isLoading, error } = useChainRunsList(chainId)

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          {runs?.length ?? 0} run{runs?.length !== 1 ? 's' : ''}
        </p>
        <Button size="sm" onClick={() => setShowStart(true)}>
          <Play className="h-3.5 w-3.5 mr-1.5" />
          Start run
        </Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner size="lg" />
        </div>
      ) : error ? (
        <p className="text-sm text-destructive">
          {error instanceof Error ? error.message : 'Failed to load runs'}
        </p>
      ) : !runs || runs.length === 0 ? (
        <div className="text-center py-8 text-muted-foreground border border-dashed border-border rounded-lg">
          <p className="text-sm">No runs yet. Start one to begin executing this chain.</p>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-6" />
              <TableHead className="w-28">Status</TableHead>
              <TableHead>Triggered by</TableHead>
              <TableHead>Instructions</TableHead>
              <TableHead className="w-28">Started</TableHead>
              <TableHead className="w-10" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {runs.map((run) => (
              <RunRow key={run.id} run={run} chainId={chainId} isAdmin={isAdmin} />
            ))}
          </TableBody>
        </Table>
      )}

      <StartRunDialog
        chainId={chainId}
        open={showStart}
        onClose={() => setShowStart(false)}
      />
    </div>
  )
}
