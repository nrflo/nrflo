import { useState, useEffect } from 'react'
import { Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Dropdown } from '@/components/ui/Dropdown'
import { useArtifactStorage, useSetArtifactStorage } from '@/hooks/useProjectSettings'
import type { ArtifactStorageConfig } from '@/api/projectSettings'

const REDACTED = 'literal:***'

const MODE_OPTIONS = [
  { value: 'internal', label: 'Internal' },
  { value: 'cloudflare_r2', label: 'Cloudflare R2' },
  { value: 's3', label: 'S3', disabled: true, tooltip: 'Coming soon' },
]

interface FormState {
  mode: string
  account_id: string
  bucket: string
  prefix: string
  access_key_ref: string
  secret_key_ref: string
}

function toForm(cfg: ArtifactStorageConfig): FormState {
  return {
    mode: cfg.mode || 'internal',
    account_id: cfg.account_id || '',
    bucket: cfg.bucket || '',
    prefix: cfg.prefix || '',
    access_key_ref: cfg.access_key_ref || '',
    secret_key_ref: cfg.secret_key_ref || '',
  }
}

function buildPayload(form: FormState, original: ArtifactStorageConfig | undefined): ArtifactStorageConfig {
  const cfg: ArtifactStorageConfig = { mode: form.mode }
  if (form.mode === 'cloudflare_r2') {
    if (form.account_id) cfg.account_id = form.account_id
    if (form.bucket) cfg.bucket = form.bucket
    if (form.prefix) cfg.prefix = form.prefix
    if (form.access_key_ref && form.access_key_ref !== REDACTED) cfg.access_key_ref = form.access_key_ref
    else if (original?.access_key_ref && form.access_key_ref === REDACTED) cfg.access_key_ref = original.access_key_ref
    if (form.secret_key_ref && form.secret_key_ref !== REDACTED) cfg.secret_key_ref = form.secret_key_ref
    else if (original?.secret_key_ref && form.secret_key_ref === REDACTED) cfg.secret_key_ref = original.secret_key_ref
  }
  return cfg
}

export function ProjectArtifactStorageEditor({ projectId }: { projectId: string }) {
  const { data } = useArtifactStorage(projectId)
  const mutation = useSetArtifactStorage()
  const [form, setForm] = useState<FormState>({ mode: 'internal', account_id: '', bucket: '', prefix: '', access_key_ref: '', secret_key_ref: '' })
  const [serverError, setServerError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (data) setForm(toForm(data))
  }, [data])

  function handleSubmit() {
    setServerError(null)
    setSaved(false)
    mutation.mutate(
      { projectId, cfg: buildPayload(form, data) },
      {
        onSuccess: () => setSaved(true),
        onError: (err) => setServerError((err as Error).message),
      }
    )
  }

  const isS3 = form.mode === 's3'

  return (
    <div className="border-t border-border pt-3 space-y-3">
      <div className="text-sm font-medium text-muted-foreground">Artifact Storage</div>
      <div>
        <label className="text-sm font-medium text-muted-foreground">Mode</label>
        <Dropdown
          value={form.mode}
          onChange={(val) => setForm({ ...form, mode: val })}
          options={MODE_OPTIONS}
        />
        {isS3 && (
          <p className="text-xs text-muted-foreground mt-1">S3 support is coming soon.</p>
        )}
      </div>
      {form.mode === 'cloudflare_r2' && (
        <div className="space-y-3 pl-4 border-l-2 border-border">
          <div>
            <label className="text-sm font-medium text-muted-foreground">Account ID</label>
            <Input
              value={form.account_id}
              onChange={(e) => setForm({ ...form, account_id: e.target.value })}
              placeholder="your-account-id"
            />
          </div>
          <div>
            <label className="text-sm font-medium text-muted-foreground">Bucket</label>
            <Input
              value={form.bucket}
              onChange={(e) => setForm({ ...form, bucket: e.target.value })}
              placeholder="my-bucket"
            />
          </div>
          <div>
            <label className="text-sm font-medium text-muted-foreground">Prefix</label>
            <Input
              value={form.prefix}
              onChange={(e) => setForm({ ...form, prefix: e.target.value })}
              placeholder="optional/path/prefix/"
            />
          </div>
          <div>
            <label className="text-sm font-medium text-muted-foreground">Access Key Ref</label>
            <Input
              value={form.access_key_ref}
              onChange={(e) => setForm({ ...form, access_key_ref: e.target.value })}
              placeholder="env:R2_ACCESS_KEY"
            />
            <p className="text-xs text-muted-foreground mt-0.5">Prefix: <code>env:</code>, <code>file:</code>, or <code>literal:</code></p>
          </div>
          <div>
            <label className="text-sm font-medium text-muted-foreground">Secret Key Ref</label>
            <Input
              value={form.secret_key_ref}
              onChange={(e) => setForm({ ...form, secret_key_ref: e.target.value })}
              placeholder="env:R2_SECRET_KEY"
            />
            <p className="text-xs text-muted-foreground mt-0.5">Prefix: <code>env:</code>, <code>file:</code>, or <code>literal:</code></p>
          </div>
        </div>
      )}
      <div className="flex gap-2 justify-end">
        <Button onClick={handleSubmit} disabled={isS3 || mutation.isPending}>
          {mutation.isPending ? 'Saving...' : <><Check className="h-4 w-4 mr-1" />Save</>}
        </Button>
      </div>
      {saved && !mutation.isPending && (
        <p className="text-sm text-green-600 dark:text-green-400">Saved.</p>
      )}
      {serverError && (
        <p className="text-sm text-destructive">{serverError}</p>
      )}
    </div>
  )
}
