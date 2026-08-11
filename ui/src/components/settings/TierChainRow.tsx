import { useEffect, useState } from 'react'
import { Plus, Trash2, ChevronUp, ChevronDown } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { cn } from '@/lib/utils'
import { useSetTierChain } from '@/hooks/useTierModels'
import { TierChainEntryForm } from './TierChainEntryForm'
import type { SetTierChainEntry, TierModel } from '@/api/tierModels'

function toEntry(row: TierModel): SetTierChainEntry {
  return { execution_mode: row.execution_mode, model_id: row.model_id, reasoning_effort: row.reasoning_effort, weight: row.weight }
}

const EMPTY_ENTRY: SetTierChainEntry = { execution_mode: '', model_id: '', reasoning_effort: '', weight: 0 }

// TierChainRow is one tier's ordered fallback-chain editor: add/remove
// entries plus up/down reorder (position = fallback priority, 1 = primary).
// When the tier has no saved rows it renders the inherited chain greyed out
// with a note instead of an editable list.
export function TierChainRow({
  tier,
  savedEntries,
  inheritedEntries,
}: {
  tier: number
  savedEntries: TierModel[]
  inheritedEntries: TierModel[]
}) {
  const isInherited = savedEntries.length === 0
  const [entries, setEntries] = useState<SetTierChainEntry[]>(
    isInherited ? [] : savedEntries.map(toEntry)
  )
  const setTierChain = useSetTierChain()

  useEffect(() => {
    setEntries(isInherited ? [] : savedEntries.map(toEntry))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tier, isInherited, savedEntries.map((e) => `${e.position}:${e.model_id}:${e.reasoning_effort}:${e.weight}`).join(',')])

  const moveUp = (index: number) => {
    if (index === 0) return
    const next = [...entries]
    ;[next[index - 1], next[index]] = [next[index], next[index - 1]]
    setEntries(next)
  }

  const moveDown = (index: number) => {
    if (index === entries.length - 1) return
    const next = [...entries]
    ;[next[index], next[index + 1]] = [next[index + 1], next[index]]
    setEntries(next)
  }

  const removeAt = (index: number) => setEntries(entries.filter((_, i) => i !== index))

  const addEntry = () => setEntries([...entries, { ...EMPTY_ENTRY }])

  const updateAt = (index: number, entry: SetTierChainEntry) =>
    setEntries(entries.map((e, i) => (i === index ? entry : e)))

  const dirty =
    isInherited
      ? entries.length > 0
      : JSON.stringify(entries) !== JSON.stringify(savedEntries.map(toEntry))

  const canSave = entries.every((e) => e.model_id)

  return (
    <div className="border border-border rounded-lg p-4 space-y-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <h3 className="text-sm font-semibold">Tier {tier}</h3>
          {isInherited && <Badge variant="secondary">Inherited</Badge>}
        </div>
        <Button size="sm" variant="ghost" onClick={addEntry}>
          <Plus className="h-4 w-4 mr-1" />
          Add entry
        </Button>
      </div>

      {isInherited && entries.length === 0 ? (
        <div className="opacity-60 space-y-1">
          <p className="text-xs text-muted-foreground">
            No chain set for this tier — inherits from the nearest lower populated tier.
          </p>
          {inheritedEntries.length === 0 ? (
            <p className="text-xs text-muted-foreground italic">No chain available to inherit.</p>
          ) : (
            inheritedEntries.map((row, i) => (
              <div key={`${row.tier}-${row.position}`} className="text-xs flex items-center gap-2">
                <span className="w-4 text-right shrink-0">{i + 1}.</span>
                <span>
                  {row.model_id} ({row.execution_mode || 'agent mode'}) — {row.reasoning_effort || 'default effort'} · from tier {row.tier}
                </span>
              </div>
            ))
          )}
        </div>
      ) : (
        <div className="space-y-2">
          {entries.length === 0 && (
            <p className="text-xs text-muted-foreground">No entries — add one to set a chain for this tier.</p>
          )}
          {entries.map((entry, index) => (
            <div key={index} className={cn('flex items-end gap-2 rounded-md border border-border/60 p-2')}>
              <span className="text-xs text-muted-foreground w-16 shrink-0 pb-2">
                {index === 0 ? 'Primary' : `Fallback ${index}`}
              </span>
              <TierChainEntryForm entry={entry} onChange={(e) => updateAt(index, e)} />
              <div className="flex items-center gap-0.5 shrink-0 pb-1.5">
                <button
                  type="button"
                  onClick={() => moveUp(index)}
                  disabled={index === 0}
                  className="p-0.5 rounded hover:bg-muted disabled:opacity-30 disabled:cursor-not-allowed"
                  aria-label={`Move entry ${index + 1} up`}
                >
                  <ChevronUp className="h-4 w-4" />
                </button>
                <button
                  type="button"
                  onClick={() => moveDown(index)}
                  disabled={index === entries.length - 1}
                  className="p-0.5 rounded hover:bg-muted disabled:opacity-30 disabled:cursor-not-allowed"
                  aria-label={`Move entry ${index + 1} down`}
                >
                  <ChevronDown className="h-4 w-4" />
                </button>
                <button
                  type="button"
                  onClick={() => removeAt(index)}
                  className="p-0.5 rounded hover:bg-muted text-destructive"
                  aria-label={`Remove entry ${index + 1}`}
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      <div className="flex items-center justify-end gap-2">
        {setTierChain.isError && (
          <p className="text-destructive text-xs mr-auto">
            {setTierChain.error instanceof Error ? setTierChain.error.message : 'Save failed'}
          </p>
        )}
        <Button
          size="sm"
          disabled={!dirty || !canSave || setTierChain.isPending}
          onClick={() => setTierChain.mutate({ tier, entries })}
        >
          {setTierChain.isPending ? 'Saving…' : 'Save'}
        </Button>
      </div>
    </div>
  )
}
