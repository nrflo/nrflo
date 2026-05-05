import { useState } from 'react'
import { Plus, Pencil, Trash2, FileCode } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { Dialog, DialogHeader, DialogBody } from '@/components/ui/Dialog'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { ReadOnlyHint } from '@/components/auth/ReadOnlyHint'
import { PythonScriptForm, type FormData } from '@/components/pythonScripts/PythonScriptForm'
import {
  usePythonScripts,
  useCreatePythonScript,
  useUpdatePythonScript,
  useDeletePythonScript,
} from '@/hooks/usePythonScripts'
import { useIsAdmin } from '@/stores/authStore'
import type { PythonScript, PythonScriptCreateRequest, PythonScriptUpdateRequest, ValidationResult } from '@/types/pythonScript'

type FormDialog = { type: 'none' } | { type: 'create' } | { type: 'edit'; script: PythonScript }
type SaveAnyway = { editId?: string; data: FormData; message: string }

export function PythonScriptsPage() {
  const [formDialog, setFormDialog] = useState<FormDialog>({ type: 'none' })
  const [saveAnyway, setSaveAnyway] = useState<SaveAnyway | null>(null)
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)
  const isAdmin = useIsAdmin()

  const { data: scripts = [], isLoading, error } = usePythonScripts()
  const createMutation = useCreatePythonScript()
  const updateMutation = useUpdatePythonScript()
  const deleteMutation = useDeletePythonScript()

  const closeForm = () => setFormDialog({ type: 'none' })

  const handleCreate = (data: FormData) => {
    createMutation.mutate(data as PythonScriptCreateRequest, {
      onSuccess: closeForm,
    })
  }

  const handleUpdate = (id: string) => (data: FormData) => {
    updateMutation.mutate({ id, data: data as PythonScriptUpdateRequest }, {
      onSuccess: closeForm,
    })
  }

  const handleValidationFailure = (editId?: string) => (result: ValidationResult, data: FormData) => {
    const msg = result.line !== undefined
      ? `Line ${result.line}, col ${result.col ?? 0}: ${result.error}`
      : (result.error ?? 'Syntax error')
    setSaveAnyway({ editId, data, message: msg })
  }

  const handleSaveAnyway = () => {
    if (!saveAnyway) return
    if (saveAnyway.editId) {
      updateMutation.mutate(
        { id: saveAnyway.editId, data: saveAnyway.data as PythonScriptUpdateRequest },
        { onSuccess: () => { setSaveAnyway(null); closeForm() } }
      )
    } else {
      createMutation.mutate(saveAnyway.data as PythonScriptCreateRequest, {
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
              <CardDescription>Reusable Python scripts for script-mode agent definitions</CardDescription>
            </div>
            {isAdmin && (
              <Button onClick={() => setFormDialog({ type: 'create' })} disabled={isFormOpen}>
                <Plus className="h-4 w-4 mr-2" />
                New Python Script
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {isLoading && <p className="text-center py-8 text-muted-foreground">Loading…</p>}
            {error && <p className="text-center py-8 text-destructive">Error: {(error as Error).message}</p>}
            {!isLoading && !error && scripts.length === 0 && (
              <p className="text-center py-8 text-muted-foreground">No Python scripts yet.</p>
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
          <h3 className="text-lg font-semibold">
            {formDialog.type === 'create' ? 'New Python Script' : 'Edit Python Script'}
          </h3>
        </DialogHeader>
        <DialogBody>
          {formDialog.type === 'create' && (
            <PythonScriptForm
              isCreate
              onSubmit={handleCreate}
              onValidationFailure={handleValidationFailure(undefined)}
              onCancel={closeForm}
              isPending={createMutation.isPending}
            />
          )}
          {formDialog.type === 'edit' && (
            <PythonScriptForm
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
        title="Syntax errors detected"
        message={saveAnyway ? `${saveAnyway.message}\n\nSave anyway?` : ''}
        confirmLabel="Save anyway"
      />

      <ConfirmDialog
        open={!!deleteConfirmId}
        onClose={() => setDeleteConfirmId(null)}
        onConfirm={() => deleteConfirmId && handleDelete(deleteConfirmId)}
        title="Delete Python Script"
        message={`Delete script '${deleteTarget?.name ?? deleteConfirmId}'? Agent definitions referencing this script will need to be updated.`}
        confirmLabel="Delete"
        variant="destructive"
      />
    </div>
  )
}
