import { Terminal } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { AgentSessionCard } from './AgentSessionCard'
import type { AgentSession } from '@/types/workflow'

interface AgentMessagesPanelProps {
  sessions: AgentSession[]
  isLoading?: boolean
}

export function AgentMessagesPanel({ sessions, isLoading }: AgentMessagesPanelProps) {
  if (isLoading) {
    return (
      <div className="text-sm text-muted-foreground animate-pulse">
        Loading agent sessions...
      </div>
    )
  }

  if (!sessions || sessions.length === 0) {
    return null
  }

  // Sort sessions: running first, then by updated_at descending
  const sortedSessions = [...sessions].sort((a, b) => {
    if (a.status === 'running' && b.status !== 'running') return -1
    if (b.status === 'running' && a.status !== 'running') return 1
    return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
  })

  const runningCount = sessions.filter(s => s.status === 'running').length

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <Terminal className="h-3 w-3" />
        <span>Agent Sessions</span>
        {runningCount > 0 && (
          <Badge className="text-xs bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200">
            {runningCount} running
          </Badge>
        )}
      </div>
      <div className="space-y-2">
        {sortedSessions.map((session) => (
          <AgentSessionCard
            key={session.id}
            session={session}
            defaultExpanded={session.status === 'running'}
          />
        ))}
      </div>
    </div>
  )
}
