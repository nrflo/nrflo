import { useRef, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { checkImport, importWorkflows } from '@/api/workflows'
import type { WorkflowBundle, ImportConflicts } from '@/api/workflows'

type Step = 'select_file' | 'parsing_error' | 'checking' | 'conflicts' | 'importing' | 'error'

interface Props {
  open: boolean
  onClose: () => void
  onSuccess: () => void
}

export function WorkflowImportDialog({ open, onClose, onSuccess }: Props) {
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [step, setStep] = useState<Step>('select_file')
  const [parseError, setParseError] = useState<string | null>(null)
  const [bundle, setBundle] = useState<WorkflowBundle | null>(null)
  const [conflicts, setConflicts] = useState<ImportConflicts | null>(null)
  const [importError, setImportError] = useState<string | null>(null)

  function reset() {
    setStep('select_file')
    setParseError(null)
    setBundle(null)
    setConflicts(null)
    setImportError(null)
    if (fileInputRef.current) fileInputRef.current.value = ''
  }

  function handleClose() {
    reset()
    onClose()
  }

  const checkMutation = useMutation({
    mutationFn: (b: WorkflowBundle) => checkImport(b),
    onSuccess: (result, b) => {
      const hasConflicts = result.workflow_ids.length > 0 || result.python_script_ids.length > 0
      if (!hasConflicts) {
        setStep('importing')
        doImport(b, 'overwrite')
      } else {
        setConflicts(result)
        setStep('conflicts')
      }
    },
    onError: (err: Error) => {
      setImportError(err.message)
      setStep('error')
    },
  })

  const importMutation = useMutation({
    mutationFn: ({ b, action }: { b: WorkflowBundle; action: 'overwrite' | 'rename' | 'cancel' }) =>
      importWorkflows(b, action),
    onSuccess: () => {
      reset()
      onSuccess()
    },
    onError: (err: Error) => {
      setImportError(err.message)
      setStep('error')
    },
  })

  function doImport(b: WorkflowBundle, action: 'overwrite' | 'rename' | 'cancel') {
    importMutation.mutate({ b, action })
  }

  function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return

    setParseError(null)
    const reader = new FileReader()
    reader.onload = (ev) => {
      try {
        const parsed = JSON.parse(ev.target?.result as string) as WorkflowBundle
        if (!parsed.version || !Array.isArray(parsed.workflows)) {
          throw new Error('Invalid bundle: missing version or workflows array')
        }
        setBundle(parsed)
        setStep('checking')
        checkMutation.mutate(parsed)
      } catch (err) {
        setParseError(err instanceof Error ? err.message : 'Failed to parse JSON')
        setStep('parsing_error')
      }
    }
    reader.readAsText(file)
  }

  const isPending = step === 'checking' || step === 'importing'

  return (
    <Dialog open={open} onClose={handleClose}>
      <DialogHeader onClose={handleClose}>
        <h2 className="text-lg font-semibold">Import Workflows</h2>
      </DialogHeader>

      <DialogBody>
        {(step === 'select_file' || step === 'parsing_error') && (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              Select a workflow bundle JSON file exported from nrflo.
            </p>
            <input
              ref={fileInputRef}
              type="file"
              accept=".json,application/json"
              onChange={handleFileChange}
              className="block w-full text-sm text-foreground file:mr-4 file:py-1.5 file:px-3 file:rounded-md file:border-0 file:text-sm file:bg-muted file:text-foreground hover:file:bg-muted/80 cursor-pointer"
            />
            {step === 'parsing_error' && parseError && (
              <p className="text-sm text-destructive">{parseError}</p>
            )}
          </div>
        )}

        {step === 'checking' && (
          <p className="text-sm text-muted-foreground">Checking for conflicts...</p>
        )}

        {step === 'importing' && (
          <p className="text-sm text-muted-foreground">Importing workflows...</p>
        )}

        {step === 'conflicts' && conflicts && (
          <div className="space-y-4">
            <p className="text-sm">The following already exist in this project:</p>
            {conflicts.workflow_ids.length > 0 && (
              <div>
                <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-1">
                  Workflow IDs
                </p>
                <ul className="text-sm font-mono space-y-0.5">
                  {conflicts.workflow_ids.map((id) => (
                    <li key={id} className="text-destructive">{id}</li>
                  ))}
                </ul>
              </div>
            )}
            {conflicts.python_script_ids.length > 0 && (
              <div>
                <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-1">
                  Python Script IDs
                </p>
                <ul className="text-sm font-mono space-y-0.5">
                  {conflicts.python_script_ids.map((id) => (
                    <li key={id} className="text-destructive">{id}</li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        )}

        {step === 'error' && importError && (
          <p className="text-sm text-destructive">{importError}</p>
        )}
      </DialogBody>

      <DialogFooter>
        {step === 'conflicts' && bundle ? (
          <>
            <Button variant="ghost" size="sm" onClick={handleClose}>
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={() => {
                setStep('importing')
                doImport(bundle, 'rename')
              }}
              disabled={isPending}
            >
              Rename
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => {
                setStep('importing')
                doImport(bundle, 'overwrite')
              }}
              disabled={isPending}
            >
              Overwrite
            </Button>
          </>
        ) : (
          <Button variant="ghost" size="sm" onClick={handleClose} disabled={isPending}>
            {step === 'error' ? 'Close' : 'Cancel'}
          </Button>
        )}
      </DialogFooter>
    </Dialog>
  )
}
