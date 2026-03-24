import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { Toggle } from '@/components/ui/Toggle'
import { getGlobalSettings, updateGlobalSettings, settingsKeys } from '@/api/settings'

export function GlobalSettingsSection() {
  const queryClient = useQueryClient()

  const { data: settings, isLoading, error } = useQuery({
    queryKey: settingsKeys.global(),
    queryFn: getGlobalSettings,
  })

  const mutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ low_consumption_mode: val }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>Global Settings</CardTitle>
        <CardDescription>System-wide configuration options</CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading && (
          <div className="text-center py-4 text-muted-foreground">Loading settings...</div>
        )}
        {error && (
          <div className="text-center py-4 text-destructive">
            Error: {(error as Error).message}
          </div>
        )}
        {settings && (
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm font-medium">Low consumption mode</div>
              <p className="text-xs text-muted-foreground mt-0.5">
                When enabled, agents with a configured alternative will use their low-consumption substitute
              </p>
            </div>
            <Toggle
              checked={settings.low_consumption_mode}
              onChange={(val) => mutation.mutate(val)}
              disabled={mutation.isPending}
            />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
