import { Dropdown } from '@/components/ui/Dropdown'
import { Input } from '@/components/ui/Input'
import type { ArtifactStorageConfig } from '@/api/projectSettings'

const REDACTED = 'literal:***'

const MODE_OPTIONS = [
  { value: 'internal', label: 'Internal' },
  { value: 'cloudflare_r2', label: 'Cloudflare R2' },
  { value: 's3', label: 'S3', disabled: true, tooltip: 'Coming soon' },
]

export interface FormState {
  mode: string
  account_id: string
  bucket: string
  prefix: string
  access_key_ref: string
  secret_key_ref: string
}

export function toForm(cfg: ArtifactStorageConfig): FormState {
  return {
    mode: cfg.mode || 'internal',
    account_id: cfg.account_id || '',
    bucket: cfg.bucket || '',
    prefix: cfg.prefix || '',
    access_key_ref: cfg.access_key_ref || '',
    secret_key_ref: cfg.secret_key_ref || '',
  }
}

export function buildPayload(form: FormState, original: ArtifactStorageConfig | undefined): ArtifactStorageConfig {
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

export function ProjectArtifactStorageEditor({
  value,
  onChange,
  serverError,
}: {
  projectId: string
  value: FormState
  onChange: (next: FormState) => void
  serverError?: string | null
}) {
  return (
    <div className="border-t border-border pt-3 space-y-3">
      <div className="text-sm font-medium text-muted-foreground">Artifact Storage</div>
      <div>
        <label className="text-sm font-medium text-muted-foreground">Mode</label>
        <Dropdown
          value={value.mode}
          onChange={(val) => onChange({ ...value, mode: val })}
          options={MODE_OPTIONS}
        />
      </div>
      {value.mode === 'cloudflare_r2' && (
        <div className="space-y-3 pl-4 border-l-2 border-border">
          <div>
            <label className="text-sm font-medium text-muted-foreground">Account ID</label>
            <Input
              value={value.account_id}
              onChange={(e) => onChange({ ...value, account_id: e.target.value })}
              placeholder="your-account-id"
            />
          </div>
          <div>
            <label className="text-sm font-medium text-muted-foreground">Bucket</label>
            <Input
              value={value.bucket}
              onChange={(e) => onChange({ ...value, bucket: e.target.value })}
              placeholder="my-bucket"
            />
          </div>
          <div>
            <label className="text-sm font-medium text-muted-foreground">Prefix</label>
            <Input
              value={value.prefix}
              onChange={(e) => onChange({ ...value, prefix: e.target.value })}
              placeholder="optional/path/prefix/"
            />
          </div>
          <div>
            <label className="text-sm font-medium text-muted-foreground">Access Key Ref</label>
            <Input
              value={value.access_key_ref}
              onChange={(e) => onChange({ ...value, access_key_ref: e.target.value })}
              placeholder="env:R2_ACCESS_KEY"
            />
            <p className="text-xs text-muted-foreground mt-0.5">Prefix: <code>env:</code>, <code>file:</code>, or <code>literal:</code></p>
          </div>
          <div>
            <label className="text-sm font-medium text-muted-foreground">Secret Key Ref</label>
            <Input
              value={value.secret_key_ref}
              onChange={(e) => onChange({ ...value, secret_key_ref: e.target.value })}
              placeholder="env:R2_SECRET_KEY"
            />
            <p className="text-xs text-muted-foreground mt-0.5">Prefix: <code>env:</code>, <code>file:</code>, or <code>literal:</code></p>
          </div>
        </div>
      )}
      {serverError && (
        <p className="text-sm text-destructive">{serverError}</p>
      )}
    </div>
  )
}
