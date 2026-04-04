import { useState } from 'react'
import { Check, X, ShieldCheck } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { checkSafetyHook, type SafetyHookCheckResponse } from '@/api/projects'
import { buildSafetyHookJSON, type ProjectFormData } from './ProjectForm'

interface SafetyHookCheckDialogProps {
  open: boolean
  onClose: () => void
  formData: ProjectFormData
}

export function SafetyHookCheckDialog({ open, onClose, formData }: SafetyHookCheckDialogProps) {
  const [command, setCommand] = useState('')
  const [result, setResult] = useState<SafetyHookCheckResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const configJSON = buildSafetyHookJSON(formData)
  const config = configJSON ? JSON.parse(configJSON) : null

  const dangerousCount = config?.dangerous_patterns?.length ?? 0
  const rmPathsCount = config?.rm_rf_allowed_paths?.length ?? 0
  const gitAllowed = config?.allow_git ?? true

  const handleExecute = async () => {
    if (!command.trim() || !config) return
    setLoading(true)
    setResult(null)
    setError('')
    try {
      const res = await checkSafetyHook({ config, command: command.trim() })
      setResult(res)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const handleClose = () => {
    setCommand('')
    setResult(null)
    setError('')
    setLoading(false)
    onClose()
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && command.trim() && !loading) {
      handleExecute()
    }
  }

  return (
    <Dialog open={open} onClose={handleClose}>
      <DialogHeader onClose={handleClose}>
        <div className="flex items-center gap-2">
          <ShieldCheck className="h-5 w-5" />
          Safety Hook Check
        </div>
      </DialogHeader>
      <DialogBody>
        <div className="space-y-4">
          <div className="text-sm text-muted-foreground space-y-1">
            <div>{dangerousCount} dangerous pattern{dangerousCount !== 1 ? 's' : ''} configured</div>
            <div>{rmPathsCount} allowed rm path{rmPathsCount !== 1 ? 's' : ''}</div>
            <div>Git operations: {gitAllowed ? 'allowed' : 'blocked'}</div>
          </div>
          <div>
            <label className="text-sm font-medium text-muted-foreground">Enter command to check</label>
            <Input
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="ls -la"
              autoFocus
            />
          </div>
          <Button
            onClick={handleExecute}
            disabled={!command.trim() || loading}
            size="sm"
          >
            {loading ? <Spinner className="h-4 w-4 mr-1" /> : null}
            Execute
          </Button>
          {result && (
            <div className="flex items-center gap-2 text-sm">
              {result.allowed ? (
                <>
                  <Check className="h-4 w-4 text-green-500" />
                  <span className="text-green-600 dark:text-green-400">Allowed</span>
                </>
              ) : (
                <>
                  <X className="h-4 w-4 text-red-500" />
                  <span className="text-red-600 dark:text-red-400">Blocked: {result.reason}</span>
                </>
              )}
            </div>
          )}
          {error && (
            <div className="text-sm text-red-600 dark:text-red-400">{error}</div>
          )}
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="ghost" onClick={handleClose}>Close</Button>
      </DialogFooter>
    </Dialog>
  )
}
