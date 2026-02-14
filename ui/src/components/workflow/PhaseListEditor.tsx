import { useMemo } from 'react'
import { Plus, Trash2, AlertTriangle } from 'lucide-react'
import { Button } from '@/components/ui/Button'

export interface PhaseFormEntry {
  agent: string
  layer: number
}

interface PhaseListEditorProps {
  value: PhaseFormEntry[]
  onChange: (phases: PhaseFormEntry[]) => void
}

/** Check fan-in: if layer N has >1 agent, next non-empty layer must have exactly 1 agent */
function getFanInErrors(entries: PhaseFormEntry[]): Record<number, string> {
  const errors: Record<number, string> = {}
  const byLayer: Record<number, PhaseFormEntry[]> = {}
  for (const e of entries) {
    if (!byLayer[e.layer]) byLayer[e.layer] = []
    byLayer[e.layer].push(e)
  }
  const layers = Object.keys(byLayer).map(Number).sort((a, b) => a - b)
  for (let i = 0; i < layers.length - 1; i++) {
    if (byLayer[layers[i]].length > 1) {
      const nextLayer = layers[i + 1]
      if (byLayer[nextLayer].length !== 1) {
        errors[nextLayer] = `Fan-in violation: layer ${nextLayer} must have exactly 1 agent because layer ${layers[i]} has ${byLayer[layers[i]].length} agents`
      }
    }
  }
  return errors
}

export function PhaseListEditor({ value, onChange }: PhaseListEditorProps) {
  const fanInErrors = useMemo(() => getFanInErrors(value), [value])

  const update = (index: number, entry: PhaseFormEntry) => {
    const next = [...value]
    next[index] = entry
    onChange(next)
  }

  const remove = (index: number) => {
    onChange(value.filter((_, i) => i !== index))
  }

  const add = () => {
    const maxLayer = value.length > 0 ? Math.max(...value.map((e) => e.layer)) : -1
    onChange([...value, { agent: '', layer: maxLayer + 1 }])
  }

  // Group entries by layer for display
  const sorted = useMemo(() => {
    const indexed = value.map((entry, i) => ({ entry, i }))
    indexed.sort((a, b) => a.entry.layer - b.entry.layer || a.i - b.i)
    return indexed
  }, [value])

  // Track layer boundaries for group headers
  let lastLayer = -1

  return (
    <div className="space-y-2">
      {sorted.map(({ entry, i }) => {
        const showHeader = entry.layer !== lastLayer
        lastLayer = entry.layer
        const layerError = showHeader ? fanInErrors[entry.layer] : undefined

        return (
          <div key={i}>
            {showHeader && (
              <div className="flex items-center gap-2 mt-3 first:mt-0 mb-1">
                <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                  Layer {entry.layer}
                </span>
                <div className="flex-1 h-px bg-border" />
                {layerError && (
                  <span className="flex items-center gap-1 text-xs text-destructive">
                    <AlertTriangle className="h-3 w-3" />
                    {layerError}
                  </span>
                )}
              </div>
            )}
            <div className="flex items-center gap-2 p-2 border border-border rounded-lg bg-muted/20">
              <div className="shrink-0 w-16">
                <label className="block text-[10px] text-muted-foreground mb-0.5">Layer</label>
                <input
                  type="number"
                  value={entry.layer}
                  onChange={(e) => update(i, { ...entry, layer: Number(e.target.value) })}
                  min={0}
                  className="w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm text-center"
                />
              </div>

              <div className="flex-1">
                <input
                  type="text"
                  value={entry.agent}
                  onChange={(e) => update(i, { ...entry, agent: e.target.value })}
                  placeholder="Agent type (e.g., setup-analyzer)"
                  required
                  className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-sm"
                />
              </div>

              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 text-destructive hover:text-destructive shrink-0"
                onClick={() => remove(i)}
                title="Remove agent"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        )
      })}

      <Button type="button" variant="outline" size="sm" onClick={add} className="w-full">
        <Plus className="h-3.5 w-3.5 mr-1" />
        Add Agent
      </Button>
    </div>
  )
}
