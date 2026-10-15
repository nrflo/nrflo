import { apiGet, apiPost } from './client'

export interface InboxNotification {
  id: string
  project_id: string
  ticket_id: string
  workflow_id: string
  event_type: string
  payload: Record<string, unknown>
  created_at: string
  read_at: string | null
}

export interface NotificationInbox {
  notifications: InboxNotification[]
  unread: number
  total: number
}

export function listInbox(global: boolean, offset = 0) {
  return apiGet<NotificationInbox>(`/api/v1/notifications${global ? '/global' : ''}?limit=50&offset=${offset}`)
}

export function markInboxRead(global: boolean, id?: string) {
  return apiPost(`/api/v1/notifications${global ? '/global' : ''}/read`, { id: id ?? '' })
}

export function getInboxNotification(global: boolean, id: string) {
  return apiGet<InboxNotification>(`/api/v1/notifications${global ? '/global' : ''}/${encodeURIComponent(id)}`)
}
