import { Fragment } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { Toggle } from '@/components/ui/Toggle'
import { getGlobalSettings, updateGlobalSettings, settingsKeys } from '@/api/settings'
import type { GlobalSettings } from '@/api/settings'

type MenuKey =
  | 'menu_new_ticket'
  | 'menu_import_spec'
  | 'menu_git'
  | 'menu_chain_executions'
  | 'menu_schedules'
  | 'menu_workflow_chains'
  | 'menu_python_scripts'
  | 'menu_documentation'
  | 'menu_errors'
  | 'menu_agent_sessions'

const rows: { key: MenuKey; label: string; defaultVal: boolean }[] = [
  { key: 'menu_new_ticket', label: 'New Ticket', defaultVal: false },
  { key: 'menu_import_spec', label: 'Import Spec', defaultVal: false },
  { key: 'menu_git', label: 'Git', defaultVal: true },
  { key: 'menu_chain_executions', label: 'Chain Executions', defaultVal: true },
  { key: 'menu_schedules', label: 'Schedules', defaultVal: false },
  { key: 'menu_workflow_chains', label: 'Workflow Chains', defaultVal: false },
  { key: 'menu_python_scripts', label: 'Python Scripts', defaultVal: false },
  { key: 'menu_documentation', label: 'Documentation', defaultVal: true },
  { key: 'menu_errors', label: 'Errors', defaultVal: false },
  { key: 'menu_agent_sessions', label: 'Agent Sessions', defaultVal: false },
]

export function MenuPanelSection() {
  const queryClient = useQueryClient()

  const { data: settings, isLoading, error } = useQuery({
    queryKey: settingsKeys.global(),
    queryFn: getGlobalSettings,
  })

  const menuMutation = useMutation({
    mutationFn: ({ key, val }: { key: MenuKey; val: boolean }) => {
      const patch: Partial<GlobalSettings> = {}
      patch[key] = val
      return updateGlobalSettings(patch)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>Menu Panel</CardTitle>
        <CardDescription>
          Toggle sidebar menu items. The top Header menu is independent and unaffected by these settings.
        </CardDescription>
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
          <div className="space-y-4">
            {rows.map(({ key, label, defaultVal }, idx) => (
              <Fragment key={key}>
                {idx > 0 && <div className="border-t border-border" />}
                <div className="flex items-center justify-between">
                  <div className="text-sm font-medium">{label}</div>
                  <Toggle
                    checked={settings[key] ?? defaultVal}
                    onChange={(val) => menuMutation.mutate({ key, val })}
                    disabled={menuMutation.isPending}
                  />
                </div>
              </Fragment>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
