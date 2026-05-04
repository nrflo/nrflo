import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Plus, Pencil, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { ReadOnlyHint } from '@/components/auth/ReadOnlyHint'
import { useIsAdmin } from '@/stores/authStore'
import { useWorkflowChainsList, useCreateWorkflowChain, useDeleteWorkflowChain } from '@/hooks/useWorkflowChains'
import { formatRelativeTime } from '@/lib/utils'
import type { WorkflowChain } from '@/types/workflowChain'

function CreateChainDialog({
  open,
  onClose,
}: {
  open: boolean
  onClose: () => void
}) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const createMutation = useCreateWorkflowChain()
  const navigate = useNavigate()

  const canSubmit = name.trim().length > 0

  const handleSubmit = () => {
    if (!canSubmit) return
    createMutation.mutate(
      { name: name.trim(), description: description.trim(), steps: [] },
      {
        onSuccess: (chain) => {
          onClose()
          setName('')
          setDescription('')
          navigate(`/workflow-chains/${chain.id}`)
        },
      }
    )
  }

  const handleClose = () => {
    setName('')
    setDescription('')
    onClose()
  }

  return (
    <Dialog open={open} onClose={handleClose}>
      <DialogHeader onClose={handleClose}>
        <h2 className="text-lg font-semibold">New Workflow Chain</h2>
      </DialogHeader>
      <DialogBody className="space-y-4">
        <div className="space-y-1.5">
          <label className="text-sm font-medium">Name</label>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="My workflow chain"
            autoFocus
          />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">Description</label>
          <Input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Optional description"
          />
        </div>
        {createMutation.error && (
          <p className="text-sm text-destructive">
            {createMutation.error instanceof Error
              ? createMutation.error.message
              : 'Failed to create chain'}
          </p>
        )}
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={handleClose} disabled={createMutation.isPending}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={!canSubmit || createMutation.isPending}>
          {createMutation.isPending ? 'Creating…' : 'Create'}
        </Button>
      </DialogFooter>
    </Dialog>
  )
}

export function WorkflowChainsPage() {
  const [showCreate, setShowCreate] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<WorkflowChain | null>(null)
  const isAdmin = useIsAdmin()
  const navigate = useNavigate()

  const { data: chains, isLoading, error } = useWorkflowChainsList()
  const deleteMutation = useDeleteWorkflowChain()

  return (
    <div className="max-w-[85%] mx-auto space-y-6">
      {!isAdmin && <ReadOnlyHint />}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Workflow Chains</h1>
          <p className="text-muted-foreground">
            {chains?.length ?? 0} chain{chains?.length !== 1 ? 's' : ''}
          </p>
        </div>
        {isAdmin && (
          <Button onClick={() => setShowCreate(true)}>
            <Plus className="h-4 w-4 mr-2" />
            New Chain
          </Button>
        )}
      </div>

      {isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : error ? (
        <p className="text-destructive text-sm">
          {error instanceof Error ? error.message : 'Failed to load workflow chains'}
        </p>
      ) : !chains || chains.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          <p>No workflow chains found. Create one to get started!</p>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Description</TableHead>
              <TableHead className="w-36">Updated</TableHead>
              <TableHead className="w-24" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {chains.map((chain) => (
              <TableRow
                key={chain.id}
                className="cursor-pointer"
                onClick={() => navigate(`/workflow-chains/${chain.id}`)}
              >
                <TableCell>
                  <span className="font-medium">{chain.name}</span>
                </TableCell>
                <TableCell className="text-muted-foreground text-sm">
                  {chain.description || '—'}
                </TableCell>
                <TableCell className="text-muted-foreground text-sm">
                  {formatRelativeTime(chain.updated_at)}
                </TableCell>
                <TableCell onClick={(e) => e.stopPropagation()}>
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => navigate(`/workflow-chains/${chain.id}`)}
                      className="p-1 text-muted-foreground hover:text-foreground transition-colors"
                      title="Edit"
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </button>
                    {isAdmin && (
                      <button
                        onClick={() => setDeleteTarget(chain)}
                        className="p-1 text-muted-foreground hover:text-destructive transition-colors"
                        title="Delete"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      {isAdmin && (
        <CreateChainDialog open={showCreate} onClose={() => setShowCreate(false)} />
      )}

      {isAdmin && (
        <ConfirmDialog
          open={!!deleteTarget}
          onClose={() => setDeleteTarget(null)}
          onConfirm={() => {
            if (deleteTarget) {
              deleteMutation.mutate(deleteTarget.id, {
                onSettled: () => setDeleteTarget(null),
              })
            }
          }}
          title="Delete Workflow Chain"
          message={`Are you sure you want to delete "${deleteTarget?.name}"? This action cannot be undone.`}
          confirmLabel="Delete"
          variant="destructive"
        />
      )}
    </div>
  )
}
