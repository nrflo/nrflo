import { Dropdown, type DropdownOptionGroup } from '@/components/ui/Dropdown'
import { Toggle } from '@/components/ui/Toggle'
import { AgentDefEffortField } from '@/components/workflow/AgentDefEffortField'
import type { AgentFormData } from './AgentForm'

// AgentModelOverrideField is the "Override model" block of AgentForm: a
// toggle that, when off, leaves the agent on its tier's fallback chain
// (model=''); when on, exposes a mode-filtered model dropdown + reasoning
// effort field that win over the tier chain.
export function AgentModelOverrideField({
  formData,
  setFormData,
  modelOptions,
}: {
  formData: AgentFormData
  setFormData: (data: AgentFormData) => void
  modelOptions: DropdownOptionGroup[]
}) {
  return (
    <div className="space-y-2 rounded-lg border border-border p-3">
      <Toggle
        checked={formData.override}
        onChange={(checked) =>
          setFormData({
            ...formData,
            override: checked,
            model: checked ? formData.model || modelOptions[0]?.options[0]?.value || '' : '',
          })
        }
        label="Override model (skip tier fallback chain)"
      />
      {formData.override && (
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1">Model</label>
            <Dropdown
              value={formData.model}
              onChange={(val) => setFormData({ ...formData, model: val })}
              options={modelOptions}
            />
          </div>
          <AgentDefEffortField
            executionMode={formData.execution_mode as 'cli_interactive' | 'api'}
            model={formData.model}
            value={formData.reasoning_effort}
            onChange={(val) => setFormData({ ...formData, reasoning_effort: val })}
          />
        </div>
      )}
    </div>
  )
}
