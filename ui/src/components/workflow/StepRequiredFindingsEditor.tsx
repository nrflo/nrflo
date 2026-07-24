import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Dropdown } from '@/components/ui/Dropdown'
import { STEP_FINDING_SCHEMAS } from '@/lib/stepDefinitions'
import type { RequiredFinding } from '@/types/workflow'

const SCHEMA_OPTIONS = STEP_FINDING_SCHEMAS.map((s) => ({ value: s, label: s }))

// required_findings sub-editor for a single step: add/remove key+schema rows.
export function StepRequiredFindingsEditor({
  value,
  onChange,
}: {
  value: RequiredFinding[]
  onChange: (next: RequiredFinding[]) => void
}) {
  const update = (idx: number, patch: Partial<RequiredFinding>) => {
    onChange(value.map((f, i) => (i === idx ? { ...f, ...patch } : f)))
  }

  return (
    <div className="space-y-1.5">
      <label className="block text-xs font-medium text-muted-foreground">Required findings</label>
      {value.map((f, idx) => (
        <div key={idx} className="flex items-center gap-2">
          <Input
            value={f.key}
            onChange={(e) => update(idx, { key: e.target.value })}
            placeholder="finding key"
            className="flex-1"
          />
          <div className="w-48">
            <Dropdown value={f.schema} onChange={(v) => update(idx, { schema: v })} options={SCHEMA_OPTIONS} placeholder="schema" />
          </div>
          <Button type="button" variant="ghost" size="sm" onClick={() => onChange(value.filter((_, i) => i !== idx))}>
            Remove
          </Button>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={value.length >= 20}
        onClick={() => onChange([...value, { key: '', schema: STEP_FINDING_SCHEMAS[0] }])}
      >
        Add required finding
      </Button>
    </div>
  )
}
