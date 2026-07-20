interface ChatStatusBarProps {
  engine?: string
  model?: string
  profile?: string
  workDir?: string
  contextLeft?: number
  cost?: number
  turn: 'idle' | 'running'
}

// Pure presentational bottom status bar: passive session info moved out of
// the top header. Single muted line; truncates the workdir segment and never
// reflows on value updates (tabular-nums on %/$).
export function ChatStatusBar({ engine, model, profile, workDir, contextLeft, cost, turn }: ChatStatusBarProps) {
  return (
    <div className="flex items-center gap-3 whitespace-nowrap overflow-hidden border-t border-border px-4 py-1 text-xs text-muted-foreground">
      <span className="shrink-0">
        {engine}
        {model && <span> · {model}</span>}
      </span>
      {profile && <span className="shrink-0">{profile}</span>}
      {workDir && <span className="truncate">{workDir}</span>}
      {contextLeft != null && <span className="shrink-0 tabular-nums">Context left: {contextLeft}%</span>}
      {cost != null && <span className="shrink-0 tabular-nums">~${cost.toFixed(2)}</span>}
      <span className="shrink-0">{turn === 'running' ? 'Running…' : 'Idle'}</span>
    </div>
  )
}
