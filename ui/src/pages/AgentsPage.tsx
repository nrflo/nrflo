import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { Terminal, ExternalLink, FolderOpen } from 'lucide-react'
import { useRecentAgents } from '@/hooks/useTickets'
import { AgentSessionCard } from '@/components/workflow/AgentSessionCard'
import { Badge } from '@/components/ui/Badge'
import type { AgentsByProject } from '@/types/workflow'

export function AgentsPage() {
  const { data, isLoading, error } = useRecentAgents(10, { refetchInterval: 5000 })

  // Group sessions by project
  const groupedByProject = useMemo<AgentsByProject[]>(() => {
    if (!data?.sessions) return []

    const groups: Record<string, AgentsByProject> = {}
    for (const session of data.sessions) {
      const projectId = session.project_id || 'unknown'
      if (!groups[projectId]) {
        groups[projectId] = {
          project_id: projectId,
          project_name: data.projects[projectId] || projectId,
          agents: [],
        }
      }
      groups[projectId].agents.push(session)
    }

    // Sort by project name
    return Object.values(groups).sort((a, b) =>
      a.project_name.localeCompare(b.project_name)
    )
  }, [data])

  const runningCount = useMemo(() => {
    if (!data?.sessions) return 0
    return data.sessions.filter((s) => s.status === 'running').length
  }, [data])

  if (isLoading) {
    return (
      <div className="p-6">
        <div className="text-muted-foreground animate-pulse">Loading agents...</div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="text-destructive">Error loading agents: {error.message}</div>
      </div>
    )
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Terminal className="h-6 w-6" />
          <h1 className="text-2xl font-semibold">Recent Agents</h1>
          {runningCount > 0 && (
            <Badge className="bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200">
              {runningCount} running
            </Badge>
          )}
        </div>
        <span className="text-sm text-muted-foreground">
          Auto-refreshes every 5s
        </span>
      </div>

      {groupedByProject.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          <Terminal className="h-12 w-12 mx-auto mb-4 opacity-50" />
          <p>No recent agent sessions</p>
        </div>
      ) : (
        <div className="space-y-6">
          {groupedByProject.map((group) => (
            <div
              key={group.project_id}
              className="border border-border rounded-lg overflow-hidden"
            >
              <div className="bg-muted/50 px-4 py-3 flex items-center gap-2 border-b border-border">
                <FolderOpen className="h-4 w-4 text-muted-foreground" />
                <span className="font-medium">{group.project_name}</span>
                <Badge variant="outline" className="text-xs">
                  {group.agents.length} agent{group.agents.length !== 1 ? 's' : ''}
                </Badge>
              </div>
              <div className="p-4 space-y-3">
                {group.agents.map((session) => (
                  <AgentSessionCard
                    key={session.id}
                    session={session}
                    defaultExpanded={session.status === 'running'}
                  >
                    <Link
                      to={`/tickets/${encodeURIComponent(session.ticket_id)}`}
                      className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <span className="font-mono">{session.ticket_id}</span>
                      <ExternalLink className="h-3 w-3" />
                    </Link>
                  </AgentSessionCard>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
