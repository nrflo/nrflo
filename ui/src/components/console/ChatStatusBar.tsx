interface ChatStatusBarProps {
  engine?: string
  model?: string
  profile?: string
  workDir?: string
  contextLeft?: number
  cost?: number
  turn: 'idle' | 'running'
  yolo?: boolean
}

// Pure presentational bottom status bar: passive session info moved out of
// the top header. Single muted line; truncates the workdir segment and never
// reflows on value updates (tabular-nums on %/$).
export function ChatStatusBar({ engine, model, profile, workDir, contextLeft, cost, turn, yolo }: ChatStatusBarProps) {
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
      {yolo && (
        <span className="shrink-0 rounded-full border border-amber-500/40 bg-amber-500/10 px-1.5 py-0.5 font-medium text-amber-600 dark:text-amber-400">
          YOLO
        </span>
      )}
      <span className="shrink-0">{turn === 'running' ? 'Running…' : 'Idle'}</span>
    </div>
  )
}
