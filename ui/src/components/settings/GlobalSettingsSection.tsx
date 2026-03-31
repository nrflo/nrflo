import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { Toggle } from '@/components/ui/Toggle'
import { Input } from '@/components/ui/Input'
import { Tooltip } from '@/components/ui/Tooltip'
import { getGlobalSettings, updateGlobalSettings, settingsKeys } from '@/api/settings'
import { parseOptionalInt } from './AgentForm'
import { Info } from 'lucide-react'

export function GlobalSettingsSection() {
  const queryClient = useQueryClient()
  const [retentionLimit, setRetentionLimit] = useState<number>(1000)
  const [stallStartTimeout, setStallStartTimeout] = useState<string>('')
  const [stallRunningTimeout, setStallRunningTimeout] = useState<string>('')

  const { data: settings, isLoading, error } = useQuery({
    queryKey: settingsKeys.global(),
    queryFn: getGlobalSettings,
  })

  useEffect(() => {
    if (settings?.session_retention_limit != null) {
      setRetentionLimit(settings.session_retention_limit)
    }
  }, [settings?.session_retention_limit])

  useEffect(() => {
    if (settings) {
      setStallStartTimeout(settings.stall_start_timeout_sec != null ? String(settings.stall_start_timeout_sec) : '')
      setStallRunningTimeout(settings.stall_running_timeout_sec != null ? String(settings.stall_running_timeout_sec) : '')
    }
  }, [settings?.stall_start_timeout_sec, settings?.stall_running_timeout_sec])

  const toggleMutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ low_consumption_mode: val }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  const retentionMutation = useMutation({
    mutationFn: (val: number) => updateGlobalSettings({ session_retention_limit: val }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  const submitRetention = () => {
    if (retentionLimit >= 10 && retentionLimit !== settings?.session_retention_limit) {
      retentionMutation.mutate(retentionLimit)
    } else if (retentionLimit < 10) {
      setRetentionLimit(settings?.session_retention_limit ?? 1000)
    }
  }

  const stallMutation = useMutation({
    mutationFn: (data: Partial<{ stall_start_timeout_sec: number | null; stall_running_timeout_sec: number | null }>) =>
      updateGlobalSettings(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  const submitStallStart = () => {
    const parsed = parseOptionalInt(stallStartTimeout)
    if (parsed !== null && parsed < 0) {
      setStallStartTimeout(settings?.stall_start_timeout_sec != null ? String(settings.stall_start_timeout_sec) : '')
      return
    }
    if (parsed !== settings?.stall_start_timeout_sec) {
      stallMutation.mutate({ stall_start_timeout_sec: parsed })
    }
  }

  const submitStallRunning = () => {
    const parsed = parseOptionalInt(stallRunningTimeout)
    if (parsed !== null && parsed < 0) {
      setStallRunningTimeout(settings?.stall_running_timeout_sec != null ? String(settings.stall_running_timeout_sec) : '')
      return
    }
    if (parsed !== settings?.stall_running_timeout_sec) {
      stallMutation.mutate({ stall_running_timeout_sec: parsed })
    }
  }

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
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <div className="text-sm font-medium">Low consumption mode</div>
                <p className="text-xs text-muted-foreground mt-0.5">
                  When enabled, agents with a configured alternative will use their low-consumption substitute
                </p>
              </div>
              <Toggle
                checked={settings.low_consumption_mode}
                onChange={(val) => toggleMutation.mutate(val)}
                disabled={toggleMutation.isPending}
              />
            </div>
            <div className="border-t border-border" />
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-1.5">
                <div className="text-sm font-medium">Session retention limit</div>
                <Tooltip
                  placement="right"
                  text="Maximum number of completed agent sessions to keep per cleanup cycle (every 20 min). Associated agent messages are automatically removed with their sessions. Minimum: 10, Default: 1000."
                >
                  <Info className="h-3.5 w-3.5 text-muted-foreground" />
                </Tooltip>
              </div>
              <Input
                type="number"
                min={10}
                value={retentionLimit}
                onChange={(e) => setRetentionLimit(Number(e.target.value))}
                onBlur={submitRetention}
                onKeyDown={(e) => { if (e.key === 'Enter') submitRetention() }}
                disabled={retentionMutation.isPending}
                className="w-24"
              />
            </div>
            <div className="border-t border-border" />
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-1.5">
                <div className="text-sm font-medium">Stall start timeout (sec)</div>
                <Tooltip
                  placement="right"
                  text="Time before first agent message before triggering stall restart. 0 = disabled, empty = default (120s). Per-agent values take precedence."
                >
                  <Info className="h-3.5 w-3.5 text-muted-foreground" />
                </Tooltip>
              </div>
              <Input
                type="text"
                value={stallStartTimeout}
                onChange={(e) => setStallStartTimeout(e.target.value)}
                onBlur={submitStallStart}
                onKeyDown={(e) => { if (e.key === 'Enter') submitStallStart() }}
                disabled={stallMutation.isPending}
                placeholder="120"
                className="w-24"
              />
            </div>
            <div className="border-t border-border" />
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-1.5">
                <div className="text-sm font-medium">Stall running timeout (sec)</div>
                <Tooltip
                  placement="right"
                  text="Time without output from a running agent before triggering stall restart. 0 = disabled, empty = default (480s). Per-agent values take precedence."
                >
                  <Info className="h-3.5 w-3.5 text-muted-foreground" />
                </Tooltip>
              </div>
              <Input
                type="text"
                value={stallRunningTimeout}
                onChange={(e) => setStallRunningTimeout(e.target.value)}
                onBlur={submitStallRunning}
                onKeyDown={(e) => { if (e.key === 'Enter') submitStallRunning() }}
                disabled={stallMutation.isPending}
                placeholder="480"
                className="w-24"
              />
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
