import { cn } from '@/lib/utils'

export type WorkflowSubTab = 'running' | 'failed' | 'completed'

export function WorkflowSubTabBar({
  activeSubTab,
  onSwitch,
  runningCount,
  failedCount,
  completedCount,
}: {
  activeSubTab: WorkflowSubTab
  onSwitch: (tab: WorkflowSubTab) => void
  runningCount: number
  failedCount: number
  completedCount: number
}) {
  const tabs: { id: WorkflowSubTab; label: string }[] = [
    { id: 'running', label: `Running (${runningCount})` },
    { id: 'failed', label: `Failed (${failedCount})` },
    { id: 'completed', label: `Completed (${completedCount})` },
  ]

  return (
    <div className="flex gap-1 mb-4">
      {tabs.map(({ id, label }) => (
        <button
          key={id}
          onClick={() => onSwitch(id)}
          className={cn(
            'px-3 py-1 text-xs font-medium rounded-md transition-colors',
            activeSubTab === id
              ? 'bg-primary/10 text-primary'
              : 'text-muted-foreground hover:text-foreground hover:bg-muted'
          )}
        >
          {label}
        </button>
      ))}
    </div>
  )
}
