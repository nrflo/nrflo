import { Spinner } from '@/components/ui/Spinner'
import { resolveTierChain, useTierModels } from '@/hooks/useTierModels'
import { TierChainRow } from './TierChainRow'

const TIERS = [1, 2, 3, 4, 5]

// TierModelsSection is the "Tier Models" settings sub-tab: an ordered
// fallback-chain editor per tier (1-5). Empty tiers show the chain inherited
// from the nearest lower populated tier, greyed out.
export function TierModelsSection() {
  const { data: rows = [], isLoading, error } = useTierModels()

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Tier Models</h2>
        <p className="text-sm text-muted-foreground">
          Ordered fallback chains for tiers 1-5. Position 1 is the primary model; agents assigned a tier
          fall back down the chain on repeated failure.
        </p>
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
