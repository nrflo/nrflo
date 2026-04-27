import { Tooltip } from '@/components/ui/Tooltip'

const BUILTIN_TOOLS_HINT = [
  'findings_add, findings_get, findings_append, findings_delete',
  'project_findings_add, project_findings_get, project_findings_append, project_findings_delete',
  'agent_fail, agent_continue, agent_callback, agent_context_update',
  'workflow_skip',
  '* (wildcard — all builtins + all project HTTP tools)',
].join('\n')

interface AgentDefAPIModeFieldsProps {
  tools: string
  setTools: (v: string) => void
  apiMaxIterations: number | ''
  setApiMaxIterations: (v: number | '') => void
}

export function AgentDefAPIModeFields({ tools, setTools, apiMaxIterations, setApiMaxIterations }: AgentDefAPIModeFieldsProps) {
  const toolsEmpty = !tools.trim()

  return (
    <div className="space-y-3 rounded-md border border-violet-200 dark:border-violet-800 bg-violet-50/30 dark:bg-violet-950/10 p-3">
      <div>
        <div className="flex items-center gap-1 mb-1">
          <label className="text-xs font-medium text-muted-foreground">Tools (CSV)</label>
          <Tooltip
            text={`Available builtins:\n${BUILTIN_TOOLS_HINT}`}
            placement="top"
          >
            <span className="text-xs text-muted-foreground cursor-help underline decoration-dotted">?</span>
          </Tooltip>
        </div>
        <input
          type="text"
          value={tools}
          onChange={(e) => setTools(e.target.value)}
          placeholder="findings_add,findings_get,agent_fail,* "
          className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
        />
        {toolsEmpty && (
          <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">
            Tools must be non-empty for API-mode agents. Use <code>*</code> to allow all.
          </p>
        )}
        <p className="text-xs text-muted-foreground mt-1">
          Comma-separated tool names or glob patterns. HTTP tool definitions are resolved at spawn time.
        </p>
      </div>
      <div className="w-40">
        <label className="block text-xs font-medium text-muted-foreground mb-1">Max iterations</label>
        <input
          type="number"
          value={apiMaxIterations}
          onChange={(e) => setApiMaxIterations(e.target.value === '' ? '' : Number(e.target.value))}
          placeholder="50"
          min={1}
          className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
        />
        <p className="text-xs text-muted-foreground mt-1">Max tool-use turns (default 50)</p>
      </div>
    </div>
  )
}
