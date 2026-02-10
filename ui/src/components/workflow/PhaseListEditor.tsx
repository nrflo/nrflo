import { useState } from 'react'
import { ChevronUp, ChevronDown, Plus, Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'

export interface PhaseFormEntry {
  agent: string
  skip_for: string[]
}

interface PhaseListEditorProps {
  value: PhaseFormEntry[]
  onChange: (phases: PhaseFormEntry[]) => void
  categories: string[]
}

export function PhaseListEditor({ value, onChange, categories }: PhaseListEditorProps) {
  const [skipInput, setSkipInput] = useState<Record<number, string>>({})

  const update = (index: number, entry: PhaseFormEntry) => {
    const next = [...value]
    next[index] = entry
    onChange(next)
  }

  const move = (index: number, dir: -1 | 1) => {
    const target = index + dir
    if (target < 0 || target >= value.length) return
    const next = [...value]
    ;[next[index], next[target]] = [next[target], next[index]]
    onChange(next)
  }

  const remove = (index: number) => {
    onChange(value.filter((_, i) => i !== index))
  }

  const add = () => {
    onChange([...value, { agent: '', skip_for: [] }])
  }

  const addSkipFor = (index: number, cat: string) => {
    const entry = value[index]
    if (!cat || entry.skip_for.includes(cat)) return
    update(index, { ...entry, skip_for: [...entry.skip_for, cat] })
  }

  const removeSkipFor = (index: number, cat: string) => {
    const entry = value[index]
    update(index, { ...entry, skip_for: entry.skip_for.filter((c) => c !== cat) })
  }

  return (
    <div className="space-y-2">
      {value.map((entry, i) => (
        <div key={i} className="flex items-start gap-2 p-2 border border-border rounded-lg bg-muted/20">
          <span className="text-xs text-muted-foreground mt-2 w-5 text-right shrink-0">
            {i + 1}.
          </span>

          <div className="flex-1 space-y-1.5">
            <input
              type="text"
              value={entry.agent}
              onChange={(e) => update(i, { ...entry, agent: e.target.value })}
              placeholder="Agent type (e.g., setup-analyzer)"
              required
              className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-sm"
            />

            <div className="flex flex-wrap items-center gap-1">
              {entry.skip_for.map((cat) => (
                <Badge key={cat} variant="secondary" className="text-xs gap-1 pr-1">
                  {cat}
                  <button
                    type="button"
                    onClick={() => removeSkipFor(i, cat)}
                    className="hover:text-destructive"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </Badge>
              ))}

              {/* Quick-add from available categories */}
              {categories
                .filter((c) => !entry.skip_for.includes(c))
                .map((cat) => (
                  <button
                    key={cat}
                    type="button"
                    onClick={() => addSkipFor(i, cat)}
                    className="text-xs px-1.5 py-0.5 rounded border border-dashed border-border text-muted-foreground hover:text-foreground hover:border-foreground transition-colors"
                  >
                    +{cat}
                  </button>
                ))}

              {/* Manual skip_for input */}
              <input
                type="text"
                value={skipInput[i] || ''}
                onChange={(e) => setSkipInput({ ...skipInput, [i]: e.target.value })}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    const val = (skipInput[i] || '').trim()
                    if (val) {
                      addSkipFor(i, val)
                      setSkipInput({ ...skipInput, [i]: '' })
                    }
                  }
                }}
                placeholder="skip_for..."
                className="w-24 rounded border border-border bg-background px-1.5 py-0.5 text-xs"
              />
            </div>
          </div>

          <div className="flex flex-col gap-0.5 shrink-0">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-6 w-6 p-0"
              onClick={() => move(i, -1)}
              disabled={i === 0}
              title="Move up"
            >
              <ChevronUp className="h-3.5 w-3.5" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-6 w-6 p-0"
              onClick={() => move(i, 1)}
              disabled={i === value.length - 1}
              title="Move down"
            >
              <ChevronDown className="h-3.5 w-3.5" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-6 w-6 p-0 text-destructive hover:text-destructive"
              onClick={() => remove(i)}
              title="Remove phase"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      ))}

      <Button type="button" variant="outline" size="sm" onClick={add} className="w-full">
        <Plus className="h-3.5 w-3.5 mr-1" />
        Add Agent
      </Button>
    </div>
  )
}
