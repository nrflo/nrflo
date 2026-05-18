import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Plus, Pencil, Trash2, FileCode } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { Dialog, DialogHeader, DialogBody } from '@/components/ui/Dialog'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { ReadOnlyHint } from '@/components/auth/ReadOnlyHint'
import { PythonScriptForm, type FormData } from '@/components/pythonScripts/PythonScriptForm'
import { PythonToolForm, type ToolFormData } from '@/components/pythonScripts/PythonToolForm'
import {
  usePythonScripts,
  useCreatePythonScript,
  useUpdatePythonScript,
  useDeletePythonScript,
} from '@/hooks/usePythonScripts'
import { useIsAdmin } from '@/stores/authStore'
import type { PythonScript, PythonScriptCreateRequest, PythonScriptUpdateRequest, PythonToolCreateRequest, PythonToolUpdateRequest, ValidationResult } from '@/types/pythonScript'

type ActiveKind = 'agent' | 'tool'
type FormDialog = { type: 'none' } | { type: 'create' } | { type: 'edit'; script: PythonScript }
type SaveAnyway = { editId?: string; data: FormData | ToolFormData; message: string }

export function PythonScriptsPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const kindParam = searchParams.get('kind')
  const activeKind: ActiveKind = kindParam === 'tool' ? 'tool' : 'agent'

  const [formDialog, setFormDialog] = useState<FormDialog>({ type: 'none' })
  const [saveAnyway, setSaveAnyway] = useState<SaveAnyway | null>(null)
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)
  const isAdmin = useIsAdmin()

  const { data: scripts = [], isLoading, error } = usePythonScripts(activeKind)
  const createMutation = useCreatePythonScript()
  const updateMutation = useUpdatePythonScript()
  const deleteMutation = useDeletePythonScript()

  const setKind = (kind: ActiveKind) => {
    setSearchParams({ kind }, { replace: true })
    setFormDialog({ type: 'none' })
  }

  const closeForm = () => setFormDialog({ type: 'none' })

  const handleCreate = (data: FormData | ToolFormData) => {
    createMutation.mutate(data as PythonScriptCreateRequest | PythonToolCreateRequest, {
      onSuccess: closeForm,
    })
  }

  const handleUpdate = (id: string) => (data: FormData | ToolFormData) => {
    updateMutation.mutate({ id, data: data as PythonScriptUpdateRequest | PythonToolUpdateRequest }, {
      onSuccess: closeForm,
    })
  }

  const handleValidationFailure = (editId?: string) => (result: ValidationResult, data: FormData | ToolFormData) => {
    const msg = result.line !== undefined
      ? `Line ${result.line}, col ${result.col ?? 0}: ${result.error}`
      : (result.error ?? 'Error')
    setSaveAnyway({ editId, data, message: msg })
  }

  const handleSaveAnyway = () => {
    if (!saveAnyway) return
    if (saveAnyway.editId) {
      updateMutation.mutate(
        { id: saveAnyway.editId, data: saveAnyway.data as PythonScriptUpdateRequest | PythonToolUpdateRequest },
        { onSuccess: () => { setSaveAnyway(null); closeForm() } }
      )
    } else {
      createMutation.mutate(saveAnyway.data as PythonScriptCreateRequest | PythonToolCreateRequest, {
        onSuccess: () => { setSaveAnyway(null); closeForm() },
      })
    }
  }

  const handleDelete = (id: string) => {
    deleteMutation.mutate(id, {
      onSuccess: () => setDeleteConfirmId(null),
    })
  }

  const formatDate = (iso: string) =>
    new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })

  const isFormOpen = formDialog.type !== 'none'
  const deleteTarget = scripts.find((s) => s.id === deleteConfirmId)

  const cardDescription = activeKind === 'agent'
    ? 'Reusable Python scripts for script-mode agent definitions'
    : 'Python tool implementations callable by API-mode agents'
  const newButtonLabel = activeKind === 'agent' ? 'New Script' : 'New Tool'
  const emptyMessage = activeKind === 'agent' ? 'No agent scripts yet.' : 'No Python tools yet.'
  const dialogTitle = activeKind === 'agent'
    ? (formDialog.type === 'create' ? 'New Agent Script' : 'Edit Agent Script')
    : (formDialog.type === 'create' ? 'New Python Tool' : 'Edit Python Tool')

  return (
    <div className="p-6 max-w-5xl mx-auto space-y-4">
      {!isAdmin && <ReadOnlyHint />}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <FileCode className="h-5 w-5" />
                Python Scripts
              </CardTitle>
              <CardDescription>{cardDescription}</CardDescription>
            </div>
            {isAdmin && (
              <Button onClick={() => setFormDialog({ type: 'create' })} disabled={isFormOpen}>
                <Plus className="h-4 w-4 mr-2" />
                {newButtonLabel}
              </Button>
            )}
          </div>
          <div className="flex gap-1 mt-3">
            {(['agent', 'tool'] as ActiveKind[]).map((k) => (
              <button
                key={k}
                type="button"
                onClick={() => setKind(k)}
                className={cn(
                  'px-3 py-1 rounded-md text-sm font-medium transition-colors',
                  activeKind === k
                    ? 'bg-primary/10 text-primary'
                    : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                )}
              >
                {k === 'agent' ? 'Agents' : 'Tools'}
              </button>
            ))}
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {isLoading && <p className="text-center py-8 text-muted-foreground">Loading…</p>}
            {error && <p className="text-center py-8 text-destructive">Error: {(error as Error).message}</p>}
            {!isLoading && !error && scripts.length === 0 && (
              <p className="text-center py-8 text-muted-foreground">{emptyMessage}</p>
            )}
            {scripts.map((script: PythonScript) => (
              <div key={script.id} className="border border-border rounded-lg p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <span className="font-medium text-sm">{script.name}</span>
                    {script.description && (
                      <div className="text-xs text-muted-foreground mt-0.5 truncate max-w-lg">
                        {script.description}
                      </div>
                    )}
                    <div className="text-xs text-muted-foreground mt-0.5">
                      Updated {formatDate(script.updated_at)}
                    </div>
                  </div>
                  {isAdmin && (
                    <div className="flex gap-1 shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => setFormDialog({ type: 'edit', script })}
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => setDeleteConfirmId(script.id)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      <Dialog open={isFormOpen} onClose={closeForm} className="max-w-3xl">
        <DialogHeader onClose={closeForm}>
          <h3 className="text-lg font-semibold">{dialogTitle}</h3>
        </DialogHeader>
        <DialogBody>
          {formDialog.type === 'create' && activeKind === 'agent' && (
            <PythonScriptForm
              isCreate
              onSubmit={handleCreate}
              onValidationFailure={handleValidationFailure(undefined)}
              onCancel={closeForm}
              isPending={createMutation.isPending}
            />
          )}
          {formDialog.type === 'edit' && activeKind === 'agent' && (
            <PythonScriptForm
              initial={formDialog.script}
              isCreate={false}
              onSubmit={handleUpdate(formDialog.script.id)}
              onValidationFailure={handleValidationFailure(formDialog.script.id)}
              onCancel={closeForm}
              isPending={updateMutation.isPending}
            />
          )}
          {formDialog.type === 'create' && activeKind === 'tool' && (
            <PythonToolForm
              isCreate
              onSubmit={handleCreate}
              onValidationFailure={handleValidationFailure(undefined)}
              onCancel={closeForm}
              isPending={createMutation.isPending}
            />
          )}
          {formDialog.type === 'edit' && activeKind === 'tool' && (
            <PythonToolForm
              initial={formDialog.script}
              isCreate={false}
              onSubmit={handleUpdate(formDialog.script.id)}
              onValidationFailure={handleValidationFailure(formDialog.script.id)}
              onCancel={closeForm}
              isPending={updateMutation.isPending}
            />
          )}
        </DialogBody>
      </Dialog>

      <ConfirmDialog
        open={!!saveAnyway}
        onClose={() => setSaveAnyway(null)}
        onConfirm={handleSaveAnyway}
        title="Validation error"
        message={saveAnyway ? `${saveAnyway.message}\n\nSave anyway?` : ''}
        confirmLabel="Save anyway"
      />

      <ConfirmDialog
        open={!!deleteConfirmId}
        onClose={() => setDeleteConfirmId(null)}
        onConfirm={() => deleteConfirmId && handleDelete(deleteConfirmId)}
        title={activeKind === 'agent' ? 'Delete Agent Script' : 'Delete Python Tool'}
        message={`Delete '${deleteTarget?.name ?? deleteConfirmId}'? Agent definitions referencing this will need to be updated.`}
        confirmLabel="Delete"
        variant="destructive"
      />
    </div>
  )
}
