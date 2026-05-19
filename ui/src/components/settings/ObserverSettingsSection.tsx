import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Toggle } from '@/components/ui/Toggle'
import { Textarea } from '@/components/ui/Textarea'
import { Dropdown } from '@/components/ui/Dropdown'
import { updateGlobalSettings, settingsKeys } from '@/api/settings'
import { useCLIModels } from '@/hooks/useCLIModels'
import type { GlobalSettings } from '@/api/settings'

interface ObserverSettingsSectionProps {
  settings: GlobalSettings
}

export function ObserverSettingsSection({ settings }: ObserverSettingsSectionProps) {
  const qc = useQueryClient()
  const { data: models = [] } = useCLIModels()

  const observerEnabledMutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ experimental_observer_enabled: val }),
    onSuccess: () => qc.invalidateQueries({ queryKey: settingsKeys.all }),
  })

  const observerSettingsMutation = useMutation({
    mutationFn: (data: Partial<Pick<GlobalSettings, 'observer_system_context' | 'observer_provider' | 'observer_model'>>) =>
      updateGlobalSettings(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: settingsKeys.all }),
  })

  const providerOptions = [
    { value: '', label: 'None (use default)' },
    ...Array.from(new Set(models.filter(m => m.enabled).map(m => m.cli_type))).map(t => ({
      value: t,
      label: t.charAt(0).toUpperCase() + t.slice(1),
    })),
  ]

  const modelOptions = [
    { value: '', label: 'None (use default)' },
    ...models
      .filter(m => m.enabled && (!settings.observer_provider || m.cli_type === settings.observer_provider))
      .map(m => ({ value: m.id, label: m.display_name })),
  ]

  return (
    <>
      <div className="flex items-center justify-between">
        <div>
          <div className="text-sm font-medium">Observer mode</div>
          <p className="text-xs text-muted-foreground mt-0.5">
            Enable observer agents that monitor workflow execution in real time.
          </p>
        </div>
        <Toggle
          checked={settings.experimental_observer_enabled ?? false}
          onChange={(val) => observerEnabledMutation.mutate(val)}
          disabled={observerEnabledMutation.isPending}
        />
      </div>
      {settings.experimental_observer_enabled && (
        <div className="space-y-3 pl-4 border-l-2 border-border">
          <div className="text-xs font-medium text-muted-foreground">Observer defaults</div>
          <div>
            <label className="text-sm font-medium text-muted-foreground">System context</label>
            <Textarea
              value={settings.observer_system_context ?? ''}
              onChange={(e) => observerSettingsMutation.mutate({ observer_system_context: e.target.value })}
              rows={3}
              placeholder="System context for observer agents"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-sm font-medium text-muted-foreground">Provider</label>
              <Dropdown
                value={settings.observer_provider ?? ''}
                onChange={(v) => observerSettingsMutation.mutate({ observer_provider: v })}
                options={providerOptions}
              />
            </div>
            <div>
              <label className="text-sm font-medium text-muted-foreground">Model</label>
              <Dropdown
                value={settings.observer_model ?? ''}
                onChange={(v) => observerSettingsMutation.mutate({ observer_model: v })}
                options={modelOptions}
              />
            </div>
          </div>
        </div>
      )}
    </>
  )
}
