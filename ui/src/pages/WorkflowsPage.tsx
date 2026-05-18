import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Download, Upload } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { WorkflowDefForm } from '@/components/workflow/WorkflowDefForm'
import { WorkflowImportDialog } from '@/components/workflow/WorkflowImportDialog'
import { WorkflowCard } from './WorkflowCard'
import {
  listWorkflowDefs,
  createWorkflowDef,
  updateWorkflowDef,
  deleteWorkflowDef,
  exportWorkflow,
  exportAllWorkflows,
} from '@/api/workflows'
import type { WorkflowDefSummary, WorkflowDefCreateRequest, WorkflowDefUpdateRequest, ScopeType } from '@/types/workflow'
import { useProjectStore } from '@/stores/projectStore'
import { triggerDownload, fallbackExportFilename } from '@/lib/downloadBlob'
import { pythonScriptKeys } from '@/hooks/usePythonScripts'

interface EditingWorkflow {
  id: string
  description?: string
  scope_type?: ScopeType
  groups?: string[]
  close_ticket_on_complete?: boolean
  next_workflow_on_success?: string
}

export function WorkflowsPage() {
  const queryClient = useQueryClient()
  const currentProject = useProjectStore((s) => s.currentProject)
  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [editingWorkflow, setEditingWorkflow] = useState<EditingWorkflow | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [showImportDialog, setShowImportDialog] = useState(false)

  const queryKey = ['workflow-defs', currentProject] as const

  const { data: workflows, isLoading, error } = useQuery({
    queryKey,
    queryFn: listWorkflowDefs,
  })

  const createMutation = useMutation({
    mutationFn: (data: WorkflowDefCreateRequest) => createWorkflowDef(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey })
      setShowCreateDialog(false)
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: WorkflowDefUpdateRequest }) =>
      updateWorkflowDef(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey })
      setEditingWorkflow(null)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteWorkflowDef(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey })
    },
  })

  const handleExport = async (id: string) => {
    const { blob, filename } = await exportWorkflow(id)
    triggerDownload(blob, filename ?? `nrflo-workflow-${id}.json`)
  }

  const handleExportAll = async () => {
    const { blob, filename } = await exportAllWorkflows()
    triggerDownload(blob, filename ?? fallbackExportFilename(currentProject))
  }

  const handleImportSuccess = () => {
    queryClient.invalidateQueries({ queryKey })
    queryClient.invalidateQueries({ queryKey: pythonScriptKeys.all })
    setShowImportDialog(false)
  }

  return (
    <div className="max-w-7xl mx-auto">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Workflows</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Workflow definitions and their agent configurations.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={handleExportAll}>
            <Download className="h-4 w-4 mr-1" />
            Export All
          </Button>
          <Button variant="outline" size="sm" onClick={() => setShowImportDialog(true)}>
            <Upload className="h-4 w-4 mr-1" />
            Import
          </Button>
          <Button size="sm" onClick={() => setShowCreateDialog(true)}>
            <Plus className="h-4 w-4 mr-1" />
            Create Workflow
          </Button>
        </div>
      </div>

      {isLoading && <p className="text-muted-foreground">Loading workflows...</p>}
      {error && <p className="text-destructive">Failed to load workflows: {String(error)}</p>}

      {workflows && Object.keys(workflows).length === 0 && (
        <div className="text-center py-12 text-muted-foreground">
          <p>No workflow definitions found.</p>
        </div>
      )}

      <div className="space-y-3">
        {workflows &&
          Object.entries(workflows).map(([id, def]: [string, WorkflowDefSummary]) => (
            <WorkflowCard
              key={id}
              id={id}
              def={def}
              onEdit={() => setEditingWorkflow({ id, ...def })}
              onDelete={() => setDeleteTarget(id)}
              onExport={() => handleExport(id)}
            />
          ))}
      </div>

      {/* Create Dialog */}
      <Dialog open={showCreateDialog} onClose={() => setShowCreateDialog(false)} className="max-w-[85vw]">
        <DialogHeader onClose={() => setShowCreateDialog(false)}>
          <h2 className="text-lg font-semibold">Create Workflow</h2>
        </DialogHeader>
        <DialogBody>
          <WorkflowDefForm
            formId="create-workflow-form"
            isCreate
            onSubmit={(data) => createMutation.mutate(data as WorkflowDefCreateRequest)}
          />
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={() => setShowCreateDialog(false)}>Cancel</Button>
          <Button type="submit" form="create-workflow-form" size="sm" disabled={createMutation.isPending}>
            {createMutation.isPending ? 'Saving...' : 'Create Workflow'}
          </Button>
        </DialogFooter>
      </Dialog>

      {/* Delete Confirm Dialog */}
      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget)}
        title="Delete Workflow"
        message={`Delete workflow '${deleteTarget}'? This cannot be undone.`}
        confirmLabel="Delete"
        variant="destructive"
      />

      {/* Import Dialog */}
      <WorkflowImportDialog
        open={showImportDialog}
        onClose={() => setShowImportDialog(false)}
        onSuccess={handleImportSuccess}
      />

      {/* Edit Dialog */}
      <Dialog open={!!editingWorkflow} onClose={() => setEditingWorkflow(null)} className="max-w-[85vw]">
        <DialogHeader onClose={() => setEditingWorkflow(null)}>
          <h2 className="text-lg font-semibold">
            Edit Workflow: {editingWorkflow?.id}
          </h2>
        </DialogHeader>
        <DialogBody>
          {editingWorkflow && (
            <WorkflowDefForm
              formId="edit-workflow-form"
              key={editingWorkflow.id}
              initial={editingWorkflow}
              isCreate={false}
              onSubmit={(data) =>
                updateMutation.mutate({
                  id: editingWorkflow.id,
                  data: data as WorkflowDefUpdateRequest,
                })
              }
            />
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={() => setEditingWorkflow(null)}>Cancel</Button>
          <Button type="submit" form="edit-workflow-form" size="sm" disabled={updateMutation.isPending}>
            {updateMutation.isPending ? 'Saving...' : 'Save Changes'}
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  )
}
