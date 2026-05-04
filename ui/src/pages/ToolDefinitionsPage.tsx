import { useState } from 'react'
import { Plus, Pencil, Trash2, Wrench } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { ToolDefinitionForm } from '@/components/toolDefinitions/ToolDefinitionForm'
import { ReadOnlyHint } from '@/components/auth/ReadOnlyHint'
import {
  useToolDefinitions,
  useCreateToolDefinition,
  useUpdateToolDefinition,
  useDeleteToolDefinition,
} from '@/hooks/useToolDefinitions'
import { useProjects } from '@/hooks/useProjects'
import { useIsAdmin } from '@/stores/authStore'
import type { ToolDefinition, ToolDefinitionCreateRequest, ToolDefinitionUpdateRequest } from '@/types/toolDefinition'

export function ToolDefinitionsPage() {
  const [isCreating, setIsCreating] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)
  const isAdmin = useIsAdmin()

  const { data: toolDefs = [], isLoading, error } = useToolDefinitions()
  const { data: projectsData } = useProjects()
  const createMutation = useCreateToolDefinition()
  const updateMutation = useUpdateToolDefinition()
  const deleteMutation = useDeleteToolDefinition()

  const projectName = (id?: string) => {
    if (!id) return 'Global'
    return projectsData?.projects.find((p) => p.id === id)?.name ?? id
  }

  const handleCreate = (data: ToolDefinitionCreateRequest | ToolDefinitionUpdateRequest) => {
    createMutation.mutate(data as ToolDefinitionCreateRequest, {
      onSuccess: () => setIsCreating(false),
    })
  }

  const handleUpdate = (id: string) => (data: ToolDefinitionCreateRequest | ToolDefinitionUpdateRequest) => {
    updateMutation.mutate({ id, data: data as ToolDefinitionUpdateRequest }, {
      onSuccess: () => setEditingId(null),
    })
  }

  const handleDelete = (id: string) => {
    deleteMutation.mutate(id, {
      onSuccess: () => setDeleteConfirmId(null),
    })
  }

  return (
    <div className="p-6 max-w-5xl mx-auto space-y-4">
      {!isAdmin && <ReadOnlyHint />}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <Wrench className="h-5 w-5" />
                Tool Definitions
              </CardTitle>
              <CardDescription>HTTP tools callable by API-mode agents</CardDescription>
            </div>
            {isAdmin && (
              <Button onClick={() => { setIsCreating(true); setEditingId(null) }} disabled={isCreating}>
                <Plus className="h-4 w-4 mr-2" />
                New Tool Definition
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {isLoading && <p className="text-center py-8 text-muted-foreground">Loading…</p>}
            {error && <p className="text-center py-8 text-destructive">Error: {(error as Error).message}</p>}
            {isAdmin && isCreating && (
              <ToolDefinitionForm
                isCreate
                onSubmit={handleCreate}
                onCancel={() => setIsCreating(false)}
                isPending={createMutation.isPending}
              />
            )}
            {!isLoading && !error && toolDefs.length === 0 && !isCreating && (
              <p className="text-center py-8 text-muted-foreground">No tool definitions yet.</p>
            )}
            {toolDefs.map((def: ToolDefinition) => (
              <div key={def.id} className="border rounded-lg p-4">
                {isAdmin && editingId === def.id ? (
                  <ToolDefinitionForm
                    initial={def}
                    isCreate={false}
                    onSubmit={handleUpdate(def.id)}
                    onCancel={() => setEditingId(null)}
                    isPending={updateMutation.isPending}
                  />
                ) : (
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-sm">{def.name}</span>
                        <span className="text-xs text-muted-foreground px-1.5 py-0.5 rounded bg-muted">{def.auth_method}</span>
                      </div>
                      <div className="text-xs text-muted-foreground mt-0.5 truncate max-w-lg">{def.endpoint}</div>
                      <div className="text-xs text-muted-foreground mt-0.5">
                        Project: {projectName(def.project_id ?? undefined)}
                        {def.workflow_id && ` · Workflow: ${def.workflow_id}`}
                        {` · Timeout: ${def.timeout_sec}s`}
                      </div>
                    </div>
                    {isAdmin && (
                      <div className="flex gap-1 shrink-0">
                        <Button variant="ghost" size="icon" onClick={() => { setEditingId(def.id); setIsCreating(false) }}>
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button variant="ghost" size="icon" onClick={() => setDeleteConfirmId(def.id)}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
      <ConfirmDialog
        open={!!deleteConfirmId}
        onClose={() => setDeleteConfirmId(null)}
        onConfirm={() => deleteConfirmId && handleDelete(deleteConfirmId)}
        title="Delete Tool Definition"
        message={`Delete tool definition '${deleteConfirmId}'?`}
        confirmLabel="Delete"
        variant="destructive"
      />
    </div>
  )
}
