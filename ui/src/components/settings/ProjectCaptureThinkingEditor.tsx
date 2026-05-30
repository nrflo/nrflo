import { Dropdown } from '@/components/ui/Dropdown'
import { useCaptureThinking, useSetCaptureThinking } from '@/hooks/useProjectSettings'

const OPTIONS = [
  { value: 'inherit', label: 'Inherit (global)' },
  { value: 'on', label: 'On' },
  { value: 'off', label: 'Off' },
]

function toDropdownValue(enabled: boolean, inherited: boolean): string {
  if (inherited) return 'inherit'
  return enabled ? 'on' : 'off'
}

function toEnabled(value: string): boolean | null {
  if (value === 'inherit') return null
  return value === 'on'
}

export function ProjectCaptureThinkingEditor({ projectId }: { projectId: string }) {
  const { data } = useCaptureThinking(projectId)
  const mutation = useSetCaptureThinking()

  const selected = data ? toDropdownValue(data.enabled, data.inherited) : 'inherit'
  const globalResolved = data?.inherited ? (data.enabled ? 'On' : 'Off') : null

  return (
    <div className="border-t border-border pt-3 space-y-3">
      <div className="text-sm font-medium text-muted-foreground">Capture Model Thinking</div>
      <Dropdown
        value={selected}
        onChange={(value) => mutation.mutate({ projectId, enabled: toEnabled(value) })}
        options={OPTIONS}
        disabled={mutation.isPending}
      />
      {selected === 'inherit' && globalResolved && (
        <p className="text-xs text-muted-foreground">Global setting: {globalResolved}</p>
      )}
      {mutation.isError && (
        <p className="text-sm text-destructive">{(mutation.error as Error).message}</p>
      )}
    </div>
  )
}
