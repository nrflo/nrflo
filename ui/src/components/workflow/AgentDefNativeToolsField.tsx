import { useMemo, useState } from 'react'
import { Toggle } from '@/components/ui/Toggle'
import { Button } from '@/components/ui/Button'
import { Textarea } from '@/components/ui/Textarea'

// Curated list of claude CLI built-in tools offered as chips. Advisory only —
// the CLI's set evolves; the Advanced raw CSV covers anything missing.
// Agent/Task/Workflow/SendMessage are absent on purpose: nrflo force-denies
// them via --disallowedTools regardless of this field.
export const CLAUDE_NATIVE_TOOLS = [
  'Bash', 'BashOutput', 'Edit', 'Glob', 'Grep', 'KillShell',
  'NotebookEdit', 'Read', 'TodoWrite', 'WebFetch', 'WebSearch', 'Write',
] as const

// Sentinel meaning "disable every native tool" (spawner maps it to --tools "").
export const NATIVE_TOOLS_NONE = 'none'

/**
 * AgentDefNativeToolsField restricts a claude cli_interactive agent's native
 * (built-in) tools: empty = unrestricted (default), the 'none' sentinel
 * disables all of them (MCP-only agent), otherwise a CSV allowlist passed to
 * claude --tools. Rendered only for anthropic-provider CLI agents.
 */
export function AgentDefNativeToolsField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const selected = useMemo(
    () => new Set(value.split(',').map((s) => s.trim()).filter(Boolean)),
    [value]
  )
  const isAll = value.trim() === ''
  const isNone = value.trim() === NATIVE_TOOLS_NONE
  const hasUnknown = [...selected].some(
    (name) => name !== NATIVE_TOOLS_NONE && !(CLAUDE_NATIVE_TOOLS as readonly string[]).includes(name)
  )
  const [advanced, setAdvanced] = useState(hasUnknown)

  const toggleTool = (name: string) => {
    const next = new Set(isNone ? [] : selected)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    next.delete(NATIVE_TOOLS_NONE)
    onChange([...next].sort().join(','))
  }

  return (
    <div className="space-y-3 rounded-md border border-sky-200 dark:border-sky-800 bg-sky-50/30 dark:bg-sky-950/10 p-3">
      <div className="flex items-center justify-between">
        <label className="text-xs font-medium text-muted-foreground">Native CLI tools (claude)</label>
        <div className="flex items-center gap-4">
          <Toggle checked={isAll} onChange={(on) => onChange(on ? '' : NATIVE_TOOLS_NONE)} label="All (default)" />
          <Toggle checked={isNone} onChange={(on) => onChange(on ? NATIVE_TOOLS_NONE : '')} label="None (MCP only)" />
          <Toggle checked={advanced} onChange={setAdvanced} label="Advanced (raw)" />
        </div>
      </div>

      {advanced ? (
        <div>
          <Textarea
            value={value}
            onChange={(e) => onChange(e.target.value)}
            placeholder="Bash,Edit,Read  (empty = all, 'none' = disable all)"
            rows={2}
          />
          <p className="text-xs text-muted-foreground mt-1">
            Comma-separated built-in tool names passed to <code>--tools</code>. Empty = unrestricted;{' '}
            <code>none</code> (sole entry) disables every built-in tool.
          </p>
        </div>
      ) : (
        <div className="flex flex-wrap gap-1.5">
          {CLAUDE_NATIVE_TOOLS.map((name) => (
            <Button
              key={name}
              type="button"
              size="sm"
              variant={isAll || selected.has(name) ? 'default' : 'outline'}
              disabled={isAll}
              onClick={() => toggleTool(name)}
            >
              {name}
            </Button>
          ))}
        </div>
      )}

      {!isAll && !isNone && selected.size === 0 && (
        <p className="text-xs text-amber-600 dark:text-amber-400">
          No tools selected — an empty value means unrestricted. Pick tools or use “None (MCP only)”.
        </p>
      )}
    </div>
  )
}
