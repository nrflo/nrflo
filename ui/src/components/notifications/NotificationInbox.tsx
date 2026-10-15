import { useState, useRef, useEffect } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { NotificationContent } from './NotificationContent'
import { Bell } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { useNotificationInbox, useInboxNotification } from '@/hooks/useNotificationInbox'
import { useConnectionsStore } from '@/stores/connectionsStore'
import { useProjectStore } from '@/stores/projectStore'
import type { InboxNotification } from '@/api/notificationInbox'

const labels: Record<string, string> = {
  'orchestration.completed': 'Workflow completed',
  'orchestration.failed': 'Workflow failed',
  'agent.completed': 'Agent failed',
  'agent.context_saving': 'Agent saving context',
  'agent.stall_restart': 'Stalled agent restarting',
}

export function NotificationInbox({ global = false, selectedId }: { global?: boolean; selectedId?: string }) {
  const [offset, setOffset] = useState(0)
  const { data, isPending, error, markRead } = useNotificationInbox(global, offset)
  const selected = useInboxNotification(global, selectedId)
  const connection = useConnectionsStore((s) => s.activeId)
  const selectedNode = useRef<HTMLElement | null>(null)
  useEffect(() => { selectedNode.current?.scrollIntoView?.({ block: 'center' }) }, [selected.data?.id])
  const notifications = selected.data
    ? [selected.data, ...(data?.notifications ?? []).filter((n) => n.id !== selected.data.id)]
    : data?.notifications
  const navigate = useNavigate()
  const open = (n: InboxNotification) => {
    if (n.project_id) useProjectStore.getState().setCurrentProject(n.project_id)
    navigate(n.ticket_id ? `/tickets/${encodeURIComponent(n.ticket_id)}?tab=workflow` : '/project-workflows')
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="flex items-center gap-2 text-lg font-semibold"><Bell className="h-5 w-5" />Notifications {data && <span className="text-amber-600">({data.unread} unread)</span>}</h2>
          <p className="text-sm text-muted-foreground">{global ? 'All projects and general notifications.' : 'Notifications for the selected project.'} Notifications are saved here automatically, alongside Telegram, Slack, and script deliveries.</p>
        </div>
        <Button variant="outline" disabled={!data?.unread || markRead.isPending} onClick={() => markRead.mutate(undefined)}>Mark all read</Button>
      </div>
      {(error || markRead.error || selected.error) && <p role="alert" className="text-destructive">{(error || markRead.error || selected.error)?.message}</p>}
      {isPending && <p className="text-muted-foreground">Loading notifications…</p>}
      {data?.total === 0 && <p className="py-8 text-center text-muted-foreground">No notifications yet.</p>}
      <div className="space-y-2">
        {notifications?.map((n) => (
          <article key={n.id} id={`notification-${n.id}`} tabIndex={-1} ref={n.id === selectedId ? selectedNode : undefined} className={`rounded-lg border p-4 ${n.id === selectedId ? 'ring-2 ring-primary' : ''} ${n.read_at ? 'border-border' : 'border-amber-400 bg-amber-500/5'}`}>
            <div className="flex flex-col sm:flex-row items-start justify-between gap-3">
              <div className="min-w-0 space-y-1">
                <h3 className="font-medium">{labels[n.event_type] ?? n.event_type}{!n.read_at && <span className="ml-2 text-xs text-amber-600">Unread</span>}</h3>
                <p className="text-xs text-muted-foreground">{[n.payload.project_name || n.project_id || 'General', n.ticket_id, n.workflow_id, n.payload.agent_type].filter(Boolean).join(' · ')} · {new Date(n.created_at).toLocaleString()}</p>
                <NotificationContent value={n.payload.workflow_final_result || n.payload.reason || n.payload.error} />
                <details className="text-xs text-muted-foreground"><summary className="cursor-pointer">Details</summary><NotificationContent value={n.payload} /></details>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Link className="text-xs text-primary underline" to={`/notifications?${new URLSearchParams({ scope: 'all', id: n.id, connection })}`}>Link</Link>
                {(n.ticket_id || n.workflow_id) && <Button variant="outline" size="sm" onClick={() => open(n)}>Open</Button>}
                {!n.read_at && <Button variant="ghost" size="sm" disabled={markRead.isPending} onClick={() => markRead.mutate(n.id)}>Mark read</Button>}
              </div>
            </div>
          </article>
        ))}
      </div>
      {data && data.total > 50 && <div className="flex items-center justify-end gap-3">
        <Button variant="outline" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}>Previous</Button>
        <span className="text-sm text-muted-foreground">{offset + 1}–{Math.min(offset + 50, data.total)} of {data.total}</span>
        <Button variant="outline" disabled={offset + 50 >= data.total} onClick={() => setOffset(offset + 50)}>Next</Button>
      </div>}
    </div>
  )
}
