import { X } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import type { FindingSchema } from '@/types/workflow'

export interface FindingSchemaRow {
  key: string
  schemaText: string
  exampleText: string
}

interface FindingSchemasSectionProps {
  rows: FindingSchemaRow[]
  onChange: (rows: FindingSchemaRow[]) => void
}

/** Maps stored FindingSchema entries to editable text rows. */
export function findingSchemasToRows(defs?: FindingSchema[]): FindingSchemaRow[] {
  if (!defs) return []
  return defs.map((d) => ({
    key: d.key,
    schemaText: JSON.stringify(d.schema ?? {}, null, 2),
    exampleText: JSON.stringify(d.example ?? null, null, 2),
  }))
}

/** Parses editable rows back into FindingSchema entries. Empty rows (no key and
 *  no content) are dropped. Returns an error string on the first invalid JSON or
 *  incomplete row. */
export function parseFindingSchemaRows(rows: FindingSchemaRow[]): { schemas: FindingSchema[]; error: string | null } {
  const schemas: FindingSchema[] = []
  for (const row of rows) {
    const key = row.key.trim()
    const schemaText = row.schemaText.trim()
    const exampleText = row.exampleText.trim()
    if (!key && !schemaText && !exampleText) continue
    if (!key) return { schemas: [], error: 'Every finding schema needs a key.' }
    if (!schemaText) return { schemas: [], error: `Schema for "${key}" is required.` }
    if (!exampleText) return { schemas: [], error: `Example for "${key}" is required.` }
    let schema: unknown
    let example: unknown
    try {
      schema = JSON.parse(schemaText)
    } catch {
      return { schemas: [], error: `Schema for "${key}" is not valid JSON.` }
    }
    try {
      example = JSON.parse(exampleText)
    } catch {
      return { schemas: [], error: `Example for "${key}" is not valid JSON.` }
    }
    schemas.push({ key, schema, example })
  }
  return { schemas, error: null }
}

function jsonError(text: string): boolean {
  const t = text.trim()
  if (!t) return false
  try {
    JSON.parse(t)
    return false
  } catch {
    return true
  }
}

export function FindingSchemasSection({ rows, onChange }: FindingSchemasSectionProps) {
  const update = (i: number, patch: Partial<FindingSchemaRow>) => {
    onChange(rows.map((r, idx) => (idx === i ? { ...r, ...patch } : r)))
  }
  const remove = (i: number) => onChange(rows.filter((_, idx) => idx !== i))
  const add = () => onChange([...rows, { key: '', schemaText: '', exampleText: '' }])

  return (
    <div className="border-t border-border pt-3 space-y-3">
      <div className="text-xs font-medium text-muted-foreground">Finding schemas</div>
      <p className="text-xs text-muted-foreground">
        Validation contracts for the <code>emit_findings</code> tool. Each key maps to a JSON Schema (Draft 2020) the
        emitted value must satisfy, plus an example shown to agents when validation fails.
      </p>
      {rows.map((row, i) => (
        <div key={i} className="rounded border border-border p-2 space-y-2">
          <div className="flex items-center gap-2">
            <Input
              type="text"
              value={row.key}
              onChange={(e) => update(i, { key: e.target.value })}
              placeholder="Finding key (e.g. security_issues)"
            />
            <button
              type="button"
              onClick={() => remove(i)}
              className="hover:text-destructive shrink-0"
              aria-label={`Remove finding schema ${i + 1}`}
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          <div>
            <label className="block text-xs text-muted-foreground mb-1">Schema (JSON Schema)</label>
            <Textarea
              value={row.schemaText}
              onChange={(e) => update(i, { schemaText: e.target.value })}
              rows={6}
              className="font-mono text-xs"
              placeholder={'{\n  "type": "array",\n  "items": { "type": "object" }\n}'}
            />
            {jsonError(row.schemaText) && <p className="text-xs text-destructive mt-1">Invalid JSON</p>}
          </div>
          <div>
            <label className="block text-xs text-muted-foreground mb-1">Example (valid value)</label>
            <Textarea
              value={row.exampleText}
              onChange={(e) => update(i, { exampleText: e.target.value })}
              rows={3}
              className="font-mono text-xs"
              placeholder={'[{ "file": "a.go", "severity": "high" }]'}
            />
            {jsonError(row.exampleText) && <p className="text-xs text-destructive mt-1">Invalid JSON</p>}
          </div>
        </div>
      ))}
      <Button type="button" variant="outline" size="sm" onClick={add}>
        Add finding schema
      </Button>
    </div>
  )
}
