import { useState, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Input } from '@/components/ui/Input'
import { Tooltip } from '@/components/ui/Tooltip'
import { Info } from 'lucide-react'
import { updateGlobalSettings, settingsKeys, type GlobalSettings } from '@/api/settings'
import { parseOptionalInt } from './AgentForm'

export function GlobalRefinerySettings({ settings }: { settings: GlobalSettings }) {
  const queryClient = useQueryClient()
  const [value, setValue] = useState<string>(
    settings.refinery_fold_start_context_pct != null ? String(settings.refinery_fold_start_context_pct) : ''
  )

  useEffect(() => {
    setValue(settings.refinery_fold_start_context_pct != null ? String(settings.refinery_fold_start_context_pct) : '')
  }, [settings.refinery_fold_start_context_pct])

  const mutation = useMutation({
    mutationFn: (data: Partial<GlobalSettings>) => updateGlobalSettings(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: settingsKeys.all }),
  })

  const submit = () => {
    const parsed = parseOptionalInt(value)
    if (parsed !== null && (parsed < 0 || parsed > 100)) {
      setValue(settings.refinery_fold_start_context_pct != null ? String(settings.refinery_fold_start_context_pct) : '')
      return
    }
    if (parsed !== settings.refinery_fold_start_context_pct) mutation.mutate({ refinery_fold_start_context_pct: parsed })
  }

  return (
    <>
      <div className="border-t border-border" />
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-1.5">
          <div className="text-sm font-medium">Refinery fold-start context (%)</div>
          <Tooltip
            placement="right"
            className="max-w-sm"
            text="% context free below which the autonomous refinery begins folding. Default 40. ~95% of sessions finish above this and never fold. Kill/relaunch fires at 25%."
          >
            <Info className="h-3.5 w-3.5 text-muted-foreground" />
          </Tooltip>
        </div>
        <Input
          type="text"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onBlur={submit}
          onKeyDown={(e) => { if (e.key === 'Enter') submit() }}
          disabled={mutation.isPending}
          placeholder="40"
          className="w-24"
        />
      </div>
    </>
  )
}
