import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { Toggle } from '@/components/ui/Toggle'
import { ProviderModelsList } from './ProviderModelsList'
import { getGlobalSettings, updateGlobalSettings, settingsKeys } from '@/api/settings'
import type { ProviderName } from '@/api/providers'


interface Props {
  activeProvider: ProviderName
}

export function ProvidersSection({ activeProvider }: Props) {
  const queryClient = useQueryClient()
  const { data: settings, isLoading: settingsLoading } = useQuery({
    queryKey: settingsKeys.global(),
    queryFn: getGlobalSettings,
  })
  const syncClaudeLimitsMutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ sync_claude_limits: val }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  return (
    <div className="space-y-4">
      {activeProvider === 'claude' && (
        <Card>
          <CardHeader>
            <CardTitle>Claude limits sync</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-center justify-between">
              <div>
                <div className="text-sm font-medium">Sync Claude limits</div>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Periodically run a tiny haiku CLI request once an hour to refresh the 5-hour and weekly Claude usage counters shown in the header. Skips when the cached limits are less than 30 minutes old.
                </p>
              </div>
              <Toggle
                checked={settings?.sync_claude_limits ?? false}
                onChange={(val) => syncClaudeLimitsMutation.mutate(val)}
                disabled={syncClaudeLimitsMutation.isPending || settingsLoading}
              />
            </div>
          </CardContent>
        </Card>
      )}

      <ProviderModelsList provider={activeProvider} />
    </div>
  )
}
