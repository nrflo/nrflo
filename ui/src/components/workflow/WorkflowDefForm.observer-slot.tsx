import { useMemo } from 'react'
import { Textarea } from '@/components/ui/Textarea'
import { Dropdown } from '@/components/ui/Dropdown'
import { useCLIModels } from '@/hooks/useCLIModels'

export interface ObserverState {
  context: string
  provider: string
  model: string
}

interface ObserverSectionProps {
  state: ObserverState
  onChange: (patch: Partial<ObserverState>) => void
}

export function ObserverSection({ state, onChange }: ObserverSectionProps) {
  const { data: models = [] } = useCLIModels()

  const providerOptions = useMemo(() => {
    const types = Array.from(new Set(models.filter((m) => m.enabled).map((m) => m.cli_type)))
    return [
      { value: '', label: 'Inherit project default' },
      ...types.map((t) => ({ value: t, label: t.charAt(0).toUpperCase() + t.slice(1) })),
    ]
  }, [models])

  const modelOptions = useMemo(() => {
    const filtered = models.filter((m) => m.enabled && (!state.provider || m.cli_type === state.provider))
    return [
      { value: '', label: 'Inherit project default' },
      ...filtered.map((m) => ({ value: m.id, label: m.display_name })),
    ]
  }, [models, state.provider])

  return (
    <div className="border-t border-border pt-3 space-y-3">
      <div className="text-xs font-medium text-muted-foreground">Observer overrides</div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Observer context</label>
        <Textarea
          value={state.context}
          onChange={(e) => onChange({ context: e.target.value })}
          rows={2}
          placeholder="Optional observer context for this workflow (overrides project setting)"
        />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">Provider</label>
          <Dropdown
            value={state.provider}
            onChange={(v) => onChange({ provider: v, model: '' })}
            options={providerOptions}
          />
        </div>
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">Model</label>
          <Dropdown value={state.model} onChange={(v) => onChange({ model: v })} options={modelOptions} />
        </div>
      </div>
    </div>
  )
}
