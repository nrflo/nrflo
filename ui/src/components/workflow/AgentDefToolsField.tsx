import { useMemo, useState } from 'react'
import { Toggle } from '@/components/ui/Toggle'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Textarea } from '@/components/ui/Textarea'
import { Spinner } from '@/components/ui/Spinner'
import { useAvailableTools } from '@/hooks/useAvailableTools'
import type { AvailableTool } from '@/types/availableTool'

type ExecutionMode = 'cli_interactive' | 'api' | 'script'

// matchName mirrors the backend matcher (apirun.MatchName): "*" matches all,
// a trailing "*" is a prefix match, otherwise an exact match.
function matchName(pattern: string, name: string): boolean {
  if (pattern === '*') return true
  if (pattern.endsWith('*')) return name.startsWith(pattern.slice(0, -1))
  return pattern === name
}

function parsePatterns(csv: string): string[] {
  return csv.split(',').map((s) => s.trim()).filter(Boolean)
}

interface Props {
  value: string
  onChange: (v: string) => void
  executionMode: ExecutionMode
}

/**
 * AgentDefToolsField is the dynamic per-agent tools picker. It lists the tools
 * actually available (builtins + project python tools), highlighting builtins
 * and pinning the mandatory tools. "All tools (*)" grants everything; an
 * Advanced mode exposes the raw CSV for glob patterns.
 */
export function AgentDefToolsField({ value, onChange, executionMode }: Props) {
  const { data: tools = [], isLoading } = useAvailableTools()

  const patterns = useMemo(() => parsePatterns(value), [value])
  const hasGlob = patterns.some((p) => p !== '*' && p.includes('*'))
  const [advanced, setAdvanced] = useState(hasGlob)

  const allSelected = value.trim() === '*'
  const mandatory = useMemo(() => tools.filter((t) => t.mandatory).map((t) => t.name), [tools])
  const builtins = useMemo(() => tools.filter((t) => t.source === 'builtin'), [tools])
  const pythonTools = useMemo(() => tools.filter((t) => t.source === 'python'), [tools])
  const selected = useMemo(() => new Set(patterns.filter((p) => !p.includes('*'))), [patterns])

  // Always include mandatory names so a subset never collapses to an
  // empty CSV — which means "all" for CLI agents.
  const emitSelection = (names: Set<string>) => {
    const all = new Set<string>(names)
    for (const m of mandatory) all.add(m)
    onChange(Array.from(all).sort().join(','))
  }

  const toggleTool = (name: string) => {
    const next = new Set(selected)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    emitSelection(next)
  }

  const warnings = advanced
    ? patterns.filter((p) => p !== '*' && !tools.some((t) => matchName(p, t.name)))
    : []

  const renderChip = (t: AvailableTool) => {
    const isSelected = allSelected || t.mandatory || selected.has(t.name)
    return (
      <Button
        key={t.name}
        type="button"
        size="sm"
        variant={isSelected ? 'default' : 'outline'}
        disabled={allSelected || t.mandatory}
        title={t.description}
        onClick={() => toggleTool(t.name)}
        className={t.source === 'builtin' ? 'border-violet-300 dark:border-violet-700' : ''}
      >
        {t.name}
        {t.mandatory && <span className="ml-1.5 opacity-70">· always</span>}
      </Button>
    )
  }

  return (
    <div className="space-y-3 rounded-md border border-violet-200 dark:border-violet-800 bg-violet-50/30 dark:bg-violet-950/10 p-3">
      <div className="flex items-center justify-between">
        <label className="text-xs font-medium text-muted-foreground">Tools</label>
        <div className="flex items-center gap-4">
          <Toggle checked={allSelected} onChange={(on) => onChange(on ? '*' : '')} label="All tools (*)" />
          <Toggle checked={advanced} onChange={setAdvanced} label="Advanced (raw)" />
        </div>
      </div>

      {isLoading ? (
        <div className="flex items-center gap-2 text-xs text-muted-foreground"><Spinner /> Loading tools…</div>
      ) : advanced ? (
        <div>
          <Textarea
            value={value}
            onChange={(e) => onChange(e.target.value)}
            placeholder="agent_*,findings_*,artifact_add  (or * for all)"
            rows={2}
          />
          <p className="text-xs text-muted-foreground mt-1">
            Comma-separated names or <code>prefix*</code> globs. <code>*</code> = all.
          </p>
          {warnings.length > 0 && (
            <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">
              {warnings.map((w) => `"${w}"`).join(', ')} match no known tool.
            </p>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          <div>
            <div className="flex items-center gap-1.5 mb-1.5">
              <Badge variant="secondary" className="bg-violet-100 text-violet-800 dark:bg-violet-900/30 dark:text-violet-300">built-in</Badge>
              <span className="text-xs text-muted-foreground">mandatory tools (· always) are granted to CLI agents regardless</span>
            </div>
            <div className="flex flex-wrap gap-1.5">{builtins.map(renderChip)}</div>
          </div>
          {pythonTools.length > 0 && (
            <div>
              <Badge variant="outline" className="mb-1.5">python</Badge>
              <div className="flex flex-wrap gap-1.5">{pythonTools.map(renderChip)}</div>
            </div>
          )}
        </div>
      )}

      {executionMode === 'api' && value.trim() === '' && (
        <p className="text-xs text-amber-600 dark:text-amber-400">
          Empty grants no tools to an API agent (text-only). Use “All tools” for everything.
        </p>
      )}
    </div>
  )
}
