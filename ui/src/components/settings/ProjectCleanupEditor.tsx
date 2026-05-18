import { useState, useEffect } from 'react'
import { Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Toggle } from '@/components/ui/Toggle'
import { useCleanup, useSetCleanup } from '@/hooks/useProjectSettings'

export function ProjectCleanupEditor({ projectId }: { projectId: string }) {
  const { data } = useCleanup(projectId)
  const mutation = useSetCleanup()
  const [enabled, setEnabled] = useState(false)
  const [retentionLimit, setRetentionLimit] = useState(0)
  const [serverError, setServerError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (data) {
      setEnabled(data.enabled)
      setRetentionLimit(data.retention_limit)
    }
  }, [data])

  function handleSubmit() {
    setServerError(null)
    setSaved(false)
    mutation.mutate(
      { projectId, cfg: { enabled, retention_limit: retentionLimit } },
      {
        onSuccess: () => setSaved(true),
        onError: (err) => setServerError((err as Error).message),
      }
    )
  }

  return (
    <div className="border-t border-border pt-3 space-y-3">
      <div className="text-sm font-medium text-muted-foreground">Workflow Cleanup</div>
      <Toggle
        checked={enabled}
        onChange={(checked) => { setEnabled(checked); setSaved(false) }}
        label="Enable cleanup"
      />
      {!enabled && (
        <p className="text-xs text-muted-foreground">When disabled (default), workflow instances are kept indefinitely.</p>
      )}
      {enabled && (
        <div>
          <label className="text-sm font-medium text-muted-foreground">Retention limit (instances per workflow)</label>
          <Input
            type="number"
            value={retentionLimit}
            onChange={(e) => setRetentionLimit(Number(e.target.value))}
            placeholder="e.g. 1000"
            min={10}
          />
        </div>
      )}
      <div className="flex gap-2 justify-end">
        <Button onClick={handleSubmit} disabled={mutation.isPending}>
          {mutation.isPending ? 'Saving...' : <><Check className="h-4 w-4 mr-1" />Save</>}
        </Button>
      </div>
      {saved && !mutation.isPending && (
        <p className="text-sm text-green-600 dark:text-green-400">Saved.</p>
      )}
      {serverError && (
        <p className="text-sm text-destructive">{serverError}</p>
      )}
    </div>
  )
}
