import { useEffect, useState } from 'react'
import { Copy, Check, AlertTriangle } from 'lucide-react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Dropdown } from '@/components/ui/Dropdown'
import { ApiError } from '@/api/client'
import { useProjectStore } from '@/stores/projectStore'
import { useCreateServiceToken } from '@/hooks/useServiceTokens'
import type { ServiceToken, CreateServiceTokenResponse } from '@/types/serviceToken'

interface CreateDialogProps {
  open: boolean
  onClose: () => void
  onCreated: (result: CreateServiceTokenResponse) => void
}

export function CreateServiceTokenDialog({ open, onClose, onCreated }: CreateDialogProps) {
  const projects = useProjectStore((s) => s.projects)
  const currentProject = useProjectStore((s) => s.currentProject)
  const [projectId, setProjectId] = useState<string>(currentProject || projects[0]?.id || '')
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const createMutation = useCreateServiceToken()

  useEffect(() => {
    if (open) {
      setProjectId(currentProject || projects[0]?.id || '')
      setName('')
      setError(null)
    }
  }, [open, currentProject, projects])

  const projectOptions = projects.map((p) => ({ value: p.id, label: p.name }))

  const handleSubmit = async () => {
    setError(null)
    try {
      const result = await createMutation.mutateAsync({ projectId, name: name.trim() })
      onCreated(result)
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Failed to create token')
    }
  }

  const canSubmit = !!projectId && name.trim().length > 0 && !createMutation.isPending

  return (
    <Dialog open={open} onClose={onClose} className="max-w-md">
      <DialogHeader onClose={onClose}>
        <h3 className="text-lg font-semibold">Create Service Token</h3>
      </DialogHeader>
      <DialogBody className="space-y-4">
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="space-y-1">
          <label className="text-sm font-medium">Project</label>
          <Dropdown value={projectId} onChange={setProjectId} options={projectOptions} />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Name</label>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. ci-deploy, dashboard-readonly"
            maxLength={64}
          />
          <p className="text-xs text-muted-foreground">
            A human-readable label so you can identify this token later. The token itself will be
            generated automatically.
          </p>
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" size="sm" onClick={onClose} disabled={createMutation.isPending}>
          Cancel
        </Button>
        <Button size="sm" onClick={handleSubmit} disabled={!canSubmit}>
          {createMutation.isPending ? 'Creating…' : 'Create Token'}
        </Button>
      </DialogFooter>
    </Dialog>
  )
}

interface RevealDialogProps {
  open: boolean
  token: string
  record: ServiceToken | null
  onClose: () => void
}

export function RevealServiceTokenDialog({ open, token, record, onClose }: RevealDialogProps) {
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!open) setCopied(false)
  }, [open])

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(token)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // clipboard blocked; user can still select the text
    }
  }

  return (
    <Dialog open={open} onClose={onClose} className="max-w-lg">
      <DialogHeader onClose={onClose}>
        <h3 className="text-lg font-semibold">Token created</h3>
      </DialogHeader>
      <DialogBody className="space-y-4">
        <div className="flex items-start gap-2 rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-300">
          <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" />
          <span>
            Copy this token now. For security reasons, it will not be shown again — only the
            short identifier <code className="font-mono">{record?.display_hint}</code>.
          </span>
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Token</label>
          <div className="flex gap-2">
            <Input value={token} readOnly className="font-mono text-xs" />
            <Button size="sm" variant="outline" onClick={handleCopy}>
              {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
            </Button>
          </div>
        </div>
        <div className="text-xs text-muted-foreground space-y-1">
          <p className="font-medium text-foreground">Usage</p>
          <p>Send the token in the Authorization header of every request:</p>
          <pre className="bg-muted px-2 py-1.5 rounded font-mono text-[11px] overflow-x-auto">
{`curl -H "Authorization: Bearer ${token}" \\
     -H "X-Project: ${record?.project_id ?? ''}" \\
     http://127.0.0.1:6587/api/v1/...`}
          </pre>
          <p>X-Project is optional — the token already carries its project scope.</p>
        </div>
      </DialogBody>
      <DialogFooter>
        <Button size="sm" onClick={onClose}>Done</Button>
      </DialogFooter>
    </Dialog>
  )
}
