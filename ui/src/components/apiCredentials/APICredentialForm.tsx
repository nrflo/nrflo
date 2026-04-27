import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import type { APICredential, APICredentialCreateRequest, APICredentialUpdateRequest } from '@/types/apiCredential'
import type { Project } from '@/api/projects'

const PROVIDER_OPTIONS = [
  { value: 'anthropic', label: 'Anthropic' },
]

const SECRET_REF_PLACEHOLDER = 'env:ANTHROPIC_API_KEY or file:/path or literal:sk-...'

interface APICredentialFormProps {
  initial?: Partial<APICredential>
  isCreate: boolean
  projects: Project[]
  onSubmit: (data: APICredentialCreateRequest | APICredentialUpdateRequest) => void
  onCancel: () => void
  isPending?: boolean
}

export function APICredentialForm({ initial, isCreate, projects, onSubmit, onCancel, isPending }: APICredentialFormProps) {
  const [provider, setProvider] = useState(initial?.provider ?? 'anthropic')
  const [projectId, setProjectId] = useState(initial?.project_id ?? '')
  const [secretRef, setSecretRef] = useState(
    initial?.secret_ref?.startsWith('literal:') ? '' : (initial?.secret_ref ?? '')
  )

  const projectOptions = [
    { value: '', label: '(Global)' },
    ...projects.map((p) => ({ value: p.id, label: p.name })),
  ]

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const data: APICredentialCreateRequest = {
      provider,
      project_id: projectId || undefined,
      secret_ref: secretRef.trim(),
    }
    onSubmit(isCreate ? data : (data as APICredentialUpdateRequest))
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3 p-4 border border-border rounded-lg bg-muted/30">
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Provider</label>
        <Dropdown value={provider} onChange={setProvider} options={PROVIDER_OPTIONS} />
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Project</label>
        <Dropdown value={projectId} onChange={setProjectId} options={projectOptions} />
        <p className="text-xs text-muted-foreground mt-1">Leave as Global to apply to all projects</p>
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Secret Ref</label>
        <input
          type="text"
          value={secretRef}
          onChange={(e) => setSecretRef(e.target.value)}
          placeholder={SECRET_REF_PLACEHOLDER}
          required={isCreate}
          className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm font-mono"
        />
        <p className="text-xs text-muted-foreground mt-1">
          Format: <code>env:VAR_NAME</code>, <code>file:/path/to/key</code>, or <code>literal:sk-...</code> (stored encrypted).
          Leave blank to keep existing value.
        </p>
      </div>
      <div className="flex gap-2 justify-end">
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>Cancel</Button>
        <Button type="submit" size="sm" disabled={isPending}>
          {isPending ? 'Saving…' : isCreate ? 'Create' : 'Save'}
        </Button>
      </div>
    </form>
  )
}
