import { Link } from 'react-router-dom'
import { Bell } from 'lucide-react'
import { useNotificationInbox } from '@/hooks/useNotificationInbox'

export function NotificationIndicator() {
  const { data } = useNotificationInbox(true)
  const count = data?.unread ?? 0
  return <Link to="/notifications?scope=all" className="relative p-2 rounded-md hover:bg-muted" title={`${count} unread notifications`} aria-label={`Notifications (${count} unread)`}>
    <Bell className="h-5 w-5" />
    {count > 0 && <span className="absolute -top-1 -right-1 min-w-4 rounded-full bg-amber-500 px-1 text-center text-[10px] font-bold text-black">{count > 99 ? '99+' : count}</span>}
  </Link>
}
