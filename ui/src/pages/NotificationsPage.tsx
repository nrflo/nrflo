import { useEffect } from 'react'
import { useConnectionsStore } from '@/stores/connectionsStore'
import { useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/Button'
import { NotificationInbox } from '@/components/notifications/NotificationInbox'

export function NotificationsPage() {
  const [params, setParams] = useSearchParams()
  const connection = params.get('connection')
  const active = useConnectionsStore((s) => s.activeId)
  const connections = useConnectionsStore((s) => s.list)
  useEffect(() => {
    const store = useConnectionsStore.getState()
    if (connection && connection !== store.activeId && store.list.some((item) => item.id === connection)) store.setActive(connection)
  }, [connection])
  const global = params.get('scope') === 'all'
  if (connection && !connections.some((item) => item.id === connection)) return <p role="alert">The notification’s server connection is no longer configured.</p>
  if (connection && connection !== active) return <p className="text-muted-foreground">Switching to the notification’s server…</p>
  return <div className="max-w-5xl mx-auto space-y-6">
    <div className="flex gap-2">
      <Button variant={global ? 'outline' : 'default'} onClick={() => setParams({})}>Current project</Button>
      <Button variant={global ? 'default' : 'outline'} onClick={() => setParams({ scope: 'all' })}>All projects / General</Button>
    </div>
    <NotificationInbox key={global ? 'all' : 'project'} global={global} selectedId={params.get('id') ?? undefined} />
  </div>
}
