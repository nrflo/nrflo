import { AlertTriangle, CheckCircle, Loader2 } from 'lucide-react'
import type { AgentSession, AgentHistoryEntry } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'

interface ConflictResolverBannerProps {
  sessions: AgentSession[]
  agentHistory: AgentHistoryEntry[]
  onAgentSelect: (data: SelectedAgentData | null) => void
}

export function ConflictResolverBanner({ sessions, agentHistory, onAgentSelect }: ConflictResolverBannerProps) {
  // Find conflict-resolver from sessions (project-scoped) or agentHistory (ticket-scoped)
  const runningSession = sessions?.find(s => s.agent_type === 'conflict-resolver' && s.status === 'running')
  const completedSession = sessions?.find(s => s.agent_type === 'conflict-resolver' && s.status !== 'running')
  const historyEntry = agentHistory?.find(a => a.agent_type === 'conflict-resolver')

  const handleClick = () => {
    if (runningSession) {
      onAgentSelect({ phaseName: '_conflict_resolution', session: runningSession })
    } else if (completedSession) {
      onAgentSelect({ phaseName: '_conflict_resolution', session: completedSession })
    } else if (historyEntry) {
      onAgentSelect({ phaseName: '_conflict_resolution', historyEntry })
    }
  }

  if (runningSession) {
    return (
      <button onClick={handleClick} className="w-full flex items-center gap-2 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-400 cursor-pointer hover:bg-amber-100 dark:hover:bg-amber-950/50 transition-colors">
        <Loader2 className="h-4 w-4 animate-spin" />
        <span className="font-medium">Resolving Merge Conflict...</span>
      </button>
    )
  }

  const resolved = completedSession?.result === 'pass' || historyEntry?.result === 'pass'
  const failed = completedSession?.result === 'fail' || historyEntry?.result === 'fail'

  if (resolved) {
    return (
      <button onClick={handleClick} className="w-full flex items-center gap-2 rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700 dark:border-green-800 dark:bg-green-950/30 dark:text-green-400 cursor-pointer hover:bg-green-100 dark:hover:bg-green-950/50 transition-colors">
        <CheckCircle className="h-4 w-4" />
        <span className="font-medium">Merge Conflict Resolved</span>
      </button>
    )
  }

  if (failed) {
    return (
      <button onClick={handleClick} className="w-full flex items-center gap-2 rounded-lg border border-orange-200 bg-orange-50 px-4 py-3 text-sm text-orange-700 dark:border-orange-800 dark:bg-orange-950/30 dark:text-orange-400 cursor-pointer hover:bg-orange-100 dark:hover:bg-orange-950/50 transition-colors">
        <AlertTriangle className="h-4 w-4" />
        <span className="font-medium">Merge Conflict Unresolved</span>
        <span className="text-xs ml-1">(branch preserved for manual merge)</span>
      </button>
    )
  }

  return null
}
