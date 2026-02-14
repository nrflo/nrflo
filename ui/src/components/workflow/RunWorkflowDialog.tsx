import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Play } from 'lucide-react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Select } from '@/components/ui/Select'
import { Spinner } from '@/components/ui/Spinner'
import { listWorkflowDefs } from '@/api/workflows'
import { useRunWorkflow } from '@/hooks/useTickets'
import { useProjectStore } from '@/stores/projectStore'

interface RunWorkflowDialogProps {
  open: boolean
  onClose: () => void
  ticketId: string
}

export function RunWorkflowDialog({ open, onClose, ticketId }: RunWorkflowDialogProps) {
  const [selectedWorkflow, setSelectedWorkflow] = useState('')
  const [instructions, setInstructions] = useState('')

  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)

  const { data: workflowDefs, isLoading } = useQuery({
    queryKey: ['workflows', 'defs', project],
    queryFn: listWorkflowDefs,
    enabled: open && projectsLoaded,
  })

  const runMutation = useRunWorkflow()

  const workflowIds = workflowDefs ? Object.keys(workflowDefs) : []

  // Auto-select first workflow
  useEffect(() => {
    if (workflowIds.length > 0 && !selectedWorkflow) {
      setSelectedWorkflow(workflowIds[0])
    }
  }, [workflowIds, selectedWorkflow])

  const handleRun = async () => {
    if (!selectedWorkflow) return
    try {
      await runMutation.mutateAsync({
        ticketId,
        params: {
          workflow: selectedWorkflow,
          instructions: instructions || undefined,
        },
      })
      onClose()
      setInstructions('')
    } catch {
      // Error is handled by mutation state
    }
  }

  // Reset state when dialog closes
  useEffect(() => {
    if (!open) {
      setSelectedWorkflow('')
      setInstructions('')
    }
  }, [open])

  return (
    <Dialog open={open} onClose={onClose} className="max-w-lg">
      <DialogHeader onClose={onClose}>
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <Play className="h-5 w-5" />
          Run Workflow
        </h2>
      </DialogHeader>

      <DialogBody className="space-y-4">
        {isLoading ? (
          <div className="flex justify-center py-8">
            <Spinner />
          </div>
        ) : workflowIds.length === 0 ? (
          <p className="text-muted-foreground text-sm text-center py-4">
            No workflow definitions found. Create one on the Workflows page.
          </p>
        ) : (
          <>
            <div>
              <label className="block text-sm font-medium mb-1.5">Workflow</label>
              <Select
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

            <div>
              <label className="block text-sm font-medium mb-1.5">
                Instructions <span className="text-muted-foreground font-normal">(optional)</span>
              </label>
              <textarea
                value={instructions}
                onChange={(e) => setInstructions(e.target.value)}
                placeholder="Additional context or instructions for the agents..."
                rows={4}
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring resize-none"
              />
            </div>
          </>
        )}

        {runMutation.isError && (
          <p className="text-sm text-destructive">
            {runMutation.error instanceof Error
              ? runMutation.error.message
              : 'Failed to start workflow'}
          </p>
        )}
      </DialogBody>

      <DialogFooter>
        <Button variant="ghost" onClick={onClose}>
          Cancel
        </Button>
        <Button
          onClick={handleRun}
          disabled={!selectedWorkflow || runMutation.isPending}
        >
          {runMutation.isPending && <Spinner size="sm" className="mr-2" />}
          Run
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
