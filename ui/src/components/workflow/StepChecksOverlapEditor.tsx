import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { Toggle } from '@/components/ui/Toggle'
import type { PathOverlap } from '@/types/workflow'

// Advanced, optional sub-editor: checks (string list, same add/remove idiom
// as AgentDefForm's validation_commands) + path_overlap (raw JSON — the
// ticket frames path_overlap as optional/advanced, so a validated raw field
// avoids over-building a dedicated left/right key-group UI).
export function StepChecksOverlapEditor({
  checks,
  onChecksChange,
  pathOverlap,
  onPathOverlapChange,
}: {
  checks: string[]
  onChecksChange: (next: string[]) => void
  pathOverlap: PathOverlap | undefined
  onPathOverlapChange: (next: PathOverlap | undefined) => void
}) {
  const [overlapText, setOverlapText] = useState(() => (pathOverlap ? JSON.stringify(pathOverlap, null, 2) : ''))
  const [overlapEnabled, setOverlapEnabled] = useState(!!pathOverlap)
  const [overlapError, setOverlapError] = useState<string | null>(null)

  const handleOverlapText = (text: string) => {
    setOverlapText(text)
    if (text.trim() === '') {
      setOverlapError(null)
      onPathOverlapChange(undefined)
      return
    }
    try {
      const parsed = JSON.parse(text)
      if (!Array.isArray(parsed.left) || !Array.isArray(parsed.right)) {
        setOverlapError('must be an object with "left" and "right" string arrays')
        return
      }
      setOverlapError(null)
      onPathOverlapChange({ left: parsed.left, right: parsed.right })
    } catch {
      setOverlapError('invalid JSON')
    }
  }

  const handleOverlapEnabled = (checked: boolean) => {
    setOverlapEnabled(checked)
    if (!checked) {
      onPathOverlapChange(undefined)
      setOverlapText('')
      setOverlapError(null)
    } else if (overlapText.trim() === '') {
      const defaultText = '{\n  "left": [],\n  "right": []\n}'
      setOverlapText(defaultText)
      onPathOverlapChange({ left: [], right: [] })
    }
  }

  return (
    <div className="space-y-3">
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Checks</label>
        <div className="space-y-1.5">
          {checks.map((c, idx) => (
            <div key={idx} className="flex items-center gap-2">
              <Input
                value={c}
                onChange={(e) => onChecksChange(checks.map((v, i) => (i === idx ? e.target.value : v)))}
                placeholder="check description"
                className="flex-1"
              />
              <Button type="button" variant="ghost" size="sm" onClick={() => onChecksChange(checks.filter((_, i) => i !== idx))}>
                Remove
              </Button>
            </div>
          ))}
        </div>
        <Button type="button" variant="outline" size="sm" className="mt-1.5" disabled={checks.length >= 20} onClick={() => onChecksChange([...checks, ''])}>
          Add check
        </Button>
      </div>
      <div>
        <div className="flex items-center justify-between mb-1">
          <label className="text-xs font-medium text-muted-foreground">Path overlap gate</label>
          <Toggle checked={overlapEnabled} onChange={handleOverlapEnabled} label="Enabled" />
        </div>
        {overlapEnabled && (
          <>
            <Textarea value={overlapText} onChange={(e) => handleOverlapText(e.target.value)} rows={4} placeholder='{"left": ["key1"], "right": ["key2"]}' />
            {overlapError && <p className="text-xs text-destructive mt-1">{overlapError}</p>}
          </>
        )}
      </div>
    </div>
  )
}
