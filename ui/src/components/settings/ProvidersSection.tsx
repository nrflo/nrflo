import { useState } from 'react'
import { AlertTriangle } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { Toggle } from '@/components/ui/Toggle'
import { useProviders, useUpdateProvider } from '@/hooks/useProviders'
import { ProviderModelsList } from './ProviderModelsList'
import type { ProviderName, CLIMode } from '@/api/providers'


interface Props {
  activeProvider: ProviderName
}

export function ProvidersSection({ activeProvider }: Props) {
  const { data: providers, isLoading, error } = useProviders()
  const updateProvider = useUpdateProvider()
  const [modeError, setModeError] = useState<string | null>(null)

  const currentModes: CLIMode[] = providers?.[activeProvider]?.modes ?? []
  const cliEnabled = currentModes.includes('cli')
  const cliInteractiveEnabled = currentModes.includes('cli_interactive')

  const handleToggleMode = (mode: CLIMode, enabled: boolean) => {
    const nextModes = enabled
      ? ([...currentModes, mode] as CLIMode[])
      : currentModes.filter((m) => m !== mode)

    if (nextModes.length === 0) {
      setModeError('At least one mode must be enabled')
      return
    }
    setModeError(null)
    updateProvider.mutate({ name: activeProvider, modes: nextModes })
  }

  if (isLoading) {
    return <div className="text-center py-8 text-muted-foreground">Loading providers...</div>
  }

  if (error) {
    return (
      <div className="text-center py-8 text-destructive">
        Error: {(error as Error).message}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Modes</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">Allow cli</span>
              <Toggle
                checked={cliEnabled}
                disabled={updateProvider.isPending}
                onChange={(checked) => handleToggleMode('cli', checked)}
              />
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">Allow cli interactive</span>
              <Toggle
                checked={cliInteractiveEnabled}
                disabled={updateProvider.isPending}
                onChange={(checked) => handleToggleMode('cli_interactive', checked)}
              />
            </div>
            {modeError && (
              <p className="text-sm text-destructive">{modeError}</p>
            )}
          </div>
        </CardContent>
      </Card>

      {activeProvider === 'claude' && cliEnabled && (
        <div className="flex items-center gap-2 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-400">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          <span>Claude Code CLI (batch) will be billed at API rate starting June 15, 2026.</span>
        </div>
      )}

      <ProviderModelsList provider={activeProvider} />
    </div>
  )
}
