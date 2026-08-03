import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/Button'
import { SessionsTable } from '@/components/sessions/SessionsTable'
import { SessionDetail } from '@/components/sessions/SessionDetail'
import { useSessions, type SessionsScope } from '@/hooks/useSessions'

const LIST_LIMIT = 100

export function LogsSessionsTab() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [scope, setScope] = useState<SessionsScope>('project')

  const sid = searchParams.get('sid') ?? undefined

  const { data, isLoading } = useSessions(scope, { limit: LIST_LIMIT })
  const sessions = data?.sessions ?? []

  const selectSession = (sessionId: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      next.set('tab', 'sessions')
      next.set('sid', sessionId)
      return next
    })
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Button variant={scope === 'project' ? 'default' : 'outline'} size="sm" onClick={() => setScope('project')}>
          This project
        </Button>
        <Button variant={scope === 'global' ? 'default' : 'outline'} size="sm" onClick={() => setScope('global')}>
          All projects
        </Button>
      </div>

      {isLoading ? (
        <div className="text-sm text-muted-foreground">Loading...</div>
      ) : (
        <div className="border rounded-lg overflow-hidden">
          <SessionsTable sessions={sessions} selectedId={sid} onSelect={selectSession} />
        </div>
      )}

      {sid && <SessionDetail sessionId={sid} />}
    </div>
  )
}
