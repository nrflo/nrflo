import { useState } from 'react'
import { Pencil, Trash2, Plus, X, Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/Table'
import { useProjectEnvVars, usePutProjectEnvVar, useDeleteProjectEnvVar } from '@/hooks/useProjectEnvVars'

export function ProjectEnvVarsEditor({ projectId }: { projectId: string }) {
  const { data: vars, isLoading } = useProjectEnvVars(projectId)
  const putMutation = usePutProjectEnvVar()
  const deleteMutation = useDeleteProjectEnvVar()

  const [editingName, setEditingName] = useState<string | null>(null)
  const [editingValue, setEditingValue] = useState('')
  const [editError, setEditError] = useState<string | null>(null)

  const [deletingName, setDeletingName] = useState<string | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const [addName, setAddName] = useState('')
  const [addValue, setAddValue] = useState('')
  const [addError, setAddError] = useState<string | null>(null)

  function startEdit(name: string, value: string) {
    setEditingName(name)
    setEditingValue(value)
    setEditError(null)
  }

  function cancelEdit() {
    setEditingName(null)
    setEditingValue('')
    setEditError(null)
  }

  function saveEdit(name: string) {
    putMutation.mutate(
      { projectId, name, value: editingValue },
      {
        onSuccess: () => { setEditingName(null); setEditingValue('') },
        onError: (err) => setEditError((err as Error).message),
      }
    )
  }

  function confirmDelete() {
    if (!deletingName) return
    deleteMutation.mutate(
      { projectId, name: deletingName },
      {
        onSuccess: () => setDeletingName(null),
        onError: (err) => { setDeleteError((err as Error).message); setDeletingName(null) },
      }
    )
  }

  function saveAdd() {
    if (!addName.trim()) return
    setAddError(null)
    putMutation.mutate(
      { projectId, name: addName.trim(), value: addValue },
      {
        onSuccess: () => { setAddName(''); setAddValue('') },
        onError: (err) => setAddError((err as Error).message),
      }
    )
  }

  return (
    <div className="border-t border-border pt-3 space-y-3">
      <div className="text-sm font-medium text-muted-foreground">Environment Variables</div>
      {isLoading ? (
        <Spinner size="sm" />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Value</TableHead>
              <TableHead className="w-20" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {(vars ?? []).map((v) => (
              <TableRow key={v.name}>
                <TableCell className="font-mono text-xs">{v.name}</TableCell>
                <TableCell>
                  {editingName === v.name ? (
                    <>
                      <Input
                        value={editingValue}
                        onChange={(e) => setEditingValue(e.target.value)}
                        className="h-7 text-xs"
                        autoFocus
                      />
                      {editError && <p className="text-xs text-destructive mt-1">{editError}</p>}
                    </>
                  ) : (
                    <span className="font-mono text-xs">{v.value}</span>
                  )}
                </TableCell>
                <TableCell>
                  <div className="flex gap-1 justify-end">
                    {editingName === v.name ? (
                      <>
                        <Button size="sm" variant="ghost" onClick={() => saveEdit(v.name)} disabled={putMutation.isPending}>
                          <Check className="h-3.5 w-3.5" />
                        </Button>
                        <Button size="sm" variant="ghost" onClick={cancelEdit}>
                          <X className="h-3.5 w-3.5" />
                        </Button>
                      </>
                    ) : (
                      <>
                        <Button size="sm" variant="ghost" onClick={() => startEdit(v.name, v.value)}>
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => { setDeletingName(v.name); setDeleteError(null) }}>
                          <Trash2 className="h-3.5 w-3.5 text-destructive" />
                        </Button>
                      </>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            ))}
            <TableRow>
              <TableCell>
                <Input
                  value={addName}
                  onChange={(e) => setAddName(e.target.value)}
                  placeholder="VAR_NAME"
                  className="h-7 text-xs"
                />
              </TableCell>
              <TableCell>
                <Input
                  value={addValue}
                  onChange={(e) => setAddValue(e.target.value)}
                  placeholder="value"
                  className="h-7 text-xs"
                />
                {addError && <p className="text-xs text-destructive mt-1">{addError}</p>}
              </TableCell>
              <TableCell>
                <div className="flex justify-end">
                  <Button size="sm" variant="ghost" onClick={saveAdd} disabled={!addName.trim() || putMutation.isPending}>
                    <Plus className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      )}
      {deleteError && <p className="text-xs text-destructive">{deleteError}</p>}
      <ConfirmDialog
        open={deletingName !== null}
        onClose={() => setDeletingName(null)}
        onConfirm={confirmDelete}
        title="Delete variable"
        message={`Delete environment variable "${deletingName}"?`}
        confirmLabel="Delete"
        variant="destructive"
      />
    </div>
  )
}
