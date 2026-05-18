import { Input } from '@/components/ui/Input'
import { Toggle } from '@/components/ui/Toggle'

export interface CleanupFormState {
  enabled: boolean
  retentionLimit: number
}

export function ProjectCleanupEditor({
  value,
  onChange,
  serverError,
}: {
  projectId: string
  value: CleanupFormState
  onChange: (next: CleanupFormState) => void
  serverError?: string | null
}) {
  return (
    <div className="border-t border-border pt-3 space-y-3">
      <div className="text-sm font-medium text-muted-foreground">Workflow Cleanup</div>
      <Toggle
        checked={value.enabled}
        onChange={(checked) => onChange({ ...value, enabled: checked })}
        label="Enable cleanup"
      />
      {!value.enabled && (
        <p className="text-xs text-muted-foreground">When disabled (default), workflow instances are kept indefinitely.</p>
      )}
      {value.enabled && (
        <div>
          <label className="text-sm font-medium text-muted-foreground">Retention limit (instances per workflow)</label>
          <Input
            type="number"
            value={value.retentionLimit}
            onChange={(e) => onChange({ ...value, retentionLimit: Number(e.target.value) })}
            placeholder="e.g. 1000"
            min={10}
          />
        </div>
      )}
      {serverError && (
        <p className="text-sm text-destructive">{serverError}</p>
      )}
    </div>
  )
}
