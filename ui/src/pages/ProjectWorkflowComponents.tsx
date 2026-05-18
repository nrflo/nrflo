import { Bookmark, CheckCircle, Play, XCircle } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { cn, formatElapsedTime } from '@/lib/utils'
import type { WorkflowState } from '@/types/workflow'

export type StartMode = 'normal' | 'interactive' | 'plan' | 'endless'

// --- Instance List ---

export function InstanceList({
  instanceIds,
  instances,
  labels,
  selectedId,
  onSelect,
  tab,
}: {
  instanceIds: string[]
  instances: Record<string, WorkflowState>
  labels: Record<string, string>
  selectedId: string
  onSelect: (id: string) => void
  tab: 'running' | 'completed'
}) {
  return (
    <div className="flex flex-wrap gap-2">
      {instanceIds.map((id) => {
        const state = instances[id]
        const isSelected = id === selectedId
        return (
          <button
            key={id}
            onClick={() => onSelect(id)}
            className={cn(
              'flex items-center gap-2 px-3 py-1.5 rounded-md border text-sm transition-colors',
              isSelected
                ? 'border-primary bg-primary/10 text-primary'
                : 'border-border hover:border-primary/50 text-foreground'
            )}
          >
            <span className="font-medium">{labels[id] ?? id}</span>
            {tab === 'running' && (
              <Badge
                variant={state?.status === 'failed' ? 'destructive' : 'default'}
                className="text-xs"
              >
                {state?.status ?? 'active'}
              </Badge>
            )}
            {tab === 'completed' && (
              <Badge variant="success" className="text-xs">completed</Badge>
            )}
            {state?.current_phase && tab === 'running' && (
              <span className="text-xs text-muted-foreground">{state.current_phase}</span>
            )}
            {state?.completed_at && tab === 'running' && state?.status !== 'completed' && (
              <span className="text-xs text-muted-foreground">
                {formatElapsedTime(state.completed_at)}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}

// --- Tab Bar ---

export type ProjectWorkflowTabId = 'run' | 'running' | 'failed' | 'completed' | 'findings'

export function ProjectWorkflowTabBar({
  activeTab,
  onTabSwitch,
  runningCount,
  failedCount,
  completedCount,
}: {
  activeTab: ProjectWorkflowTabId
  onTabSwitch: (tab: ProjectWorkflowTabId) => void
  runningCount: number
  failedCount: number
  completedCount: number
}) {
  const tabs: { id: ProjectWorkflowTabId; label: string; icon?: typeof Play; count?: number }[] = [
    { id: 'run', label: 'Run Workflow', icon: Play },
    { id: 'running', label: 'Running', count: runningCount },
    { id: 'failed', label: 'Failed', icon: XCircle, count: failedCount },
    { id: 'completed', label: 'Completed', icon: CheckCircle, count: completedCount },
    { id: 'findings', label: 'Findings', icon: Bookmark },
  ]

  return (
    <div className="border-b border-border">
      <div className="flex gap-1">
        {tabs.map(({ id, label, icon: Icon, count }) => (
          <button
            key={id}
            onClick={() => onTabSwitch(id)}
            className={cn(
              'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === id
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            )}
          >
            {Icon && <Icon className="h-4 w-4" />}
            {count !== undefined ? `${label} (${count})` : label}
          </button>
        ))}
      </div>
    </div>
  )
}

