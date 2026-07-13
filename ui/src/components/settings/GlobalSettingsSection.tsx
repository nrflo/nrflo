import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { Toggle } from '@/components/ui/Toggle'
import { Tooltip } from '@/components/ui/Tooltip'
import { getGlobalSettings, updateGlobalSettings, settingsKeys } from '@/api/settings'
import { ObserverSettingsSection } from './ObserverSettingsSection'
import { GlobalStallSettings } from './GlobalStallSettings'
import { Info } from 'lucide-react'

export function GlobalSettingsSection() {
  const queryClient = useQueryClient()

  const { data: settings, isLoading, error } = useQuery({
    queryKey: settingsKeys.global(),
    queryFn: getGlobalSettings,
  })

  const apiModeMutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ api_mode_enabled: val }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  const systemPromptOverrideMutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ claude_system_prompt_override_enabled: val }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  const toggleMutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ low_consumption_mode: val }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  const contextSaveMutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ context_save_via_agent: val }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  const simplifiedGraphMutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ simplified_agents_graph: val }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  const experimentalMutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ experimental: val }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  const apiViaCLIMutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ api_via_cli_enabled: val }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  const captureThinkingMutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ capture_thinking_enabled: val }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
    },
  })

  const dynamicWorkflowAutoMutation = useMutation({
    mutationFn: (val: boolean) => updateGlobalSettings({ dynamic_workflow_auto_enabled: val }),
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
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <div className="text-sm font-medium">Enable API mode</div>
                <p className="text-xs text-muted-foreground mt-0.5">
                  When enabled, agents with execution_mode=api can run; toggles visibility of API-mode-only tabs (Tool Definitions, Review, Config Files, Insights)
                </p>
              </div>
              <Toggle
                checked={settings.api_mode_enabled}
                onChange={(val) => apiModeMutation.mutate(val)}
                disabled={apiModeMutation.isPending}
              />
            </div>
            <div className="border-t border-border" />
            <div className="flex items-center justify-between">
              <div>
                <div className="text-sm font-medium">Override Claude system prompt</div>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Replaces the default Claude Code system prompt with the system-prompt injectable
                  (edit it under Settings → Default Templates); the completion-contract suffix is
                  still appended
                </p>
              </div>
              <Toggle
                checked={settings.claude_system_prompt_override_enabled}
                onChange={(val) => systemPromptOverrideMutation.mutate(val)}
                disabled={systemPromptOverrideMutation.isPending}
              />
            </div>
            <div className="border-t border-border" />
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
                <div>
                  <div className="text-sm font-medium">Save context via agent</div>
                  <p className="text-xs text-muted-foreground mt-0.5">
                    Use a system agent to summarize context on low-context restarts
                  </p>
                </div>
                <Tooltip
                  placement="right"
                  className="max-w-sm"
                  text="When enabled, a dedicated system agent (haiku) summarizes the message history during low-context restarts. Works for all CLI types. When disabled, the original Claude session is resumed with a save prompt (Claude CLI only; other CLIs skip context save)."
                >
                  <Info className="h-3.5 w-3.5 text-muted-foreground" />
                </Tooltip>
              </div>
              <Toggle
                checked={settings.context_save_via_agent}
                onChange={(val) => contextSaveMutation.mutate(val)}
                disabled={contextSaveMutation.isPending}
              />
            </div>
            <div className="border-t border-border" />
            <div className="flex items-center justify-between">
              <div>
                <div className="text-sm font-medium">Use simplified agents graph</div>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Show agents as a flat table instead of the interactive graph
                </p>
              </div>
              <Toggle
                checked={settings.simplified_agents_graph ?? false}
                onChange={(val) => simplifiedGraphMutation.mutate(val)}
                disabled={simplifiedGraphMutation.isPending}
              />
            </div>
            <div className="border-t border-border" />
            <div className="flex items-center justify-between">
              <div>
                <div className="text-sm font-medium">Experimental features</div>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Show experimental UI sections (Workflow Chains, Python Scripts).
                </p>
              </div>
              <Toggle
                checked={settings.experimental ?? false}
                onChange={(val) => experimentalMutation.mutate(val)}
                disabled={experimentalMutation.isPending}
              />
            </div>
            <div className="border-t border-border" />
            <div className="flex items-center justify-between">
              <div>
                <div className="text-sm font-medium">Route API agents via Claude CLI</div>
                <p className="text-xs text-muted-foreground mt-0.5">
                  When enabled, agents with execution_mode=api are launched via the Claude CLI instead of the direct Anthropic API
                </p>
              </div>
              <Toggle
                checked={settings.api_via_cli_enabled ?? false}
                onChange={(val) => apiViaCLIMutation.mutate(val)}
                disabled={apiViaCLIMutation.isPending}
              />
            </div>
            <div className="border-t border-border" />
            <ObserverSettingsSection settings={settings} />
            <div className="border-t border-border" />
            <div className="flex items-center justify-between">
              <div>
                <div className="text-sm font-medium">Capture model thinking</div>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Surface extended-thinking/reasoning text in the agent log window
                </p>
              </div>
              <Toggle
                checked={settings.capture_thinking_enabled ?? false}
                onChange={(val) => captureThinkingMutation.mutate(val)}
                disabled={captureThinkingMutation.isPending}
              />
            </div>
            <div className="border-t border-border" />
            <div className="flex items-center justify-between">
              <div>
                <div className="text-sm font-medium">Allow dynamic_workflow mode=auto</div>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Plans are approved automatically after validation, skipping human review
                </p>
              </div>
              <Toggle
                checked={settings.dynamic_workflow_auto_enabled ?? false}
                onChange={(val) => dynamicWorkflowAutoMutation.mutate(val)}
                disabled={dynamicWorkflowAutoMutation.isPending}
              />
            </div>
            <GlobalStallSettings settings={settings} />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
