import { Check, Lock, X } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { Input } from '@/components/ui/Input'
import type { ModelFormData } from './modelFormData'

const EFFORTS = ['low', 'medium', 'high', 'xhigh', 'max', 'ultra']

// Valid default-effort options: intersection of the enabled modes' effort lists.
function validEfforts(data: ModelFormData): string[] {
  return data.cli_model && data.api_model
    ? data.cli_efforts.filter((effort) => data.api_efforts.includes(effort))
    : data.cli_model ? data.cli_efforts : data.api_efforts
}

function EffortMultiSelect({
  label, values, onChange, disabled,
}: { label: string; values: string[]; onChange: (values: string[]) => void; disabled?: boolean }) {
  const toggle = (effort: string) => onChange(
    values.includes(effort) ? values.filter((item) => item !== effort) : [...values, effort],
  )
  return (
    <div>
      <label className="text-sm font-medium text-muted-foreground">{label}</label>
      <div className="mt-1 flex flex-wrap gap-1">
        {EFFORTS.map((effort) => (
          <Button
            key={effort}
            type="button"
            size="sm"
            variant={values.includes(effort) ? 'default' : 'outline'}
            onClick={() => toggle(effort)}
            disabled={disabled}
            aria-pressed={values.includes(effort)}
          >
            {effort}
          </Button>
        ))}
      </div>
    </div>
  )
}

interface Props {
  formData: ModelFormData
  setFormData: (data: ModelFormData) => void
  onCancel: () => void
  onSave: () => void
  mutation: { isPending: boolean; isError: boolean; error: Error | null }
  isCreate?: boolean
  readOnly?: boolean
}

export function ModelForm({ formData, setFormData, onCancel, onSave, mutation, isCreate, readOnly }: Props) {
  const locked = !!readOnly && !isCreate
  const defaultEfforts = validEfforts(formData)
  const valid = formData.id.trim() && formData.display_name.trim() &&
    (formData.cli_model.trim() || formData.api_model.trim())

  // Funnel all edits: if a change narrows the valid effort set past the current
  // default_effort, reset it to '' so a stale value is never submitted.
  const update = (patch: Partial<ModelFormData>) => {
    const next = { ...formData, ...patch }
    if (next.default_effort && !validEfforts(next).includes(next.default_effort)) {
      next.default_effort = ''
    }
    setFormData(next)
  }

  return (
    <div className={`space-y-4 ${isCreate ? 'rounded-lg border border-primary bg-muted/30 p-4' : ''}`}>
      {locked && (
        <div className="flex items-center gap-2 rounded-md border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
          <Lock className="h-4 w-4" /> Built-in model — only default effort and fallback models can be changed
        </div>
      )}
      <div className="grid grid-cols-3 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">ID</label>
          <Input value={formData.id} disabled={!isCreate} onChange={(e) => update({ id: e.target.value })} />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Provider</label>
          <Input value={formData.provider} disabled />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Display Name</label>
          <Input value={formData.display_name} disabled={locked} onChange={(e) => update({ display_name: e.target.value })} />
        </div>
      </div>
      <div className="grid grid-cols-2 gap-4">
        {formData.provider !== 'openrouter' && (
          <ModeFields mode="CLI" model={formData.cli_model} context={formData.cli_context} efforts={formData.cli_efforts} disabled={locked}
            onModel={(value) => update({ cli_model: value })}
            onContext={(value) => update({ cli_context: value })}
            onEfforts={(value) => update({ cli_efforts: value })} />
        )}
        <ModeFields mode="Direct API" model={formData.api_model} context={formData.api_context} efforts={formData.api_efforts} disabled={locked}
          onModel={(value) => update({ api_model: value })}
          onContext={(value) => update({ api_context: value })}
          onEfforts={(value) => update({ api_efforts: value })} />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">Default Effort</label>
          <Dropdown value={formData.default_effort} onChange={(value) => update({ default_effort: value })}
            options={[{ value: '', label: 'Provider default' }, ...defaultEfforts.map((value) => ({ value, label: value }))]} />
        </div>
        {formData.provider === 'anthropic' && (
          <div>
            <label className="text-sm font-medium text-muted-foreground">CLI Fallback Models</label>
            <Input value={formData.fallback_models} onChange={(e) => update({ fallback_models: e.target.value })} placeholder="model-a, model-b" />
          </div>
        )}
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel}>{isCreate ? 'Cancel' : <><X className="mr-1 h-4 w-4" />Cancel</>}</Button>
        <Button onClick={onSave} disabled={!valid || mutation.isPending}>
          {mutation.isPending ? 'Saving...' : isCreate ? 'Create' : <><Check className="mr-1 h-4 w-4" />Save</>}
        </Button>
      </div>
      {mutation.isError && <p className="text-sm text-destructive">Error: {mutation.error?.message}</p>}
    </div>
  )
}

function ModeFields({ mode, model, context, efforts, disabled, onModel, onContext, onEfforts }: {
  mode: string; model: string; context: string; efforts: string[]; disabled?: boolean
  onModel: (value: string) => void; onContext: (value: string) => void; onEfforts: (value: string[]) => void
}) {
  return (
    <fieldset className="space-y-3 rounded-md border p-3">
      <legend className="px-1 text-sm font-semibold">{mode}</legend>
      <div>
        <label className="text-sm font-medium text-muted-foreground">Model ID (empty disables mode)</label>
        <Input value={model} disabled={disabled} onChange={(e) => onModel(e.target.value)} />
      </div>
      <div>
        <label className="text-sm font-medium text-muted-foreground">Context Length</label>
        <Input type="number" value={context} disabled={disabled || !model} onChange={(e) => onContext(e.target.value)} />
      </div>
      <EffortMultiSelect label="Supported Efforts" values={efforts} onChange={onEfforts} disabled={disabled || !model} />
    </fieldset>
  )
}
