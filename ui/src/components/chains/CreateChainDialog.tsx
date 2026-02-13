import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link2 } from 'lucide-react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Select } from '@/components/ui/Select'
import { Spinner } from '@/components/ui/Spinner'
import { ChainTicketSelector } from './ChainTicketSelector'
import { listWorkflowDefs } from '@/api/workflows'
import { useCreateChain, useUpdateChain } from '@/hooks/useChains'
import { useProjectStore } from '@/stores/projectStore'
import type { ChainExecution } from '@/types/chain'

interface CreateChainDialogProps {
  open: boolean
  onClose: () => void
  editChain?: ChainExecution | null
}

export function CreateChainDialog({ open, onClose, editChain }: CreateChainDialogProps) {
  const [name, setName] = useState('')
  const [selectedWorkflow, setSelectedWorkflow] = useState('')
  const [category, setCategory] = useState('')
  const [ticketIds, setTicketIds] = useState<string[]>([])

  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)

  const { data: workflowDefs, isLoading: defsLoading } = useQuery({
    queryKey: ['workflows', 'defs', project],
    queryFn: listWorkflowDefs,
    enabled: open && projectsLoaded,
  })

  const createMutation = useCreateChain()
  const updateMutation = useUpdateChain()
  const isEditing = !!editChain

  const workflowIds = workflowDefs ? Object.keys(workflowDefs).filter((id) => {
    const def = workflowDefs[id]
    return !def.scope_type || def.scope_type === 'ticket'
  }) : []

  // Auto-select first workflow
  useEffect(() => {
    if (workflowIds.length > 0 && !selectedWorkflow) {
      setSelectedWorkflow(workflowIds[0])
    }
  }, [workflowIds, selectedWorkflow])

  // Categories for selected workflow
  const selectedDef = selectedWorkflow && workflowDefs ? workflowDefs[selectedWorkflow] : null
  const categories = selectedDef?.categories ?? []

  useEffect(() => {
    if (categories.length > 0 && !category) {
      setCategory(categories[0])
    }
  }, [selectedWorkflow, categories, category])

  // Populate from editChain
  useEffect(() => {
    if (editChain) {
      setName(editChain.name)
      setSelectedWorkflow(editChain.workflow_name)
      setCategory(editChain.category ?? '')
      setTicketIds(editChain.items?.map((i) => i.ticket_id) ?? [])
    }
  }, [editChain])

  // Reset on close
  useEffect(() => {
    if (!open) {
      setName('')
      setSelectedWorkflow('')
      setCategory('')
      setTicketIds([])
    }
  }, [open])

  const handleSubmit = async () => {
    if (!name.trim() || !selectedWorkflow || ticketIds.length === 0) return
    try {
      if (isEditing && editChain) {
        await updateMutation.mutateAsync({
          id: editChain.id,
          data: { name: name.trim(), ticket_ids: ticketIds },
        })
      } else {
        await createMutation.mutateAsync({
          name: name.trim(),
          workflow_name: selectedWorkflow,
          category: category || undefined,
          ticket_ids: ticketIds,
        })
      }
      onClose()
    } catch {
      // Error handled by mutation state
    }
  }

  const isPending = createMutation.isPending || updateMutation.isPending
  const mutationError = createMutation.error || updateMutation.error
  const canSubmit = name.trim() && selectedWorkflow && ticketIds.length > 0 && !isPending

  return (
    <Dialog open={open} onClose={onClose}>
      <DialogHeader onClose={onClose}>
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <Link2 className="h-5 w-5" />
          {isEditing ? 'Edit Chain' : 'Create Chain'}
        </h2>
      </DialogHeader>

      <DialogBody className="space-y-4">
        {defsLoading ? (
          <div className="flex justify-center py-8">
            <Spinner />
          </div>
        ) : workflowIds.length === 0 ? (
          <p className="text-muted-foreground text-sm text-center py-4">
            No ticket-scoped workflow definitions found. Create one on the Workflows page.
          </p>
        ) : (
          <>
            <div>
              <label htmlFor="chain-name" className="block text-sm font-medium mb-1.5">Name</label>
              <Input
                id="chain-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Chain name..."
              />
            </div>

            <div>
              <label htmlFor="chain-workflow" className="block text-sm font-medium mb-1.5">Workflow</label>
              <Select
                id="chain-workflow"
                value={selectedWorkflow}
                onChange={(e) => setSelectedWorkflow(e.target.value)}
                disabled={isEditing}
              >
                {workflowIds.map((id) => (
                  <option key={id} value={id}>
                    {id}
                    {workflowDefs![id].description ? ` - ${workflowDefs![id].description}` : ''}
                  </option>
                ))}
              </Select>
            </div>

            {categories.length > 0 && (
              <div>
                <label htmlFor="chain-category" className="block text-sm font-medium mb-1.5">Category</label>
                <Select
                  id="chain-category"
                  value={category}
                  onChange={(e) => setCategory(e.target.value)}
                  disabled={isEditing}
                >
                  {categories.map((cat) => (
                    <option key={cat} value={cat}>{cat}</option>
                  ))}
                </Select>
              </div>
            )}

            <div>
              <label className="block text-sm font-medium mb-1.5">Tickets</label>
              <ChainTicketSelector
                selectedIds={ticketIds}
                onChange={setTicketIds}
              />
            </div>
          </>
        )}

        {mutationError && (
          <p className="text-sm text-destructive">
            {mutationError instanceof Error ? mutationError.message : 'Operation failed'}
          </p>
        )}
      </DialogBody>

      <DialogFooter>
        <Button variant="ghost" onClick={onClose}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={!canSubmit}>
          {isPending && <Spinner size="sm" className="mr-2" />}
          {isEditing ? 'Update' : 'Create'}
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
