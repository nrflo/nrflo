import { Spinner } from '@/components/ui/Spinner'
import { resolveTierChain, useTierModels } from '@/hooks/useTierModels'
import { TierChainRow } from './TierChainRow'

const TIERS = [1, 2, 3, 4, 5]

const PROVIDER_LABELS: Record<string, string> = { anthropic: 'Anthropic', openai: 'OpenAI', openrouter: 'OpenRouter' }

// TierModelsSection is the "Tier Models" settings sub-tab: an ordered
// fallback-chain editor per tier (1-5), covering both system agents and
// regular workflow agents. There is no separate active-provider switch —
// the chain ORDER within each tier is the deployment-level provider
// preference: reorder entries to change which provider is preferred.
// Empty tiers show the chain inherited from the nearest lower populated
// tier, greyed out.
export function TierModelsSection() {
  const { data: rows = [], isLoading, error } = useTierModels()
  const providersInUse = Array.from(new Set(rows.map((r) => PROVIDER_LABELS[r.provider] ?? r.provider))).sort()

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Tier Models</h2>
        <p className="text-sm text-muted-foreground">
          Ordered fallback chains for tiers 1-5, used by system agents and regular workflow agents alike.
          Position 1 is the primary model and — since there is no separate active-provider setting — also
          the deployment's preferred provider; agents fall back down the chain on repeated failure.
        </p>
        {providersInUse.length > 0 && (
          <p className="text-xs text-muted-foreground mt-1">Providers in use: {providersInUse.join(', ')}</p>
        )}
      </div>

      {isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : error ? (
        <p className="text-destructive text-sm">
          {error instanceof Error ? error.message : 'Failed to load tier models'}
        </p>
      ) : (
        <div className="space-y-4">
          {TIERS.map((tier) => {
            const savedEntries = rows.filter((r) => r.tier === tier).sort((a, b) => a.position - b.position)
            const inheritedEntries = savedEntries.length === 0 ? resolveTierChain(rows, tier - 1) : []
            return (
              <TierChainRow
                key={tier}
                tier={tier}
                savedEntries={savedEntries}
                inheritedEntries={inheritedEntries}
              />
            )
          })}
        </div>
      )}
    </div>
  )
}
