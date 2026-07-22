import { Badge } from '@/components/ui/Badge'
import { Dropdown, type DropdownOption } from '@/components/ui/Dropdown'
import { AgentDefEffortField } from '@/components/workflow/AgentDefEffortField'
import { useModelOptions, useModels } from '@/hooks/useModels'
import type { SetTierChainEntry } from '@/api/tierModels'
import type { ModelMode } from '@/api/models'

const EXECUTION_MODE_OPTIONS: DropdownOption[] = [
  { value: 'cli_interactive', label: 'CLI Interactive' },
  { value: 'api', label: 'API' },
]

const PROVIDER_LABELS: Record<string, string> = { anthropic: 'Anthropic', openai: 'OpenAI', openrouter: 'OpenRouter' }

// TierChainEntryForm edits one entry of a tier's fallback chain: execution
// mode, model (filtered by mode), reasoning effort, and a derived read-only
// provider badge sourced from the selected model's row.
export function TierChainEntryForm({
  entry,
  onChange,
}: {
  entry: SetTierChainEntry
  onChange: (entry: SetTierChainEntry) => void
}) {
  const modelMode: ModelMode = entry.execution_mode === 'api' ? 'api' : 'cli'
  const modelOptions = useModelOptions(modelMode)
  const { data: models = [] } = useModels()
  const selectedModel = models.find((m) => m.id === entry.model_id)

  return (
    <div className="flex flex-1 flex-wrap items-end gap-2">
      <div className="w-40">
        <label className="block text-xs font-medium text-muted-foreground mb-1">Mode</label>
        <Dropdown
          value={entry.execution_mode}
          onChange={(val) => onChange({ ...entry, execution_mode: val as SetTierChainEntry['execution_mode'], model_id: '' })}
          options={EXECUTION_MODE_OPTIONS}
        />
      </div>
      <div className="min-w-48 flex-1">
        <label className="block text-xs font-medium text-muted-foreground mb-1">Model</label>
        <Dropdown
          value={entry.model_id}
          onChange={(val) => onChange({ ...entry, model_id: val })}
          options={modelOptions}
          placeholder="Select a model"
        />
      </div>
      <div className="w-44">
        <AgentDefEffortField
          executionMode={entry.execution_mode}
          model={entry.model_id}
          value={entry.reasoning_effort}
          onChange={(val) => onChange({ ...entry, reasoning_effort: val })}
        />
      </div>
      <Badge variant="outline" className="mb-1.5 shrink-0">
        {selectedModel ? PROVIDER_LABELS[selectedModel.provider] ?? selectedModel.provider : '—'}
      </Badge>
    </div>
  )
}
