import { X, Check, AlertTriangle, Lock } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Dropdown } from '@/components/ui/Dropdown'

export interface CLIModelFormData {
  id: string
  cli_type: string
  display_name: string
  mapped_model: string
  reasoning_effort: string
  context_length: string
}

export const emptyCLIModelForm: CLIModelFormData = {
  id: '',
  cli_type: 'claude',
  display_name: '',
  mapped_model: '',
  reasoning_effort: '',
  context_length: '200000',
}

const CLI_TYPE_OPTIONS = [
  { value: 'claude', label: 'Claude' },
  { value: 'opencode', label: 'OpenCode' },
  { value: 'codex', label: 'Codex' },
]

const REASONING_EFFORT_OPTIONS = [
  { value: '', label: 'Default' },
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
  { value: 'xhigh', label: 'Extra High (Opus 4.7 only)' },
  { value: 'max', label: 'Max' },
]

function buildEffortOptions(cliType: string, mappedModel: string) {
  if (cliType === 'claude') {
    const isOpus47 = mappedModel.startsWith('claude-opus-4-7')
    return REASONING_EFFORT_OPTIONS.map((opt) =>
      opt.value === 'xhigh' && !isOpus47
        ? { ...opt, disabled: true, tooltip: "'xhigh' is only supported on Opus 4.7 Claude models" }
        : opt
    )
  }
  return REASONING_EFFORT_OPTIONS.filter((opt) => opt.value !== 'xhigh')
}

export function CLIModelForm({
  formData,
  setFormData,
  onCancel,
  onSave,
  mutation,
  isCreate,
  readOnly,
}: {
  formData: CLIModelFormData
  setFormData: (data: CLIModelFormData) => void
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
      {formData.cli_type === 'codex' && (
        <div className="flex items-center gap-2 rounded-md border border-amber-500/50 bg-amber-500/10 px-3 py-2 text-sm text-amber-600 dark:text-amber-400">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          Codex agents run in read-only sandboxed environments
        </div>
      )}
      {formData.cli_type === 'opencode' && (
        <div className="flex items-center gap-2 rounded-md border border-amber-500/50 bg-amber-500/10 px-3 py-2 text-sm text-amber-600 dark:text-amber-400">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          OpenCode support is experimental
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
            CLI Type {isCreate && <span className="text-destructive">*</span>}
          </label>
          {isCreate ? (
            <Dropdown
              value={formData.cli_type}
              onChange={(val) => setFormData({ ...formData, cli_type: val })}
              options={CLI_TYPE_OPTIONS}
            />
          ) : (
            <Input value={formData.cli_type} disabled className="bg-muted" />
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
            placeholder="claude-sonnet-4-20250514"
            disabled={lockBuiltIn}
            className={lockBuiltIn ? 'bg-muted' : undefined}
          />
        </div>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">Reasoning Effort</label>
          {/* TODO(test-writer): cover dropdown options rendering, xhigh hidden for opencode/codex, xhigh disabled with tooltip for non-Opus-4.7 claude, setFormData on select. */}
          <Dropdown
            value={formData.reasoning_effort}
            onChange={(val) => setFormData({ ...formData, reasoning_effort: val })}
            options={buildEffortOptions(formData.cli_type, formData.mapped_model)}
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
