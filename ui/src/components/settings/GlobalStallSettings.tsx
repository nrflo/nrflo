import { useState, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Input } from '@/components/ui/Input'
import { Tooltip } from '@/components/ui/Tooltip'
import { Info } from 'lucide-react'
import { updateGlobalSettings, settingsKeys, type GlobalSettings } from '@/api/settings'
import { parseOptionalInt } from './AgentForm'

export function GlobalStallSettings({ settings }: { settings: GlobalSettings }) {
  const queryClient = useQueryClient()
  const [stallStartTimeout, setStallStartTimeout] = useState<string>(
    settings.stall_start_timeout_sec != null ? String(settings.stall_start_timeout_sec) : ''
  )
  const [stallRunningTimeout, setStallRunningTimeout] = useState<string>(
    settings.stall_running_timeout_sec != null ? String(settings.stall_running_timeout_sec) : ''
  )

  useEffect(() => {
    setStallStartTimeout(settings.stall_start_timeout_sec != null ? String(settings.stall_start_timeout_sec) : '')
    setStallRunningTimeout(settings.stall_running_timeout_sec != null ? String(settings.stall_running_timeout_sec) : '')
  }, [settings.stall_start_timeout_sec, settings.stall_running_timeout_sec])

  const stallMutation = useMutation({
    mutationFn: (data: Partial<{ stall_start_timeout_sec: number | null; stall_running_timeout_sec: number | null }>) =>
      updateGlobalSettings(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: settingsKeys.all }),
  })

  const submitStallStart = () => {
    const parsed = parseOptionalInt(stallStartTimeout)
    if (parsed !== null && parsed < 0) {
      setStallStartTimeout(settings.stall_start_timeout_sec != null ? String(settings.stall_start_timeout_sec) : '')
      return
    }
    if (parsed !== settings.stall_start_timeout_sec) stallMutation.mutate({ stall_start_timeout_sec: parsed })
  }

  const submitStallRunning = () => {
    const parsed = parseOptionalInt(stallRunningTimeout)
    if (parsed !== null && parsed < 0) {
      setStallRunningTimeout(settings.stall_running_timeout_sec != null ? String(settings.stall_running_timeout_sec) : '')
      return
    }
    if (parsed !== settings.stall_running_timeout_sec) stallMutation.mutate({ stall_running_timeout_sec: parsed })
  }

  return (
    <>
      <div className="border-t border-border" />
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-1.5">
          <div className="text-sm font-medium">Stall start timeout (sec)</div>
          <Tooltip
            placement="right"
            className="max-w-sm"
            text={
              <div className="space-y-2">
                <p>Time before first agent message before triggering stall restart. 0 = disabled, empty = default (120s). Per-agent values take precedence.</p>
                <p>Note: Claude CLI intermittently drops tool_use blocks from streaming API responses — the API generates the full response (and bills for it), but the CLI only receives the thinking block, has nothing to execute, and exits with code 0 immediately. This is a known open issue (anthropics/claude-code#25979) in the SSE streaming pipeline with no fix or read-timeout mechanism, affecting all versions.</p>
              </div>
            }
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
    </>
  )
}
