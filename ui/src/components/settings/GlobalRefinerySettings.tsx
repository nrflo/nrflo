import { useState, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Input } from '@/components/ui/Input'
import { Tooltip } from '@/components/ui/Tooltip'
import { Info } from 'lucide-react'
import { updateGlobalSettings, settingsKeys, type GlobalSettings } from '@/api/settings'
import { parseOptionalInt } from './AgentForm'

type PctKey = 'refinery_fold_start_context_pct' | 'refinery_console_fold_start_context_pct'

function PctSettingRow({
  settings,
  settingKey,
  label,
  tooltip,
  placeholder,
}: {
  settings: GlobalSettings
  settingKey: PctKey
  label: string
  tooltip: string
  placeholder: string
}) {
  const queryClient = useQueryClient()
  const stored = settings[settingKey]
  const [value, setValue] = useState<string>(stored != null ? String(stored) : '')

  useEffect(() => {
    setValue(stored != null ? String(stored) : '')
  }, [stored])

  const mutation = useMutation({
    mutationFn: (data: Partial<GlobalSettings>) => updateGlobalSettings(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: settingsKeys.all }),
  })

  const submit = () => {
    const parsed = parseOptionalInt(value)
    if (parsed !== null && (parsed < 0 || parsed > 100)) {
      setValue(stored != null ? String(stored) : '')
      return
    }
    if (parsed !== stored) mutation.mutate({ [settingKey]: parsed })
  }

  return (
    <div className="flex items-center justify-between">
      <div className="flex items-center gap-1.5">
        <div className="text-sm font-medium">{label}</div>
        <Tooltip placement="right" className="max-w-sm" text={tooltip}>
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
        placeholder={placeholder}
        className="w-24"
      />
    </div>
  )
}

export function GlobalRefinerySettings({ settings }: { settings: GlobalSettings }) {
  return (
    <>
      <div className="border-t border-border" />
      <PctSettingRow
        settings={settings}
        settingKey="refinery_fold_start_context_pct"
        label="Refinery fold-start context (%)"
        tooltip="% context free below which the autonomous refinery begins folding. Default 60. Kill/relaunch fires at 25%."
        placeholder="60"
      />
      <PctSettingRow
        settings={settings}
        settingKey="refinery_console_fold_start_context_pct"
        label="Console refinery fold-start context (%)"
        tooltip="% context free below which console-chat refinery folding begins. Default 75 (folds once ≥25% of context is used) — a barely-used chat never folds."
        placeholder="75"
      />
    </>
  )
}
