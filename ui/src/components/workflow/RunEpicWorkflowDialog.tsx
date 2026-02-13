import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Layers, Play } from 'lucide-react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Select } from '@/components/ui/Select'
import { Spinner } from '@/components/ui/Spinner'
import { Badge } from '@/components/ui/Badge'
import { listWorkflowDefs } from '@/api/workflows'
import { useRunEpicWorkflow, useStartChain, useCancelChain } from '@/hooks/useChains'
import { useProjectStore } from '@/stores/projectStore'
import type { ChainExecution } from '@/types/chain'

interface RunEpicWorkflowDialogProps {
  open: boolean
  onClose: () => void
  ticketId: string
  ticketTitle: string
}

export function RunEpicWorkflowDialog({
  open,
  onClose,
  ticketId,
  ticketTitle,
}: RunEpicWorkflowDialogProps) {
  const navigate = useNavigate()
  const [selectedWorkflow, setSelectedWorkflow] = useState('')
  const [category, setCategory] = useState('full')
  const [pendingChain, setPendingChain] = useState<ChainExecution | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)

  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)

  const { data: workflowDefs, isLoading: defsLoading } = useQuery({
    queryKey: ['workflows', 'defs', project],
    queryFn: listWorkflowDefs,
    enabled: open && projectsLoaded,
  })

  const epicMutation = useRunEpicWorkflow()
  const startMutation = useStartChain()
  const cancelMutation = useCancelChain()

  // Filter to ticket-scoped workflows only
  const workflowIds = workflowDefs
    ? Object.keys(workflowDefs).filter((id) => {
        const def = workflowDefs[id]
        return !def.scope_type || def.scope_type === 'ticket'
      })
    : []

  // Auto-select first workflow
  useEffect(() => {
    if (workflowIds.length > 0 && !selectedWorkflow) {
      setSelectedWorkflow(workflowIds[0])
    }
  }, [workflowIds, selectedWorkflow])

  // Categories for selected workflow
  const selectedDef = selectedWorkflow && workflowDefs ? workflowDefs[selectedWorkflow] : null
  const categories = selectedDef?.categories ?? ['full']

  // Reset category when workflow changes
  useEffect(() => {
    if (categories.length > 0) {
      setCategory(categories[0] || 'full')
    }
  }, [selectedWorkflow])

  const handlePreview = async () => {
    if (!selectedWorkflow) return
    setPreviewError(null)
    try {
      const chain = await epicMutation.mutateAsync({
        ticketId,
        params: {
          workflow_name: selectedWorkflow,
          category: category || undefined,
          start: false,
        },
      })
      setPendingChain(chain)
    } catch (err) {
      setPreviewError(err instanceof Error ? err.message : 'Failed to create chain preview')
    }
  }

  const handleRunNow = async () => {
    if (!pendingChain) return
    try {
      await startMutation.mutateAsync(pendingChain.id)
      navigate(`/chains/${encodeURIComponent(pendingChain.id)}`)
      onClose()
    } catch {
      // Error displayed via mutation state
    }
  }

  const handleClose = async () => {
    if (pendingChain) {
      try {
        await cancelMutation.mutateAsync(pendingChain.id)
      } catch {
        // Best-effort cleanup
      }
    }
    onClose()
  }

  // Reset state when dialog closes
  useEffect(() => {
    if (!open) {
      setSelectedWorkflow('')
      setCategory('full')
      setPendingChain(null)
      setPreviewError(null)
    }
  }, [open])

  const items = pendingChain?.items ?? []
  const isWorking = epicMutation.isPending || startMutation.isPending || cancelMutation.isPending

  return (
    <Dialog open={open} onClose={handleClose} className="max-w-lg">
      <DialogHeader onClose={handleClose}>
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <Layers className="h-5 w-5" />
          Run Epic Workflow
        </h2>
        <p className="text-sm text-muted-foreground mt-1 truncate">{ticketTitle}</p>
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
        ) : !pendingChain ? (
          <>
            <div>
              <label htmlFor="epic-workflow-select" className="block text-sm font-medium mb-1.5">Workflow</label>
              <Select
                id="epic-workflow-select"
                value={selectedWorkflow}
                onChange={(e) => setSelectedWorkflow(e.target.value)}
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
                <label htmlFor="epic-category-select" className="block text-sm font-medium mb-1.5">Category</label>
                <Select
                  id="epic-category-select"
                  value={category}
                  onChange={(e) => setCategory(e.target.value)}
                >
                  {categories.map((cat) => (
                    <option key={cat} value={cat}>{cat}</option>
                  ))}
                </Select>
              </div>
            )}
          </>
        ) : (
          <>
            <div className="flex items-center gap-3 text-sm">
              <Badge variant="secondary">{pendingChain.workflow_name}</Badge>
              {pendingChain.category && (
                <Badge variant="outline">{pendingChain.category}</Badge>
              )}
              <span className="text-muted-foreground">
                {items.length} ticket{items.length !== 1 ? 's' : ''} in chain
              </span>
            </div>

            <div className="border border-border rounded-lg max-h-64 overflow-y-auto">
              {items
                .sort((a, b) => a.position - b.position)
                .map((item) => (
                  <div
                    key={item.id}
                    className="flex items-center gap-3 px-3 py-2 border-b border-border last:border-b-0"
                  >
                    <span className="text-xs font-mono text-muted-foreground w-5 text-right shrink-0">
                      {item.position + 1}
                    </span>
                    <span className="text-sm font-mono text-primary shrink-0">
                      {item.ticket_id}
                    </span>
                    {item.ticket_title && (
                      <span className="text-sm text-muted-foreground truncate min-w-0">
                        {item.ticket_title}
                      </span>
                    )}
                  </div>
                ))}
            </div>
          </>
        )}

        {previewError && (
          <p className="text-sm text-destructive">{previewError}</p>
        )}
        {startMutation.isError && (
          <p className="text-sm text-destructive">
            {startMutation.error instanceof Error
              ? startMutation.error.message
              : 'Failed to start chain'}
          </p>
        )}
      </DialogBody>

      <DialogFooter>
        <Button variant="ghost" onClick={handleClose} disabled={isWorking}>
          Cancel
        </Button>
        {!pendingChain ? (
          <Button
            onClick={handlePreview}
            disabled={!selectedWorkflow || epicMutation.isPending}
          >
            {epicMutation.isPending && <Spinner size="sm" className="mr-2" />}
            Preview Chain
          </Button>
        ) : (
          <Button onClick={handleRunNow} disabled={startMutation.isPending}>
            {startMutation.isPending && <Spinner size="sm" className="mr-2" />}
            <Play className="h-4 w-4 mr-1" />
            Run Now
          </Button>
        )}
      </DialogFooter>
    </Dialog>
  )
}
