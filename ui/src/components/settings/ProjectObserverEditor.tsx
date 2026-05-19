import { Textarea } from '@/components/ui/Textarea'
import { Dropdown } from '@/components/ui/Dropdown'
import { useCLIModels } from '@/hooks/useCLIModels'

export interface ObserverFormState {
  systemContext: string
  provider: string
  model: string
}

export function ProjectObserverEditor({
  value,
  onChange,
  serverError,
}: {
  projectId: string
  value: ObserverFormState
  onChange: (next: ObserverFormState) => void
  serverError?: string | null
}) {
  const { data: models = [] } = useCLIModels()

  const providerOptions = Array.from(new Set(models.filter(m => m.enabled).map(m => m.cli_type))).map(t => ({
    value: t,
    label: t.charAt(0).toUpperCase() + t.slice(1),
  }))

  const modelOptions = models
    .filter(m => m.enabled && (!value.provider || m.cli_type === value.provider))
    .map(m => ({ value: m.id, label: m.display_name }))

  return (
    <div className="border-t border-border pt-3 space-y-3">
      <div className="text-sm font-medium text-muted-foreground">Observer Settings</div>
      <div>
        <label className="text-sm font-medium text-muted-foreground">System context override</label>
        <Textarea
          value={value.systemContext}
          onChange={(e) => onChange({ ...value, systemContext: e.target.value })}
          rows={3}
          placeholder="Optional system context for this project's observer (leave empty to use global setting)"
        />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">Provider</label>
          <Dropdown
            value={value.provider}
            onChange={(v) => onChange({ ...value, provider: v, model: '' })}
            options={[{ value: '', label: 'Inherit global' }, ...providerOptions]}
          />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Model</label>
          <Dropdown
            value={value.model}
            onChange={(v) => onChange({ ...value, model: v })}
            options={[{ value: '', label: 'Inherit global' }, ...modelOptions]}
          />
        </div>
      </div>
      {serverError && (
        <p className="text-sm text-destructive">{serverError}</p>
      )}
    </div>
  )
}
