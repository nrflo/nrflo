import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { Textarea } from '@/components/ui/Textarea'
import type { ToolDefinition, ToolDefinitionCreateRequest, ToolDefinitionUpdateRequest, ToolAuthMethod } from '@/types/toolDefinition'

const AUTH_METHOD_OPTIONS = [
  { value: 'none', label: 'None' },
  { value: 'bearer_env', label: 'Bearer — env var' },
  { value: 'bearer_secret_ref', label: 'Bearer — secret ref' },
]

const AUTH_REF_HINT: Record<ToolAuthMethod, string> = {
  none: '',
  bearer_env: 'Env var name, e.g. MY_API_KEY',
  bearer_secret_ref: 'Secret ref, e.g. env:MY_KEY or file:/path',
}

interface ToolDefinitionFormProps {
  initial?: Partial<ToolDefinition>
  isCreate: boolean
  onSubmit: (data: ToolDefinitionCreateRequest | ToolDefinitionUpdateRequest) => void
  onCancel: () => void
  isPending?: boolean
}

export function ToolDefinitionForm({ initial, isCreate, onSubmit, onCancel, isPending }: ToolDefinitionFormProps) {
  const [name, setName] = useState(initial?.name ?? '')
  const [description, setDescription] = useState(initial?.description ?? '')
  const [inputSchema, setInputSchema] = useState(
    initial?.input_schema ? (typeof initial.input_schema === 'string' ? initial.input_schema : JSON.stringify(initial.input_schema, null, 2)) : ''
  )
  const [schemaError, setSchemaError] = useState<string | null>(null)
  const [endpoint, setEndpoint] = useState(initial?.endpoint ?? '')
  const [authMethod, setAuthMethod] = useState<ToolAuthMethod>(initial?.auth_method ?? 'none')
  const [authRef, setAuthRef] = useState(initial?.auth_ref ?? '')
  const [timeoutSec, setTimeoutSec] = useState<number | ''>(initial?.timeout_sec ?? 30)

  const validateSchema = (value: string) => {
    if (!value.trim()) { setSchemaError(null); return }
    try { JSON.parse(value); setSchemaError(null) }
    catch { setSchemaError('Invalid JSON') }
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (schemaError) return
    const base = {
      name: name.trim(),
      description: description.trim() || undefined,
      input_schema: inputSchema.trim(),
      endpoint: endpoint.trim(),
      auth_method: authMethod,
      auth_ref: authRef.trim() || undefined,
      timeout_sec: timeoutSec !== '' ? Number(timeoutSec) : 30,
    }
    if (isCreate) {
      onSubmit(base as ToolDefinitionCreateRequest)
    } else {
      onSubmit(base as ToolDefinitionUpdateRequest)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3 p-4 border border-border rounded-lg bg-muted/30">
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Name</label>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="my-tool-name"
          required
          className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
        />
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Description</label>
        <Textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="What this tool does…"
          className="min-h-[60px] text-sm"
        />
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">
          Input Schema (JSON)
        </label>
        <Textarea
          value={inputSchema}
          onChange={(e) => { setInputSchema(e.target.value); validateSchema(e.target.value) }}
          placeholder='{"type":"object","properties":{}}'
          className="min-h-[80px] text-sm font-mono"
        />
        {schemaError && <p className="text-xs text-destructive mt-1">{schemaError}</p>}
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Endpoint URL</label>
        <input
          type="url"
          value={endpoint}
          onChange={(e) => setEndpoint(e.target.value)}
          placeholder="https://example.com/api/tool"
          required
          className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
        />
      </div>
      <div className="flex gap-3">
        <div className="flex-1">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Auth Method</label>
          <Dropdown value={authMethod} onChange={(v) => setAuthMethod(v as ToolAuthMethod)} options={AUTH_METHOD_OPTIONS} />
        </div>
        <div className="w-40">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Timeout (s)</label>
          <input
            type="number"
            value={timeoutSec}
            onChange={(e) => setTimeoutSec(e.target.value === '' ? '' : Number(e.target.value))}
            min={1}
            className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
          />
        </div>
      </div>
      {authMethod !== 'none' && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">Auth Ref</label>
          <input
            type="text"
            value={authRef}
            onChange={(e) => setAuthRef(e.target.value)}
            placeholder={AUTH_REF_HINT[authMethod]}
            className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
          />
          <p className="text-xs text-muted-foreground mt-1">{AUTH_REF_HINT[authMethod]}</p>
        </div>
      )}
      <div className="flex gap-2 justify-end">
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>Cancel</Button>
        <Button type="submit" size="sm" disabled={isPending || !!schemaError}>
          {isPending ? 'Saving…' : isCreate ? 'Create' : 'Save'}
        </Button>
      </div>
    </form>
  )
}
