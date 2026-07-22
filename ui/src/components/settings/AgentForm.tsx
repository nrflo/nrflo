import { X, Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Dropdown, type DropdownOption, type DropdownOptionGroup } from '@/components/ui/Dropdown'
import { Textarea } from '@/components/ui/Textarea'
import { useModelOptions } from '@/hooks/useModels'
import type { ModelMode } from '@/api/models'
import { AgentModelOverrideField } from './AgentModelOverrideField'

function flattenModelOptions(groups: DropdownOptionGroup[]): DropdownOption[] {
  return groups.flatMap((g) => g.options)
}

const EXECUTION_MODE_OPTIONS: DropdownOption[] = [
  { value: 'cli_interactive', label: 'CLI Interactive' },
  { value: 'api', label: 'API' },
]

const TIER_OPTIONS: DropdownOption[] = [1, 2, 3, 4, 5].map((t) => ({ value: String(t), label: `Tier ${t}` }))

export interface AgentFormData {
  id: string
  model: string
  execution_mode: string
  timeout: string
  prompt: string
  restart_threshold: string
  max_fail_restarts: string
  stall_start_timeout_sec: string
  stall_running_timeout_sec: string
  tier: string
  override: boolean
  reasoning_effort: string
}

export const emptyAgentForm: AgentFormData = {
  id: '',
  model: '',
  execution_mode: 'cli_interactive',
  timeout: '30',
  prompt: '',
  restart_threshold: '',
  max_fail_restarts: '',
  stall_start_timeout_sec: '',
  stall_running_timeout_sec: '',
  tier: '1',
  override: false,
  reasoning_effort: '',
}

export function parseOptionalInt(val: string): number | null {
  const trimmed = val.trim()
  if (trimmed === '') return null
  const n = parseInt(trimmed, 10)
  return isNaN(n) ? null : n
}

export function AgentForm({
  formData,
  setFormData,
  onCancel,
  onSave,
  mutation,
  isCreate,
}: {
  formData: AgentFormData
  setFormData: (data: AgentFormData) => void
  onCancel: () => void
  onSave: () => void
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mutation: { isPending: boolean; isError: boolean; error: any }
  isCreate?: boolean
}) {
  const modelMode: ModelMode = formData.execution_mode === 'api' ? 'api' : 'cli'
  const modelOptions = useModelOptions(modelMode)
  const cliModelOptions = useModelOptions('cli')
  const apiModelOptions = useModelOptions('api')

  return (
    <div className={`space-y-3 ${isCreate ? 'border border-primary rounded-lg p-4 bg-muted/30' : ''}`}>
      <div className="grid grid-cols-4 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">
            ID {isCreate && <span className="text-destructive">*</span>}
          </label>
          {isCreate ? (
            <Input
              value={formData.id}
              onChange={(e) => setFormData({ ...formData, id: e.target.value })}
              placeholder="conflict-resolver"
            />
          ) : (
            <Input value={formData.id} disabled className="bg-muted" />
          )}
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Mode</label>
          <Dropdown
            value={formData.execution_mode}
            onChange={(val) => {
              if (!formData.override) {
                setFormData({ ...formData, execution_mode: val })
                return
              }
              const flatOptions = flattenModelOptions(val === 'api' ? apiModelOptions : cliModelOptions)
              const stillValid = flatOptions.some((opt) => opt.value === formData.model)
              setFormData({
                ...formData,
                execution_mode: val,
                model: stillValid ? formData.model : flatOptions[0]?.value ?? '',
              })
            }}
            options={EXECUTION_MODE_OPTIONS}
          />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Tier</label>
          <Dropdown
            value={formData.tier}
            onChange={(val) => setFormData({ ...formData, tier: val })}
            options={TIER_OPTIONS}
          />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Timeout (minutes)</label>
          <Input
            type="number"
            value={formData.timeout}
            onChange={(e) => setFormData({ ...formData, timeout: e.target.value })}
            placeholder="30"
          />
        </div>
      </div>
      <AgentModelOverrideField formData={formData} setFormData={setFormData} modelOptions={modelOptions} />
      <div>
        <label className="text-sm font-medium text-muted-foreground">
          Prompt {isCreate && <span className="text-destructive">*</span>}
        </label>
        <Textarea
          className="font-mono"
          rows={10}
          value={formData.prompt}
          onChange={(e) => setFormData({ ...formData, prompt: e.target.value })}
          placeholder="Agent prompt template..."
        />
      </div>
      <div className="grid grid-cols-4 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">Restart Threshold</label>
          <Input
            type="number"
            value={formData.restart_threshold}
            onChange={(e) => setFormData({ ...formData, restart_threshold: e.target.value })}
            placeholder="Optional"
          />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Max Fail Restarts</label>
          <Input
            type="number"
            value={formData.max_fail_restarts}
            onChange={(e) => setFormData({ ...formData, max_fail_restarts: e.target.value })}
            placeholder="Optional"
          />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Stall Start (sec)</label>
          <Input
            type="number"
            value={formData.stall_start_timeout_sec}
            onChange={(e) => setFormData({ ...formData, stall_start_timeout_sec: e.target.value })}
            placeholder="Optional"
          />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Stall Running (sec)</label>
          <Input
            type="number"
            value={formData.stall_running_timeout_sec}
            onChange={(e) => setFormData({ ...formData, stall_running_timeout_sec: e.target.value })}
            placeholder="Optional"
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
              ? !formData.id.trim() || !formData.prompt.trim() || mutation.isPending
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
