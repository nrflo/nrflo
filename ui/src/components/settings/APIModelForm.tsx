import { X, Check, Lock } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Dropdown } from '@/components/ui/Dropdown'
import type { APIProviderName } from '@/api/apiModels'

export interface APIModelFormData {
  id: string
  provider: APIProviderName
  display_name: string
  mapped_model: string
  reasoning_effort: string
  context_length: string
}

export const emptyAPIModelForm: APIModelFormData = {
  id: '',
  provider: 'anthropic',
  display_name: '',
  mapped_model: '',
  reasoning_effort: '',
  context_length: '200000',
}

const PROVIDER_OPTIONS = [
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
]

const REASONING_EFFORT_OPTIONS = [
  { value: '', label: 'Default' },
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
  { value: 'xhigh', label: 'Extra High (Opus 4.7/4.8 or Sonnet 5 only)' },
  { value: 'max', label: 'Max' },
]

function buildEffortOptions(provider: APIProviderName, mappedModel: string) {
  if (provider === 'anthropic') {
    const supportsXHigh =
      mappedModel.startsWith('claude-opus-4-7') ||
      mappedModel.startsWith('claude-opus-4-8') ||
      mappedModel.startsWith('claude-sonnet-5')
    return REASONING_EFFORT_OPTIONS.map((opt) =>
      opt.value === 'xhigh' && !supportsXHigh
        ? { ...opt, disabled: true, tooltip: "'xhigh' is only supported on Anthropic Opus 4.7/4.8 or Sonnet 5 models" }
        : opt
    )
  }
  return REASONING_EFFORT_OPTIONS.filter((opt) => opt.value !== 'xhigh')
}

export function APIModelForm({
  formData,
  setFormData,
  onCancel,
  onSave,
  mutation,
  isCreate,
  readOnly,
}: {
  formData: APIModelFormData
  setFormData: (data: APIModelFormData) => void
  onCancel: () => void
  onSave: () => void
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mutation: { isPending: boolean; isError: boolean; error: any }
  isCreate?: boolean
  readOnly?: boolean
}) {
  const lockBuiltIn = !!readOnly && !isCreate
  return (
    <div className={`space-y-3 ${isCreate ? 'border border-primary rounded-lg p-4 bg-muted/30' : ''}`}>
      {lockBuiltIn && (
        <div className="flex items-center gap-2 rounded-md border border-muted bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
          <Lock className="h-4 w-4 shrink-0" />
          Built-in model — only reasoning effort can be changed
        </div>
      )}
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">
            ID {isCreate && <span className="text-destructive">*</span>}
          </label>
          {isCreate ? (
            <Input
              value={formData.id}
              onChange={(e) => setFormData({ ...formData, id: e.target.value })}
              placeholder="my-custom-model"
            />
          ) : (
            <Input value={formData.id} disabled className="bg-muted" />
          )}
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">
            Provider {isCreate && <span className="text-destructive">*</span>}
          </label>
          {isCreate ? (
            <Dropdown
              value={formData.provider}
              onChange={(val) => setFormData({ ...formData, provider: val as APIProviderName })}
              options={PROVIDER_OPTIONS}
            />
          ) : (
            <Input value={formData.provider} disabled className="bg-muted" />
          )}
        </div>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">
            Display Name <span className="text-destructive">*</span>
          </label>
          <Input
            value={formData.display_name}
            onChange={(e) => setFormData({ ...formData, display_name: e.target.value })}
            placeholder="My Model"
            disabled={lockBuiltIn}
            className={lockBuiltIn ? 'bg-muted' : undefined}
          />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">
            Mapped Model <span className="text-destructive">*</span>
          </label>
          <Input
            value={formData.mapped_model}
            onChange={(e) => setFormData({ ...formData, mapped_model: e.target.value })}
            placeholder="claude-opus-4-7-20250514"
            disabled={lockBuiltIn}
            className={lockBuiltIn ? 'bg-muted' : undefined}
          />
        </div>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">Reasoning Effort</label>
          <Dropdown
            value={formData.reasoning_effort}
            onChange={(val) => setFormData({ ...formData, reasoning_effort: val })}
            options={buildEffortOptions(formData.provider, formData.mapped_model)}
          />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Context Length</label>
          <Input
            type="number"
            value={formData.context_length}
            onChange={(e) => setFormData({ ...formData, context_length: e.target.value })}
            placeholder="200000"
            disabled={lockBuiltIn}
            className={lockBuiltIn ? 'bg-muted' : undefined}
          />
        </div>
      </div>
      <div className="flex gap-2 justify-end">
        <Button variant="ghost" onClick={onCancel}>
          {isCreate ? 'Cancel' : <><X className="h-4 w-4 mr-1" />Cancel</>}
        </Button>
        <Button
          onClick={onSave}
          disabled={
            isCreate
              ? !formData.id.trim() || !formData.display_name.trim() || !formData.mapped_model.trim() || mutation.isPending
              : mutation.isPending
          }
        >
          {isCreate ? (
            mutation.isPending ? 'Creating...' : 'Create'
          ) : (
            <>{mutation.isPending ? 'Saving...' : <><Check className="h-4 w-4 mr-1" />Save</>}</>
          )}
        </Button>
      </div>
      {mutation.isError && (
        <p className="text-sm text-destructive">
          Error: {mutation.error.message}
        </p>
      )}
    </div>
  )
}
