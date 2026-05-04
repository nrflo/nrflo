import { useState } from 'react'
import { Plus, Pencil, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { Toggle } from '@/components/ui/Toggle'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { FindingRow, isInternalKey } from './FindingsPanel'
import {
  useProjectFindings,
  useUpsertProjectFinding,
  useDeleteProjectFinding,
} from '@/hooks/useTickets'

interface DialogState {
  open: boolean
  key: string
  value: string
  isEdit: boolean
}

const CLOSED_DIALOG: DialogState = { open: false, key: '', value: '', isEdit: false }

export function ProjectFindingsTab({ projectId }: { projectId: string }) {
  const { data: findings } = useProjectFindings(projectId)
  const upsertMutation = useUpsertProjectFinding()
  const deleteMutation = useDeleteProjectFinding()

  const [showInternal, setShowInternal] = useState(false)
  const [dialog, setDialog] = useState<DialogState>(CLOSED_DIALOG)
  const [deleteKey, setDeleteKey] = useState<string | null>(null)

  const entries = findings ? Object.entries(findings) : []
  const visibleEntries = showInternal
    ? entries
    : entries.filter(([key]) => !isInternalKey(key))

  const openAdd = () => setDialog({ open: true, key: '', value: '', isEdit: false })

  const openEdit = (key: string, value: unknown) => {
    const strValue = typeof value === 'string' ? value : JSON.stringify(value, null, 2)
    setDialog({ open: true, key, value: strValue, isEdit: true })
  }

  const closeDialog = () => setDialog(CLOSED_DIALOG)

  const handleSubmit = () => {
    if (!dialog.key.trim()) return
    upsertMutation.mutate(
      { projectId, key: dialog.key.trim(), value: dialog.value },
      {
        onSuccess: closeDialog,
        onError: (err: Error) => toast.error(`Failed to save finding: ${err.message}`),
      }
    )
  }

  const handleDelete = () => {
    if (!deleteKey) return
    deleteMutation.mutate(
      { projectId, key: deleteKey },
      {
        onSuccess: () => setDeleteKey(null),
        onError: (err: Error) => toast.error(`Failed to delete finding: ${err.message}`),
      }
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Button size="sm" onClick={openAdd}>
          <Plus className="h-4 w-4 mr-1" />
          Add Finding
        </Button>
        <Toggle
          checked={showInternal}
          onChange={setShowInternal}
          label="Show internal keys"
        />
      </div>

      {visibleEntries.length === 0 ? (
        <p className="text-sm text-muted-foreground py-4">No project findings yet.</p>
      ) : (
        <div className="border border-border rounded">
          {visibleEntries.map(([key, value]) => (
            <div key={key} className="flex items-center gap-1 border-b border-border/50 last:border-b-0 pr-1">
              <div className="flex-1 min-w-0">
                <FindingRow findingKey={key} value={value} />
              </div>
              <button
                onClick={() => openEdit(key, value)}
                className="p-1.5 hover:bg-muted/50 rounded text-muted-foreground hover:text-foreground transition-colors shrink-0"
                title="Edit"
              >
                <Pencil className="h-3.5 w-3.5" />
              </button>
              <button
                onClick={() => setDeleteKey(key)}
                className="p-1.5 hover:bg-muted/50 rounded text-muted-foreground hover:text-destructive transition-colors shrink-0"
                title="Delete"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}

      <Dialog open={dialog.open} onClose={closeDialog} className="max-w-lg">
        <DialogHeader onClose={closeDialog}>
          {dialog.isEdit ? 'Edit Finding' : 'Add Finding'}
        </DialogHeader>
        <DialogBody className="space-y-4">
          <div>
            <label className="block text-sm font-medium mb-1.5">Key</label>
            <Input
              value={dialog.key}
              onChange={(e) => {
                if (!dialog.isEdit) setDialog((d) => ({ ...d, key: e.target.value }))
              }}
              disabled={dialog.isEdit}
              placeholder="finding-key"
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1.5">Value</label>
            <Textarea
              value={dialog.value}
              onChange={(e) => setDialog((d) => ({ ...d, value: e.target.value }))}
              placeholder="Value..."
              rows={6}
            />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={closeDialog} disabled={upsertMutation.isPending}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!dialog.key.trim() || upsertMutation.isPending}
          >
            {dialog.isEdit ? 'Save' : 'Add'}
          </Button>
        </DialogFooter>
      </Dialog>

      <ConfirmDialog
        open={!!deleteKey}
        onClose={() => setDeleteKey(null)}
        onConfirm={handleDelete}
        title="Delete Finding"
        message={`Are you sure you want to delete the finding "${deleteKey}"?`}
        confirmLabel="Delete"
        variant="destructive"
      />
    </div>
  )
}
