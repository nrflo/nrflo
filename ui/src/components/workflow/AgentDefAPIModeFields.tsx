interface AgentDefAPIModeFieldsProps {
  apiMaxIterations: number | ''
  setApiMaxIterations: (v: number | '') => void
  apiMaxTokens: number | ''
  setApiMaxTokens: (v: number | '') => void
}

export function AgentDefAPIModeFields({ apiMaxIterations, setApiMaxIterations, apiMaxTokens, setApiMaxTokens }: AgentDefAPIModeFieldsProps) {
  return (
    <div className="space-y-3 rounded-md border border-violet-200 dark:border-violet-800 bg-violet-50/30 dark:bg-violet-950/10 p-3">
      <div className="flex gap-3">
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
        <div className="w-40">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Max output tokens</label>
          <input
            type="number"
            value={apiMaxTokens}
            onChange={(e) => setApiMaxTokens(e.target.value === '' ? '' : Number(e.target.value))}
            placeholder="16384"
            min={1}
            className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
          />
          <p className="text-xs text-muted-foreground mt-1">Per-turn output cap (default 16384)</p>
        </div>
      </div>
    </div>
  )
}
