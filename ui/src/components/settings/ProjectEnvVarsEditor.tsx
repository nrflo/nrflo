import { useState } from 'react'
import { Pencil, Trash2, Plus, X, Check, ChevronRight, ChevronDown, Copy } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { Badge } from '@/components/ui/Badge'
import { Tooltip } from '@/components/ui/Tooltip'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/Table'
import { useProjectEnvVars, usePutProjectEnvVar, useDeleteProjectEnvVar } from '@/hooks/useProjectEnvVars'
import { useEnvVarCatalog } from '@/hooks/useEnvVarCatalog'

export function ProjectEnvVarsEditor({ projectId }: { projectId: string }) {
  const { data: vars, isLoading } = useProjectEnvVars(projectId)
  const putMutation = usePutProjectEnvVar()
  const deleteMutation = useDeleteProjectEnvVar()
  const { data: catalog } = useEnvVarCatalog()

  const [catalogOpen, setCatalogOpen] = useState(false)
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
        <>
        {catalog && catalog.length > 0 && (
          <div className="border border-border rounded-md mb-2">
            <button
              type="button"
              className="flex items-center gap-1 w-full px-3 py-2 text-xs font-medium text-muted-foreground hover:text-foreground"
              onClick={() => setCatalogOpen((o) => !o)}
            >
              {catalogOpen ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
              Suggested variables
            </button>
            {catalogOpen && (
              <ul className="divide-y divide-border border-t border-border">
                {catalog.map((entry) => {
                  const isSet = (vars ?? []).some((v) => v.name === entry.name)
                  return (
                    <li
                      key={entry.name}
                      className={`flex items-center gap-2 px-3 py-1.5 cursor-pointer hover:bg-muted/50 ${isSet ? 'opacity-50' : ''}`}
                      onClick={() => setAddName(entry.name)}
                    >
                      <span className="font-mono text-xs flex-1 min-w-0 truncate">{entry.name}</span>
                      <Badge variant="secondary" className="shrink-0 text-[10px]">{entry.feature}</Badge>
                      {entry.required && (
                        <Badge variant="destructive" className="shrink-0 text-[10px]">required</Badge>
                      )}
                      {entry.description && (
                        <Tooltip text={entry.description}>
                          <span className="text-muted-foreground text-[10px] max-w-[120px] truncate hidden sm:inline">{entry.description}</span>
                        </Tooltip>
                      )}
                      {isSet && (
                        <Badge variant="success" className="shrink-0 text-[10px] gap-0.5">
                          <Check className="h-2.5 w-2.5" />Set
                        </Badge>
                      )}
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-6 w-6 p-0 shrink-0"
                        onClick={(e) => {
                          e.stopPropagation()
                          navigator.clipboard.writeText(entry.name).then(() => {
                            toast.success(`Copied ${entry.name}`)
                          })
                        }}
                      >
                        <Copy className="h-3 w-3" />
                      </Button>
                    </li>
                  )
                })}
              </ul>
            )}
          </div>
        )}
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
        </>
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
