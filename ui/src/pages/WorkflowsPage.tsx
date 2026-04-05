import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, Plus, Pencil, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { AgentDefsSection } from '@/components/workflow/AgentDefsSection'
import { WorkflowDefForm } from '@/components/workflow/WorkflowDefForm'
import { listWorkflowDefs, createWorkflowDef, updateWorkflowDef, deleteWorkflowDef } from '@/api/workflows'
import type { WorkflowDefSummary, WorkflowDefCreateRequest, WorkflowDefUpdateRequest, PhaseDef, ScopeType } from '@/types/workflow'
import { useProjectStore } from '@/stores/projectStore'
import { cn } from '@/lib/utils'

function GroupBadges({ groups }: { groups?: string[] }) {
  if (!groups?.length) return null
  return (
    <>
      {groups.map((g) => (
        <Badge key={g} variant="outline" className="text-xs border-emerald-300 text-emerald-600">
          {g}
        </Badge>
      ))}
    </>
  )
}

function WorkflowCard({
  id,
  def,
  onEdit,
  onDelete,
}: {
  id: string
  def: WorkflowDefSummary
  onEdit: () => void
  onDelete: () => void
}) {
  const [expanded, setExpanded] = useState(false)

  // Group agents by layer for display
  const phases = (() => {
    if (!def.phases?.length) return ''
    const byLayer: Record<number, string[]> = {}
    for (const p of def.phases) {
      const layer = p.layer ?? 0
      if (!byLayer[layer]) byLayer[layer] = []
      byLayer[layer].push(p.agent || p.id)
    }
    return Object.keys(byLayer)
      .map(Number)
      .sort((a, b) => a - b)
      .map((l) => byLayer[l].length > 1 ? `[${byLayer[l].join(' | ')}]` : byLayer[l][0])
      .join(' -> ')
  })()

  return (
    <div className="border border-border rounded-lg overflow-hidden">
      <div
        className={cn(
          'w-full p-4 flex items-center justify-between text-left hover:bg-muted/30 transition-colors',
          expanded && 'border-b border-border'
        )}
      >
        <button
          className="flex items-center gap-3 flex-1 text-left"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          )}
          <div>
            <div className="font-medium">{id}</div>
            {def.description && (
              <p className="text-xs text-muted-foreground mt-0.5">{def.description}</p>
            )}
          </div>
        </button>
        <div className="flex items-center gap-2">
          {def.scope_type === 'project' && (
            <Badge variant="outline" className="text-xs border-blue-300 text-blue-600">
              project
            </Badge>
          )}
          <GroupBadges groups={def.groups} />
          <span className="text-xs text-muted-foreground">
            {def.phases?.length || 0} agents
          </span>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0"
            onClick={onEdit}
            title="Edit workflow"
          >
            <Pencil className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0 text-destructive hover:text-destructive"
            onClick={onDelete}
            title="Delete workflow"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {expanded && (
        <div className="p-4 space-y-4">
          {phases && (
            <div>
              <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-1">
                Agent Pipeline
              </h4>
              <p className="text-sm font-mono">{phases}</p>
            </div>
          )}

          <AgentDefsSection workflowId={id} groups={def.groups || []} />
        </div>
      )}
    </div>
  )
}

interface EditingWorkflow {
  id: string
  description?: string
  scope_type?: ScopeType
  groups?: string[]
  phases?: PhaseDef[]
}

export function WorkflowsPage() {
  const queryClient = useQueryClient()
  const currentProject = useProjectStore((s) => s.currentProject)
  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [editingWorkflow, setEditingWorkflow] = useState<EditingWorkflow | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)

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

  const handleDelete = (id: string) => {
    setDeleteTarget(id)
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
        <Button size="sm" onClick={() => setShowCreateDialog(true)}>
          <Plus className="h-4 w-4 mr-1" />
          Create Workflow
        </Button>
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
          Object.entries(workflows).map(([id, def]) => (
            <WorkflowCard
              key={id}
              id={id}
              def={def}
              onEdit={() => setEditingWorkflow({ id, ...def })}
              onDelete={() => handleDelete(id)}
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
