import { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link2 } from 'lucide-react'
import { generateChainName } from '@/lib/generateChainName'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Dropdown } from '@/components/ui/Dropdown'
import { Spinner } from '@/components/ui/Spinner'
import { ChainTicketSelector } from './ChainTicketSelector'
import { ChainOrderList } from './ChainOrderList'
import { listWorkflowDefs } from '@/api/workflows'
import { previewChain } from '@/api/chains'
import { useCreateChain, useUpdateChain } from '@/hooks/useChains'
import { useTicketList } from '@/hooks/useTickets'
import { useProjectStore } from '@/stores/projectStore'
import type { ChainExecution } from '@/types/chain'

interface CreateChainDialogProps {
  open: boolean
  onClose: () => void
  editChain?: ChainExecution | null
}

export function CreateChainDialog({ open, onClose, editChain }: CreateChainDialogProps) {
  const [name, setName] = useState(() => generateChainName())
  const [selectedWorkflow, setSelectedWorkflow] = useState('')
  const [ticketIds, setTicketIds] = useState<string[]>([])
  const [epicIds, setEpicIds] = useState<string[]>([])
  const [orderedIds, setOrderedIds] = useState<string[]>([])
  const [deps, setDeps] = useState<Record<string, string[]>>({})
  const [addedByDeps, setAddedByDeps] = useState<string[]>([])
  const previewTimerRef = useRef<number | null>(null)
  const previewRequestRef = useRef(0)

  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)

  const { data: workflowDefs, isLoading: defsLoading } = useQuery({
    queryKey: ['workflows', 'defs', project],
    queryFn: listWorkflowDefs,
    enabled: open && projectsLoaded,
  })

  const { data: ticketData } = useTicketList({ status: 'open' }, { enabled: open && projectsLoaded })

  const ticketTitleMap = useMemo(() => {
    const map = new Map<string, string>()
    for (const t of ticketData?.tickets ?? []) {
      map.set(t.id, t.title)
    }
    return map
  }, [ticketData])

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

  // Populate from editChain
  useEffect(() => {
    if (editChain) {
      setName(editChain.name)
      setSelectedWorkflow(editChain.workflow_name)
      const itemIds = editChain.items
        ?.slice()
        .sort((a, b) => a.position - b.position)
        .map((i) => i.ticket_id) ?? []
      setTicketIds(itemIds)
      setOrderedIds(itemIds)
      setDeps(editChain.deps ?? {})
      // Fetch preview to get addedByDeps for edit mode
      if (itemIds.length > 0) {
        previewChain({ ticket_ids: itemIds }).then((res) => {
          setAddedByDeps(res.added_by_deps)
        }).catch(() => {})
      }
    }
  }, [editChain])

  // Reset on close
  useEffect(() => {
    if (!open) {
      setName(generateChainName())
      setSelectedWorkflow('')
      setTicketIds([])
      setEpicIds([])
      setOrderedIds([])
      setDeps({})
      setAddedByDeps([])
      if (previewTimerRef.current) {
        clearTimeout(previewTimerRef.current)
        previewTimerRef.current = null
      }
    }
  }, [open])

  // Debounced preview call on ticket selection change
  const fetchPreview = useCallback((nonEpicIds: string[]) => {
    if (previewTimerRef.current) {
      clearTimeout(previewTimerRef.current)
    }
    if (nonEpicIds.length === 0) {
      setOrderedIds([])
      setDeps({})
      setAddedByDeps([])
      return
    }
    const requestId = ++previewRequestRef.current
    previewTimerRef.current = window.setTimeout(async () => {
      try {
        const res = await previewChain({ ticket_ids: nonEpicIds })
        // Ignore stale responses
        if (requestId !== previewRequestRef.current) return
        setOrderedIds(res.ticket_ids)
        setDeps(res.deps)
        setAddedByDeps(res.added_by_deps)
      } catch {
        // Preview failure is non-blocking
      }
    }, 300)
  }, [])

  // Trigger preview on ticket selection changes (skip for initial editChain population)
  const prevTicketIdsRef = useRef<string[] | null>(null)
  useEffect(() => {
    // Skip the first render when populated from editChain
    if (prevTicketIdsRef.current === null) {
      prevTicketIdsRef.current = ticketIds
      if (editChain) return
    }
    prevTicketIdsRef.current = ticketIds
    const epicSet = new Set(epicIds)
    const nonEpicIds = ticketIds.filter((id) => !epicSet.has(id))
    fetchPreview(nonEpicIds)
  }, [ticketIds, epicIds, fetchPreview, editChain])

  // Derive ChainOrderList items from orderedIds + ticketTitleMap
  const orderItems = useMemo(
    () =>
      orderedIds.map((id) => ({
        ticketId: id,
        title: ticketTitleMap.get(id) ?? id,
      })),
    [orderedIds, ticketTitleMap]
  )

  const handleReorder = useCallback((newIds: string[]) => {
    setOrderedIds(newIds)
  }, [])

  const handleSubmit = async () => {
    if (!name.trim() || !selectedWorkflow || ticketIds.length === 0) return
    // Exclude epic IDs from ticket_ids — epics aren't chain items
    const epicSet = new Set(epicIds)
    const childOnlyIds = ticketIds.filter((id) => !epicSet.has(id))
    const finalTicketIds = childOnlyIds.length > 0 ? childOnlyIds : ticketIds
    try {
      if (isEditing && editChain) {
        await updateMutation.mutateAsync({
          id: editChain.id,
          data: {
            name: name.trim(),
            ticket_ids: finalTicketIds,
            ordered_ticket_ids: orderedIds.length > 0 ? orderedIds : undefined,
          },
        })
      } else {
        await createMutation.mutateAsync({
          name: name.trim(),
          workflow_name: selectedWorkflow,
          ticket_ids: finalTicketIds,
          epic_ticket_id: epicIds.length === 1 ? epicIds[0] : undefined,
          ordered_ticket_ids: orderedIds.length > 0 ? orderedIds : undefined,
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
              <Dropdown
                value={selectedWorkflow}
                onChange={setSelectedWorkflow}
                disabled={isEditing}
                options={workflowIds.map((id) => ({
                  value: id,
                  label: id + (workflowDefs![id].description ? ` - ${workflowDefs![id].description}` : ''),
                }))}
              />
            </div>

            <div>
              <label className="block text-sm font-medium mb-1.5">Tickets</label>
              <ChainTicketSelector
                selectedIds={ticketIds}
                onChange={setTicketIds}
                onEpicIdsChange={setEpicIds}
              />
            </div>

            {orderedIds.length > 0 && (
              <ChainOrderList
                items={orderItems}
                deps={deps}
                addedByDeps={addedByDeps}
                onReorder={handleReorder}
              />
            )}
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
