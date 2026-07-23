import { Dropdown, type DropdownOption, type DropdownOptionGroup } from '@/components/ui/Dropdown'
import { Toggle } from '@/components/ui/Toggle'
import { AgentDefEffortField } from './AgentDefEffortField'
import { resolveTierChain, useTierModels } from '@/hooks/useTierModels'

const TIER_OPTIONS: DropdownOption[] = [1, 2, 3, 4, 5].map((t) => ({ value: String(t), label: `Tier ${t}` }))

// AgentDefModelTierFields is the workflow AgentDefForm analogue of
// settings/AgentModelOverrideField.tsx: a Tier selector plus an "Override
// model (skip tier fallback chain)" toggle. When the toggle is off the def
// runs on its tier's fallback chain (model=''), and this renders the
// resolved chain-primary model read-only so the operator can see what will
// actually run. When on, a mode-filtered model dropdown + effort field win
// over the tier chain.
export function AgentDefModelTierFields({
  tier,
  onTierChange,
  override,
  onOverrideChange,
  model,
  onModelChange,
  executionMode,
  reasoningEffort,
  onReasoningEffortChange,
  modelOptions,
}: {
  tier: number
  onTierChange: (tier: number) => void
  override: boolean
  onOverrideChange: (override: boolean) => void
  model: string
  onModelChange: (model: string) => void
  executionMode: 'cli_interactive' | 'api' | 'script'
  reasoningEffort: string
  onReasoningEffortChange: (v: string) => void
  modelOptions: DropdownOptionGroup[]
}) {
  const { data: tierModels = [] } = useTierModels()
  const resolvedChain = resolveTierChain(tierModels, tier)
  const resolvedModel = resolvedChain[0]?.model_id

  return (
    <div className="flex-1 space-y-2 rounded-lg border border-border p-3">
      <div className="flex items-end gap-3">
        <div className="w-32">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Tier</label>
          <Dropdown value={String(tier)} onChange={(val) => onTierChange(Number(val))} options={TIER_OPTIONS} />
        </div>
        <Toggle
          checked={override}
          onChange={(checked) => {
            onOverrideChange(checked)
            onModelChange(checked ? model || modelOptions[0]?.options[0]?.value || '' : '')
          }}
          label="Override model (skip tier fallback chain)"
        />
      </div>
      {override ? (
        <div className="flex gap-3">
          <div className="flex-1">
            <label className="block text-xs font-medium text-muted-foreground mb-1">Model</label>
            <Dropdown value={model} onChange={onModelChange} options={modelOptions} />
          </div>
          <AgentDefEffortField
            executionMode={executionMode}
            model={model}
            value={reasoningEffort}
            onChange={onReasoningEffortChange}
          />
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">
          Resolved model: <span className="font-medium">{resolvedModel || 'no chain configured for this tier'}</span>
        </p>
      )}
    </div>
  )
}
